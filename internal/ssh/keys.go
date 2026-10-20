package ssh

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type KeyTool interface {
	Generate(context.Context, []string, Streams) error
}

type OpenSSHKeyTool struct {
	LookPath func(string) (string, error)
}

func (t OpenSSHKeyTool) Generate(ctx context.Context, args []string, streams Streams) error {
	lookPath := t.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("ssh-keygen")
	if err != nil {
		return fmt.Errorf("%w: install OpenSSH and ensure 'ssh-keygen' is available in PATH", ErrSSHMissing)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = streams.In, streams.Out, streams.Err
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("ssh-keygen failed: %w", err)
	}
	return nil
}

func (s *Service) GenerateKey(ctx context.Context, request KeyGenerateRequest, streams Streams) (Key, error) {
	name, err := NormalizeAlias(request.Name)
	if err != nil {
		return Key{}, fmt.Errorf("invalid key name: %w", err)
	}
	keyType := strings.ToLower(strings.TrimSpace(request.Type))
	if keyType == "" {
		keyType = "ed25519"
	}
	if keyType != "ed25519" && keyType != "rsa" {
		return Key{}, fmt.Errorf("%w: key type must be ed25519 or rsa", ErrValidation)
	}
	if keyType == "rsa" {
		if request.Bits == 0 {
			request.Bits = 4096
		}
		if request.Bits < 3072 || request.Bits > 16384 {
			return Key{}, fmt.Errorf("%w: RSA bits must be between 3072 and 16384", ErrValidation)
		}
	}
	comment, err := ValidateDescription(request.Comment)
	if err != nil {
		return Key{}, err
	}
	if comment == "" {
		comment = "aiai:" + name
	}
	dir, err := s.managedKeyDir()
	if err != nil {
		return Key{}, err
	}
	if err := ensureKeyDirectory(dir); err != nil {
		return Key{}, err
	}
	privatePath := filepath.Join(dir, name)
	publicPath := privatePath + ".pub"
	if _, err := os.Lstat(privatePath); err == nil {
		return Key{}, fmt.Errorf("%w: managed key %q already exists", ErrDuplicate, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Key{}, fmt.Errorf("%w: inspect key destination: %v", ErrStorage, err)
	}
	args := []string{"-t", keyType, "-f", privatePath, "-C", comment}
	if keyType == "rsa" {
		args = append(args, "-b", strconv.Itoa(request.Bits))
	}
	if request.NoPassphrase {
		args = append(args, "-N", "")
	}
	tool := s.KeyTool
	if tool == nil {
		tool = OpenSSHKeyTool{}
	}
	if err := tool.Generate(ctx, args, streams); err != nil {
		_ = os.Remove(privatePath)
		_ = os.Remove(publicPath)
		return Key{}, err
	}
	if err := secureGeneratedKey(privatePath, publicPath); err != nil {
		return Key{}, err
	}
	key, err := inspectPublicKey(publicPath, privatePath, true)
	if err != nil {
		return Key{}, err
	}
	return key, nil
}

func (s *Service) FindKeys(ctx context.Context, query string) ([]Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	managed, err := s.managedKeyDir()
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Lstat(managed); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return nil, fmt.Errorf("%w: managed key directory must be a directory, not a symlink", ErrStorage)
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: inspect managed key directory: %v", ErrStorage, statErr)
	}
	dirs := []struct {
		path    string
		managed bool
	}{{managed, true}}
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		standard := filepath.Join(home, ".ssh")
		if standard != managed {
			dirs = append(dirs, struct {
				path    string
				managed bool
			}{standard, false})
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	seen := map[string]bool{}
	keys := []Key{}
	for _, dir := range dirs {
		entries, readErr := os.ReadDir(dir.path)
		if errors.Is(readErr, fs.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("%w: scan key directory %s: %v", ErrStorage, dir.path, readErr)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
				continue
			}
			publicPath := filepath.Join(dir.path, entry.Name())
			privatePath := strings.TrimSuffix(publicPath, ".pub")
			privateInfo, statErr := os.Lstat(privatePath)
			if statErr != nil || privateInfo.Mode()&os.ModeSymlink != 0 || !privateInfo.Mode().IsRegular() {
				continue
			}
			canonical := filepath.Clean(privatePath)
			if seen[canonical] {
				continue
			}
			key, inspectErr := inspectPublicKey(publicPath, privatePath, dir.managed)
			if inspectErr != nil {
				continue
			}
			haystack := strings.ToLower(key.Name + " " + key.Type + " " + key.Comment + " " + key.Fingerprint + " " + key.PrivatePath)
			if query != "" && !strings.Contains(haystack, query) {
				continue
			}
			seen[canonical] = true
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Managed != keys[j].Managed {
			return keys[i].Managed
		}
		return keys[i].Name < keys[j].Name
	})
	return keys, nil
}

func (s *Service) FindKey(ctx context.Context, value string) (Key, error) {
	keys, err := s.FindKeys(ctx, value)
	if err != nil {
		return Key{}, err
	}
	for _, key := range keys {
		if strings.EqualFold(key.Name, value) || key.PrivatePath == value || key.PublicPath == value {
			return key, nil
		}
	}
	if len(keys) == 1 {
		return keys[0], nil
	}
	if len(keys) > 1 {
		return Key{}, fmt.Errorf("%w: key query %q is ambiguous", ErrValidation, value)
	}
	return Key{}, fmt.Errorf("%w: SSH key %q", ErrNotFound, value)
}

func (s *Service) PublicKey(ctx context.Context, value string) (string, error) {
	key, err := s.FindKey(ctx, value)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Lstat(key.PublicPath)
	if err != nil {
		return "", fmt.Errorf("%w: inspect public key: %v", ErrStorage, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return "", fmt.Errorf("%w: public key path is unsafe", ErrStorage)
	}
	payload, err := os.ReadFile(key.PublicPath)
	if err != nil {
		return "", fmt.Errorf("%w: read public key: %v", ErrStorage, err)
	}
	publicKey := strings.TrimSpace(string(payload))
	if publicKey == "" {
		return "", fmt.Errorf("%w: public key is empty", ErrValidation)
	}
	return publicKey, nil
}

func (s *Service) RemoveKey(ctx context.Context, value string) error {
	key, err := s.FindKey(ctx, value)
	if err != nil {
		return err
	}
	if !key.Managed {
		return fmt.Errorf("%w: refusing to remove a key outside AIAI's managed directory", ErrValidation)
	}
	managed, err := s.managedKeyDir()
	if err != nil {
		return err
	}
	if filepath.Dir(key.PrivatePath) != managed || key.PublicPath != key.PrivatePath+".pub" {
		return fmt.Errorf("%w: managed key paths are unsafe", ErrStorage)
	}
	for _, path := range []string{key.PrivatePath, key.PublicPath} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return fmt.Errorf("%w: inspect key before removal: %v", ErrStorage, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: refusing to remove unsafe key path", ErrStorage)
		}
	}
	// Remove the private half first so a partial failure cannot leave private
	// key material discoverable as a usable pair.
	if err := os.Remove(key.PrivatePath); err != nil {
		return fmt.Errorf("%w: remove private key: %v", ErrStorage, err)
	}
	if err := os.Remove(key.PublicPath); err != nil {
		return fmt.Errorf("%w: private key removed but public key cleanup failed: %v", ErrStorage, err)
	}
	return nil
}

func (s *Service) managedKeyDir() (string, error) {
	if s.KeyDir != "" {
		return filepath.Abs(s.KeyDir)
	}
	if repository, ok := s.Repository.(*JSONRepository); ok {
		return filepath.Join(filepath.Dir(repository.Path), "keys"), nil
	}
	path, err := DefaultStoragePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "keys"), nil
}

func ensureKeyDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("%w: create managed key directory: %v", ErrStorage, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("%w: inspect managed key directory: %v", ErrStorage, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: managed key directory must not be a symlink", ErrStorage)
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("%w: secure managed key directory: %v", ErrStorage, err)
		}
	}
	return nil
}

func secureGeneratedKey(privatePath, publicPath string) error {
	for path, mode := range map[string]fs.FileMode{privatePath: 0o600, publicPath: 0o644} {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("%w: generated key file is missing: %v", ErrStorage, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: generated key path is unsafe", ErrStorage)
		}
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("%w: secure generated key: %v", ErrStorage, err)
		}
	}
	return nil
}

func inspectPublicKey(publicPath, privatePath string, managed bool) (Key, error) {
	file, err := os.Open(publicPath)
	if err != nil {
		return Key{}, fmt.Errorf("%w: read public key: %v", ErrStorage, err)
	}
	defer file.Close()
	line, err := bufio.NewReader(io.LimitReader(file, 16*1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return Key{}, fmt.Errorf("%w: read public key: %v", ErrStorage, err)
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return Key{}, fmt.Errorf("%w: invalid public key %s", ErrValidation, publicPath)
	}
	blob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return Key{}, fmt.Errorf("%w: invalid public key encoding", ErrValidation)
	}
	digest := sha256.Sum256(blob)
	comment := strings.Join(parts[2:], " ")
	info, _ := os.Stat(privatePath)
	created := time.Time{}
	if info != nil {
		created = info.ModTime().UTC()
	}
	return Key{
		Name: filepath.Base(privatePath), Type: parts[0],
		Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]), Comment: comment,
		PrivatePath: privatePath, PublicPath: publicPath, Managed: managed, CreatedAt: created,
	}, nil
}
