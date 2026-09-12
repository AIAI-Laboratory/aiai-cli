package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "aiai-cli"

type Store interface {
	Load(origin string) (StoredCredential, string, error)
	Save(origin string, credential StoredCredential) (string, error)
	Delete(origin string) error
}

type FailingStore struct {
	Err error
}

func (s FailingStore) Load(string) (StoredCredential, string, error) {
	return StoredCredential{}, "", s.Err
}
func (s FailingStore) Save(string, StoredCredential) (string, error) { return "", s.Err }
func (s FailingStore) Delete(string) error                           { return s.Err }

type SecretKeyring interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (systemKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemKeyring) Delete(service, account string) error { return keyring.Delete(service, account) }

type HybridStore struct {
	Keyring SecretKeyring
	File    *FileStore
}

func NewCredentialStore(configDir string) (*HybridStore, error) {
	if configDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("%w: locate user config directory: %v", ErrIO, err)
		}
		configDir = filepath.Join(base, "aiai")
	}
	return &HybridStore{Keyring: systemKeyring{}, File: &FileStore{Dir: configDir}}, nil
}

func (s *HybridStore) Load(origin string) (StoredCredential, string, error) {
	account := accountFor(origin)
	secret, keyringErr := s.Keyring.Get(keyringService, account)
	if keyringErr == nil {
		credential, err := decodeCredential(secret, origin)
		if err != nil {
			return StoredCredential{}, "", err
		}
		return credential, "keyring", nil
	}
	credential, fileErr := s.File.Load(origin)
	if fileErr != nil {
		if errors.Is(fileErr, ErrNotFound) && errors.Is(keyringErr, keyring.ErrNotFound) {
			return StoredCredential{}, "", ErrNotFound
		}
		if !errors.Is(fileErr, ErrNotFound) {
			return StoredCredential{}, "", fileErr
		}
		return StoredCredential{}, "", ErrNotFound
	}
	encoded, err := json.Marshal(credential)
	if err == nil && s.Keyring.Set(keyringService, account, string(encoded)) == nil {
		if err := s.File.Delete(origin); err == nil {
			return credential, "keyring", nil
		}
	}
	return credential, "file", nil
}

func (s *HybridStore) Save(origin string, credential StoredCredential) (string, error) {
	fileHasOrigin := false
	if _, err := s.File.Load(origin); err == nil {
		fileHasOrigin = true
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	credential.SchemaVersion = 1
	credential.Origin = origin
	encoded, err := json.Marshal(credential)
	if err != nil {
		return "", fmt.Errorf("%w: encode credential: %v", ErrIO, err)
	}
	if err := s.Keyring.Set(keyringService, accountFor(origin), string(encoded)); err == nil {
		if fileHasOrigin {
			if err := s.File.Delete(origin); err != nil {
				return "", err
			}
		}
		return "keyring", nil
	}
	if err := s.File.Save(origin, credential); err != nil {
		return "", err
	}
	return "file", nil
}

func (s *HybridStore) Delete(origin string) error {
	_, fileErr := s.File.Load(origin)
	keyringErr := s.Keyring.Delete(keyringService, accountFor(origin))
	deleteFileErr := s.File.Delete(origin)
	if deleteFileErr != nil && !errors.Is(deleteFileErr, ErrNotFound) {
		return deleteFileErr
	}
	if keyringErr != nil && !errors.Is(keyringErr, keyring.ErrNotFound) && errors.Is(fileErr, ErrNotFound) {
		return fmt.Errorf("%w: delete keyring credential: %v", ErrIO, keyringErr)
	}
	return nil
}

type credentialFile struct {
	SchemaVersion int                         `json:"schema_version"`
	Credentials   map[string]StoredCredential `json:"credentials"`
}

type FileStore struct {
	Dir string
}

func (s *FileStore) path() string { return filepath.Join(s.Dir, "credentials.json") }

func (s *FileStore) Load(origin string) (StoredCredential, error) {
	data, err := s.read()
	if err != nil {
		return StoredCredential{}, err
	}
	credential, ok := data.Credentials[origin]
	if !ok {
		return StoredCredential{}, ErrNotFound
	}
	if credential.Origin != origin || credential.SchemaVersion != 1 {
		return StoredCredential{}, fmt.Errorf("%w: credential file contains invalid origin or schema", ErrIO)
	}
	return credential, nil
}

func (s *FileStore) Save(origin string, credential StoredCredential) error {
	data, err := s.read()
	if errors.Is(err, ErrNotFound) {
		data = credentialFile{SchemaVersion: 1, Credentials: map[string]StoredCredential{}}
	} else if err != nil {
		return err
	}
	if data.Credentials == nil {
		data.Credentials = map[string]StoredCredential{}
	}
	credential.SchemaVersion = 1
	credential.Origin = origin
	data.SchemaVersion = 1
	data.Credentials[origin] = credential
	return s.write(data)
}

func (s *FileStore) Delete(origin string) error {
	data, err := s.read()
	if err != nil {
		return err
	}
	if _, ok := data.Credentials[origin]; !ok {
		return ErrNotFound
	}
	delete(data.Credentials, origin)
	if len(data.Credentials) == 0 {
		if err := os.Remove(s.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: delete credential file: %v", ErrIO, err)
		}
		return nil
	}
	return s.write(data)
}

func (s *FileStore) read() (credentialFile, error) {
	path := s.path()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentialFile{}, ErrNotFound
	}
	if err != nil {
		return credentialFile{}, fmt.Errorf("%w: inspect credential file: %v", ErrIO, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return credentialFile{}, fmt.Errorf("%w: credential path must be a regular file", ErrIO)
	}
	if err := validateCredentialFile(path, info); err != nil {
		return credentialFile{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return credentialFile{}, fmt.Errorf("%w: read credential file: %v", ErrIO, err)
	}
	var data credentialFile
	if err := json.Unmarshal(payload, &data); err != nil || data.SchemaVersion != 1 {
		return credentialFile{}, fmt.Errorf("%w: decode credential file", ErrIO)
	}
	return data, nil
}

func (s *FileStore) write(data credentialFile) error {
	if err := ensurePrivateDir(s.Dir); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.Dir, ".credentials-*")
	if err != nil {
		return fmt.Errorf("%w: create temporary credential file: %v", ErrIO, err)
	}
	tempPath := temporary.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("%w: secure temporary credential file: %v", ErrIO, err)
	}
	if err := secureCredentialPath(tempPath, false); err != nil {
		_ = temporary.Close()
		cleanup()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("%w: write credential file: %v", ErrIO, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("%w: sync credential file: %v", ErrIO, err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return fmt.Errorf("%w: close credential file: %v", ErrIO, err)
	}
	if err := replaceFile(tempPath, s.path()); err != nil {
		cleanup()
		return fmt.Errorf("%w: publish credential file: %v", ErrIO, err)
	}
	return nil
}

func ensurePrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("%w: create credential directory: %v", ErrIO, err)
		}
		return secureCredentialPath(dir, true)
	}
	if err != nil {
		return fmt.Errorf("%w: inspect credential directory: %v", ErrIO, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: credential directory must not be a symlink", ErrIO)
	}
	return secureCredentialPath(dir, true)
}

func decodeCredential(secret, origin string) (StoredCredential, error) {
	var credential StoredCredential
	if err := json.Unmarshal([]byte(secret), &credential); err != nil || credential.SchemaVersion != 1 || credential.Origin != origin {
		return StoredCredential{}, fmt.Errorf("%w: decode keyring credential", ErrIO)
	}
	return credential, nil
}

func accountFor(origin string) string {
	hash := sha256.Sum256([]byte(origin))
	return "session-" + hex.EncodeToString(hash[:12])
}
