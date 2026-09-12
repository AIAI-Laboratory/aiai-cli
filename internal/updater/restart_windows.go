//go:build windows

package updater

import (
	"os"
	"os/exec"
)

func restartProcess() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
