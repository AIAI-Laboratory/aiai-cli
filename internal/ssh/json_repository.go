package ssh

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

const storageSchemaVersion = 1

type JSONRepository struct {
	Path string
	Now  func() time.Time
}

type storageFile struct {
	SchemaVersion int               `json:"schema_version"`
	Servers       []Server          `json:"servers"`
	AccessLogs    []storedAccessLog `json:"ssh_access_logs"`
}

type storedAccessLog struct {
	AccessLog
	OwnerPID int `json:"owner_pid,omitempty"`
}

func DefaultStoragePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("AIAI_CONFIG_DIR")); configured != "" {
		base, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("%w: normalize AIAI_CONFIG_DIR: %v", ErrStorage, err)
		}
		return filepath.Join(base, "ssh.json"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("%w: locate user config directory: %v", ErrStorage, err)
	}
	return filepath.Join(base, "aiai", "ssh.json"), nil
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
		return nil, fmt.Errorf("%w: normalize storage path: %v", ErrStorage, err)
	}
	return &JSONRepository{Path: filepath.Clean(abs)}, nil
}

func (r *JSONRepository) AddServer(ctx context.Context, server Server) error {
	return r.update(ctx, func(data *storageFile) error {
		for _, existing := range data.Servers {
			if strings.EqualFold(existing.Alias, server.Alias) {
				return fmt.Errorf("%w: %s", ErrDuplicate, server.Alias)
			}
		}
		data.Servers = append(data.Servers, server)
		return nil
	})
}

func (r *JSONRepository) ListServers(ctx context.Context) ([]Server, error) {
	data, err := r.read(ctx)
	if err != nil {
		return nil, err
	}
	servers := make([]Server, len(data.Servers))
	copy(servers, data.Servers)
	sort.Slice(servers, func(i, j int) bool { return servers[i].Alias < servers[j].Alias })
	return servers, nil
}

func (r *JSONRepository) GetServer(ctx context.Context, alias string) (Server, error) {
	data, err := r.read(ctx)
	if err != nil {
		return Server{}, err
	}
	for _, server := range data.Servers {
		if strings.EqualFold(server.Alias, alias) {
			return server, nil
		}
	}
	return Server{}, fmt.Errorf("%w: %s", ErrNotFound, alias)
}

func (r *JSONRepository) UpdateServer(ctx context.Context, server Server) error {
	return r.update(ctx, func(data *storageFile) error {
		for i := range data.Servers {
			if strings.EqualFold(data.Servers[i].Alias, server.Alias) {
				data.Servers[i] = server
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrNotFound, server.Alias)
	})
}

func (r *JSONRepository) RemoveServer(ctx context.Context, alias string) error {
	return r.update(ctx, func(data *storageFile) error {
		for i := range data.Servers {
			if strings.EqualFold(data.Servers[i].Alias, alias) {
				data.Servers = append(data.Servers[:i], data.Servers[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrNotFound, alias)
	})
}

func (r *JSONRepository) StartLog(ctx context.Context, entry AccessLog) error {
	return r.update(ctx, func(data *storageFile) error {
		data.AccessLogs = append(data.AccessLogs, storedAccessLog{AccessLog: entry, OwnerPID: os.Getpid()})
		return nil
	})
}

func (r *JSONRepository) FinishLog(ctx context.Context, entry AccessLog) error {
	return r.update(ctx, func(data *storageFile) error {
		for i := range data.AccessLogs {
			if data.AccessLogs[i].ID == entry.ID {
				data.AccessLogs[i] = storedAccessLog{AccessLog: entry}
				return nil
			}
		}
		return fmt.Errorf("%w: access log %s not found", ErrStorage, entry.ID)
	})
}

func (r *JSONRepository) ListLogs(ctx context.Context, filter LogFilter) ([]AccessLog, error) {
	// Taking the write lock here also lets us finalize entries abandoned by a
	// process crash before returning history to the user.
	logs := []AccessLog{}
	err := r.update(ctx, func(data *storageFile) error {
		for _, stored := range data.AccessLogs {
			entry := stored.AccessLog
			if filter.ServerAlias == "" || strings.EqualFold(entry.ServerAlias, filter.ServerAlias) {
				logs = append(logs, entry)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].StartedAt.After(logs[j].StartedAt) })
	if len(logs) > filter.Limit {
		logs = logs[:filter.Limit]
	}
	return logs, nil
}

func (r *JSONRepository) read(ctx context.Context) (storageFile, error) {
	if err := ctx.Err(); err != nil {
		return storageFile{}, err
	}
	if err := r.ensureDirectory(); err != nil {
		return storageFile{}, err
	}
	data, err := r.load()
	if errors.Is(err, fs.ErrNotExist) {
		return emptyStorage(), nil
	}
	return data, err
}

func (r *JSONRepository) update(ctx context.Context, mutate func(*storageFile) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensureDirectory(); err != nil {
		return err
	}
	unlock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := r.load()
	if errors.Is(err, fs.ErrNotExist) {
		data = emptyStorage()
	} else if err != nil {
		return err
	}
	r.recoverPending(&data)
	if err := mutate(&data); err != nil {
		return err
	}
	return r.write(data)
}

func emptyStorage() storageFile {
	return storageFile{SchemaVersion: storageSchemaVersion, Servers: []Server{}, AccessLogs: []storedAccessLog{}}
}

func (r *JSONRepository) load() (storageFile, error) {
	info, err := os.Lstat(r.Path)
	if err != nil {
		return storageFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return storageFile{}, fmt.Errorf("%w: storage path must be a regular file, not a symlink", ErrStorage)
	}
	if err := ensurePrivateFile(r.Path, info); err != nil {
		return storageFile{}, err
	}
	payload, err := os.ReadFile(r.Path)
	if err != nil {
		return storageFile{}, fmt.Errorf("%w: read %s: %v", ErrStorage, r.Path, err)
	}
	var data storageFile
	if err := json.Unmarshal(payload, &data); err != nil {
		return storageFile{}, fmt.Errorf("%w: decode %s: %v", ErrStorage, r.Path, err)
	}
	if data.SchemaVersion != storageSchemaVersion {
		return storageFile{}, fmt.Errorf("%w: unsupported SSH storage schema %d", ErrStorage, data.SchemaVersion)
	}
	if data.Servers == nil {
		data.Servers = []Server{}
	}
	if data.AccessLogs == nil {
		data.AccessLogs = []storedAccessLog{}
	}
	return data, nil
}

func (r *JSONRepository) write(data storageFile) error {
	temporary, err := os.CreateTemp(filepath.Dir(r.Path), ".ssh-*.json")
	if err != nil {
		return fmt.Errorf("%w: create temporary storage: %v", ErrStorage, err)
	}
	tempPath := temporary.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	defer cleanup()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("%w: secure temporary storage: %v", ErrStorage, err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("%w: encode storage: %v", ErrStorage, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("%w: sync storage: %v", ErrStorage, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("%w: close storage: %v", ErrStorage, err)
	}
	if err := replaceStorageFile(tempPath, r.Path); err != nil {
		return fmt.Errorf("%w: publish storage: %v", ErrStorage, err)
	}
	return nil
}

func (r *JSONRepository) ensureDirectory() error {
	dir := filepath.Dir(r.Path)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("%w: create storage directory: %v", ErrStorage, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("%w: inspect storage directory: %v", ErrStorage, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: storage directory must be a directory, not a symlink", ErrStorage)
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("%w: secure storage directory: %v", ErrStorage, err)
		}
	}
	return nil
}

func (r *JSONRepository) lock(ctx context.Context) (func(), error) {
	lockPath := r.Path + ".lock"
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("%w: acquire storage lock: %v", ErrStorage, err)
		}
		if info, statErr := os.Lstat(lockPath); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%w: storage lock path is unsafe", ErrStorage)
			}
			if time.Since(info.ModTime()) > 30*time.Second {
				_ = os.Remove(lockPath)
				continue
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (r *JSONRepository) recoverPending(data *storageFile) bool {
	now := r.now()
	dirty := false
	for i := range data.AccessLogs {
		stored := &data.AccessLogs[i]
		entry := &stored.AccessLog
		if entry.Status != StatusPending {
			continue
		}
		if stored.OwnerPID > 0 && processAlive(stored.OwnerPID) {
			continue
		}
		duration := now.Sub(entry.StartedAt).Milliseconds()
		if duration < 0 {
			duration = 0
		}
		entry.EndedAt, entry.DurationMS, entry.Status = &now, &duration, StatusCanceled
		stored.OwnerPID = 0
		dirty = true
	}
	return dirty
}

func (r *JSONRepository) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func ensurePrivateFile(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("%w: secure storage file: %v", ErrStorage, err)
		}
	}
	return nil
}
