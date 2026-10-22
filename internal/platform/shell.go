package platform

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// TerminalRunning lets the application leave Ctrl+C to a foreground command.
// The child still receives the terminal interrupt directly from the OS.
var TerminalRunning atomic.Bool

// ShellSession retains the working directory and in-memory command history
// while the user moves between AIAI screens.
type ShellSession struct {
	Dir, PreviousDir string
	History          []string
}

// ShellCommand owns terminal I/O while the user's shell executes a command.
type ShellCommand struct {
	Context context.Context
	Session *ShellSession
	Line    string
	Capture bool
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

func (s *ShellCommand) SetStdin(in io.Reader)   { s.In = in }
func (s *ShellCommand) SetStdout(out io.Writer) { s.Out = out }
func (s *ShellCommand) SetStderr(out io.Writer) { s.Err = out }

func (s *ShellCommand) Run() error {
	if !s.Capture {
		TerminalRunning.Store(true)
		defer TerminalRunning.Store(false)
	}
	directory, err := os.MkdirTemp("", "aiai-shell-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	name := filepath.Join(directory, "cwd")
	cmd, err := shellCommand(s.Context, !s.Capture)
	if err != nil {
		return err
	}
	cmd.Dir = s.Session.Dir
	cmd.Env = append(os.Environ(), "AIAI_COMMAND="+s.Line, "AIAI_CWD_FILE="+name, "PWD="+s.Session.Dir, "AIAI_PREVIOUS_DIR="+s.Session.PreviousDir)
	if s.Capture {
		cmd.Env = append(cmd.Env, "TERM=dumb", "NO_COLOR=1")
		configureCapturedCommand(cmd)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.In, s.Out, s.Err
	if !s.Capture {
		_, _ = fmt.Fprintf(s.Out, "\n%s > %s\n\n", s.Session.Dir, s.Line)
	}
	err = cmd.Run()
	if data, readErr := os.ReadFile(name); readErr == nil {
		dir := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() && filepath.IsAbs(dir) && dir != s.Session.Dir {
			s.Session.PreviousDir, s.Session.Dir = s.Session.Dir, dir
		}
	}
	if !s.Capture && s.Context.Err() == nil {
		if err != nil {
			_, _ = fmt.Fprintln(s.Out, "\nCommand returned:", err)
		}
		_, _ = fmt.Fprint(s.Out, "\nPress Enter to return to AIAI… ")
		_, _ = bufio.NewReader(s.In).ReadString('\n')
	}
	return err
}
