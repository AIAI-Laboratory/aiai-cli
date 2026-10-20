package zen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrStorage        = errors.New("zen storage error")
	ErrInvalidConfig  = errors.New("invalid Zen configuration")
	ErrInvalidSession = errors.New("invalid Zen session")
)

type Repository interface {
	Load(context.Context) (Config, []Session, error)
	SaveConfig(context.Context, Config) error
	AddSession(context.Context, Session) error
}

type JSONRepository struct{ Path string }

type storageFile struct {
	SchemaVersion int       `json:"schema_version"`
	Config        Config    `json:"config"`
	Sessions      []Session `json:"sessions"`
}

func DefaultStoragePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("AIAI_CONFIG_DIR")); configured != "" {
		base, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("%w: normalize AIAI_CONFIG_DIR: %v", ErrStorage, err)
		}
		return filepath.Join(base, "zen.json"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("%w: locate config directory: %v", ErrStorage, err)
	}
	return filepath.Join(base, "aiai", "zen.json"), nil
}

func NewJSONRepository(path string) (*JSONRepository, error) {
	if path == "" {
		var err error
		path, err = DefaultStoragePath()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: normalize path: %v", ErrStorage, err)
	}
	return &JSONRepository{Path: filepath.Clean(abs)}, nil
}

func emptyStorage() storageFile {
	return storageFile{SchemaVersion: SchemaVersion, Config: DefaultConfig(), Sessions: []Session{}}
}

func (r *JSONRepository) Load(ctx context.Context) (Config, []Session, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, nil, err
	}
	data, err := r.read()
	if errors.Is(err, fs.ErrNotExist) {
		data = emptyStorage()
		err = nil
	}
	if err != nil {
		return Config{}, nil, err
	}
	sessions := make([]Session, len(data.Sessions))
	copy(sessions, data.Sessions)
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt.After(sessions[j].StartedAt) })
	return data.Config, sessions, nil
}

func (r *JSONRepository) SaveConfig(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	return r.update(ctx, func(data *storageFile) { data.Config = config })
}

func (r *JSONRepository) AddSession(ctx context.Context, session Session) error {
	if err := validateSession(session); err != nil {
		return err
	}
	return r.update(ctx, func(data *storageFile) { data.Sessions = append(data.Sessions, session) })
}

func (r *JSONRepository) update(ctx context.Context, mutate func(*storageFile)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
		return fmt.Errorf("%w: create directory: %v", ErrStorage, err)
	}
	if err := os.Chmod(filepath.Dir(r.Path), 0o700); err != nil {
		return fmt.Errorf("%w: secure directory: %v", ErrStorage, err)
	}
	unlock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := r.read()
	if errors.Is(err, fs.ErrNotExist) {
		data, err = emptyStorage(), nil
	}
	if err != nil {
		return err
	}
	mutate(&data)
	return r.write(data)
}

func (r *JSONRepository) read() (storageFile, error) {
	info, err := os.Lstat(r.Path)
	if err != nil {
		return storageFile{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return storageFile{}, fmt.Errorf("%w: path is not a regular file", ErrStorage)
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(r.Path, 0o600); err != nil {
			return storageFile{}, fmt.Errorf("%w: secure file: %v", ErrStorage, err)
		}
	}
	payload, err := os.ReadFile(r.Path)
	if err != nil {
		return storageFile{}, fmt.Errorf("%w: read: %v", ErrStorage, err)
	}
	var data storageFile
	if err := json.Unmarshal(payload, &data); err != nil {
		return storageFile{}, fmt.Errorf("%w: corrupt JSON at %s: %v", ErrStorage, r.Path, err)
	}
	if data.SchemaVersion != SchemaVersion {
		return storageFile{}, fmt.Errorf("%w: unsupported schema %d", ErrStorage, data.SchemaVersion)
	}
	if err := data.Config.Validate(); err != nil {
		return storageFile{}, fmt.Errorf("%w: invalid configuration", ErrStorage)
	}
	if data.Sessions == nil {
		data.Sessions = []Session{}
	}
	for _, session := range data.Sessions {
		if err := validateSession(session); err != nil {
			return storageFile{}, fmt.Errorf("%w: %v", ErrStorage, err)
		}
	}
	return data, nil
}

func validateSession(session Session) error {
	if strings.TrimSpace(session.ID) == "" || session.PlannedSeconds < 0 || session.ActiveSeconds < 0 {
		return ErrInvalidSession
	}
	switch session.Type {
	case SessionFocus, SessionShortBreak, SessionLongBreak:
	default:
		return ErrInvalidSession
	}
	switch session.Status {
	case StatusCompleted, StatusInterrupted, StatusCanceled:
	default:
		return ErrInvalidSession
	}
	return nil
}

func (r *JSONRepository) write(data storageFile) error {
	tmp, err := os.CreateTemp(filepath.Dir(r.Path), ".zen-*.json")
	if err != nil {
		return fmt.Errorf("%w: create temporary file: %v", ErrStorage, err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: encode: %v", ErrStorage, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceStorageFile(name, r.Path); err != nil {
		return fmt.Errorf("%w: publish: %v", ErrStorage, err)
	}
	return nil
}

func (r *JSONRepository) lock(ctx context.Context) (func(), error) {
	path := r.Path + ".lock"
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("%w: acquire lock: %v", ErrStorage, err)
		}
		if info, statErr := os.Lstat(path); statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
