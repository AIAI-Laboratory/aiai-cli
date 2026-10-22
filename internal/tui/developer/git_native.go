package developer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

type gitNativeReady struct {
	id   int
	cmd  *exec.Cmd
	args []string
	err  error
}

// A command owns the real terminal while Git prompts for credentials, opens an
// editor or pager, or shows conflicts. Pause before restoring the TUI so output
// remains readable even when a command fails immediately.
type gitTerminalProcess struct {
	ctx  context.Context
	cmd  *exec.Cmd
	args []string
}

func (p *gitTerminalProcess) SetStdin(in io.Reader)   { p.cmd.Stdin = in }
func (p *gitTerminalProcess) SetStdout(out io.Writer) { p.cmd.Stdout = out }
func (p *gitTerminalProcess) SetStderr(out io.Writer) { p.cmd.Stderr = out }
func (p *gitTerminalProcess) Run() error {
	platform.TerminalRunning.Store(true)
	defer platform.TerminalRunning.Store(false)
	_, _ = fmt.Fprintf(p.cmd.Stdout, "\ngit %s\n\n", gitlocal.FormatArguments(p.args))
	err := p.cmd.Run()
	if p.ctx.Err() == nil {
		if err != nil {
			_, _ = fmt.Fprintln(p.cmd.Stdout, "\nGit returned:", err)
		}
		_, _ = fmt.Fprint(p.cmd.Stdout, "\nPress Enter to return to AIAI… ")
		_, _ = bufio.NewReader(p.cmd.Stdin).ReadString('\n')
	}
	return err
}

func (m Model) prepareGitCommand() (tea.Model, tea.Cmd) {
	m.gitBusy = true
	m.writeID++
	m.dialog.err = ""
	id, ctx, service, root := m.writeID, m.ctx, m.opts.Git, m.status.Root
	branch, head := m.status.Branch, m.status.Head
	args := append([]string{}, m.dialog.args...)
	return m, func() tea.Msg {
		current, err := service.Status(ctx, root)
		if err == nil && (current.Head != head || current.Branch != branch) {
			err = fmt.Errorf("repository HEAD changed; edit and review the command again")
		}
		var cmd *exec.Cmd
		if err == nil {
			cmd, err = service.InteractiveCommand(ctx, root, args)
		}
		return gitNativeReady{id: id, cmd: cmd, args: args, err: err}
	}
}

func (m Model) runGitCommand(msg gitNativeReady) (tea.Model, tea.Cmd) {
	if !m.gitBusy || msg.id != m.writeID {
		return m, nil
	}
	if msg.err != nil {
		return m.Update(gitWriteDone{id: msg.id, err: msg.err})
	}
	process := &gitTerminalProcess{ctx: m.ctx, cmd: msg.cmd, args: msg.args}
	return m, tea.Exec(process, func(err error) tea.Msg {
		if err != nil {
			err = fmt.Errorf("git command failed (%v); resolve the error shown in the terminal and retry", err)
		}
		return gitWriteDone{id: msg.id, notice: "Git command completed; repository refreshed", err: err}
	})
}

func commandWarning(args []string) string {
	if len(args) == 0 {
		return ""
	}
	command := args[0]
	for _, arg := range args {
		if strings.HasPrefix(arg, "--force=") || strings.HasPrefix(arg, "--force-with-lease=") {
			return "May overwrite files, refs or history. Type confirm."
		}
		if command == "commit" && arg == "--amend" {
			return "Rewrites the latest commit. Type confirm."
		}
		switch arg {
		case "--force", "-f", "--force-with-lease", "--mirror", "--hard", "-D", "--skip":
			return "May overwrite files, refs or history. Type confirm."
		case "--delete", "-d", "drop", "clear", "remove":
			if command != "clean" {
				return "Removes a saved reference or directory. Type confirm."
			}
		}
		if command == "push" && strings.HasPrefix(arg, "+") {
			return "Rewrites remote history. Type confirm."
		}
	}
	switch command {
	case "reset":
		return "Moves HEAD or changes the index and working tree. Type confirm."
	case "restore", "checkout":
		return "May replace working files or change the checkout. Type confirm."
	}
	return ""
}
