package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReplaceExecutable replaces the currently running executable with the new binary at newBinaryPath.
func ReplaceExecutable(newBinaryPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running executable: %w", err)
	}

	// Resolve symlinks to target the real binary
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}

	return replaceTargetBinary(newBinaryPath, realPath)
}

func copyFile(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}
	return destFile.Sync()
}
