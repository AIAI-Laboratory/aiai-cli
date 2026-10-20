package ssh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

type OpenSSHRunner struct {
	LookPath func(string) (string, error)
}

func BuildArgs(server Server) []string {
	args := []string{"-p", strconv.Itoa(server.Port)}
	if server.IdentityFile != "" {
		args = append(args, "-i", server.IdentityFile)
	}
	return append(args, "-l", server.Username, "--", server.Host)
}

func (r OpenSSHRunner) Run(ctx context.Context, args []string, streams Streams) (RunResult, error) {
	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("ssh")
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: install OpenSSH and ensure 'ssh' is available in PATH", ErrSSHMissing)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = streams.In, streams.Out, streams.Err
	err = cmd.Run()
	if err == nil {
		return RunResult{ExitCode: 0}, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return RunResult{}, ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return RunResult{ExitCode: exitErr.ExitCode()}, nil
	}
	return RunResult{}, fmt.Errorf("start OpenSSH: %w", err)
}
