package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolveTarget canonicalizes the existing ancestor (including OS aliases such
// as /tmp on macOS), without creating anything during planning.
func ResolveTarget(target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	ancestor := abs
	var suffix []string
	for {
		info, err := os.Lstat(ancestor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 && ancestor == abs {
				return "", fmt.Errorf("target is a symlink: %s", abs)
			}
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			info, err = os.Stat(resolved)
			if err != nil {
				return "", err
			}
			if !info.IsDir() {
				return "", fmt.Errorf("target ancestor is not a directory: %s", ancestor)
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			if filepath.Dir(resolved) == resolved {
				return "", fmt.Errorf("cannot initialize a filesystem root")
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
}

func ValidRelative(path string) bool {
	return fs.ValidPath(path) && path != "." && !strings.ContainsAny(path, `\:`)
}

// CheckPath rejects symlinks and non-directory ancestors even when their current
// destination is within the root. Root APIs additionally enforce containment.
func CheckPath(root *os.Root, path string) error {
	if !ValidRelative(path) {
		return fmt.Errorf("unsafe template path %q", path)
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink destination refused: %s", prefix)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("parent is not a directory: %s", prefix)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("destination is not a regular file: %s", prefix)
		}
	}
	return nil
}

// OpenTarget uses a directory handle anchored at the nearest existing ancestor
// to create the target without escaping through newly introduced symlinks.
func OpenTarget(target string) (*os.Root, error) {
	ancestor := target
	for {
		_, err := os.Stat(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		ancestor = filepath.Dir(ancestor)
	}
	r, err := os.OpenRoot(ancestor)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	rel, err := filepath.Rel(ancestor, target)
	if err != nil {
		return nil, err
	}
	if err := r.MkdirAll(rel, 0o755); err != nil {
		return nil, err
	}
	return r.OpenRoot(rel)
}
