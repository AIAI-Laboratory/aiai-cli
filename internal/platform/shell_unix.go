//go:build !windows

package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func shellCommand(ctx context.Context, interactive bool) (*exec.Cmd, error) {
	shell := os.Getenv("SHELL")
	script := `OLDPWD="$AIAI_PREVIOUS_DIR"
export OLDPWD
trap 'command pwd -P > "$AIAI_CWD_FILE"' EXIT
eval "$AIAI_COMMAND"`
	if filepath.Base(shell) == "fish" {
		script = `set -gx OLDPWD "$AIAI_PREVIOUS_DIR"
function aiai_save_cwd --on-event fish_exit
pwd -P > "$AIAI_CWD_FILE"
end
eval "$AIAI_COMMAND"`
	} else {
		switch filepath.Base(shell) {
		case "bash", "zsh", "sh", "dash", "ksh":
		default:
			shell = "/bin/sh"
		}
	}
	flag := "-c"
	if interactive {
		flag = "-ic"
	}
	return exec.CommandContext(ctx, shell, flag, script), nil
}

func configureCapturedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
}
