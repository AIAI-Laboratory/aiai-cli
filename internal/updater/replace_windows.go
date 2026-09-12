//go:build windows

package updater

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func replaceTargetBinary(newBinaryPath, targetPath string) error {
	siblingTemp := targetPath + ".new"
	if err := copyFile(newBinaryPath, siblingTemp, 0o755); err != nil {
		return fmt.Errorf("copy new binary to destination directory: %w", err)
	}

	oldPath := targetPath + ".old"
	_ = os.Remove(oldPath) // remove previous leftover .old if any

	// Rename running binary to .old
	if err := os.Rename(targetPath, oldPath); err != nil {
		_ = os.Remove(siblingTemp)
		return fmt.Errorf("rename running binary to .old: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(siblingTemp, targetPath); err != nil {
		// Attempt rollback
		_ = os.Rename(oldPath, targetPath)
		_ = os.Remove(siblingTemp)
		return fmt.Errorf("move new binary into place: %w", err)
	}

	// Mark .old file for deletion on reboot if it cannot be removed now
	if err := os.Remove(oldPath); err != nil {
		oldPtr, _ := windows.UTF16PtrFromString(oldPath)
		_ = windows.MoveFileEx(oldPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}

	return nil
}
