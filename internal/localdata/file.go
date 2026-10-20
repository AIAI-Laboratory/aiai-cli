package localdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// File serializes versioned configuration updates with an OS-held lock. A dead
// process releases its lock automatically; a live writer's lock is never stolen.
type File[T any] struct {
	Path     string
	Empty    func() T
	Validate func(T) error
}

func Path(name string) (string, error) {
	base := os.Getenv("AIAI_CONFIG_DIR")
	if base == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(dir, "aiai")
	}
	abs, err := filepath.Abs(base)
	return filepath.Join(abs, name), err
}

func (f File[T]) Read(ctx context.Context) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	info, err := os.Lstat(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return f.Empty(), nil
	}
	var zero T
	if err != nil {
		return zero, err
	}
	if !info.Mode().IsRegular() {
		return zero, fmt.Errorf("storage must be a regular file: %s", f.Path)
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return zero, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return zero, fmt.Errorf("storage changed while opening: %s", f.Path)
	}
	payload, err := io.ReadAll(io.LimitReader(file, 32<<20+1))
	if err != nil {
		return zero, err
	}
	if len(payload) > 32<<20 {
		return zero, fmt.Errorf("storage exceeds 32 MiB; archive history before retrying: %s", f.Path)
	}
	var data T
	if err := json.Unmarshal(payload, &data); err != nil {
		return zero, fmt.Errorf("corrupt JSON in %s (preserved): %w", f.Path, err)
	}
	if err := f.Validate(data); err != nil {
		return zero, fmt.Errorf("invalid storage %s (preserved): %w", f.Path, err)
	}
	return data, nil
}

func (f File[T]) Update(ctx context.Context, mutate func(*T) error) error {
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe storage directory: %s", dir)
	}
	lockPath := f.Path + ".lock"
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("unsafe storage lock: %s", lockPath)
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	opened, err := lock.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(lockPath)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return fmt.Errorf("storage lock changed while opening: %s", lockPath)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, err := tryLock(lock)
		if err != nil {
			return err
		}
		if ok {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer unlock(lock)
	data, err := f.Read(ctx)
	if err != nil {
		return err
	}
	if err := mutate(&data); err != nil {
		return err
	}
	if err := f.Validate(data); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".aiai-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replace(tmp.Name(), f.Path)
}
