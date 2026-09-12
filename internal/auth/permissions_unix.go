//go:build !windows

package auth

import (
	"fmt"
	"io/fs"
	"os"
)

func validateCredentialFile(_ string, info fs.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: credential file permissions must be 0600", ErrIO)
	}
	return nil
}

func secureCredentialPath(path string, directory bool) error {
	mode := fs.FileMode(0o600)
	label := "file"
	if directory {
		mode, label = 0o700, "directory"
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("%w: secure credential %s: %v", ErrIO, label, err)
	}
	return nil
}
