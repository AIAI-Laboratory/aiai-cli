//go:build !windows

package updater

import (
	"os"
	"syscall"
)

func restartProcess() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(execPath, os.Args, os.Environ())
}
