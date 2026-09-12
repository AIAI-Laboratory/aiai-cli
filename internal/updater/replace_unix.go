//go:build !windows

package updater

import (
	"fmt"
	"os"
)

func replaceTargetBinary(newBinaryPath, targetPath string) error {
	siblingTemp := targetPath + ".new"
	if err := copyFile(newBinaryPath, siblingTemp, 0o755); err != nil {
		return fmt.Errorf("copy new binary to destination directory: %w", err)
	}

	if err := os.Rename(siblingTemp, targetPath); err != nil {
		_ = os.Remove(siblingTemp)
		return fmt.Errorf("replace existing binary (%s): %w", targetPath, err)
	}

	return nil
}
