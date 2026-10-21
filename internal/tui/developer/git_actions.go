package developer

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
)

type gitDialog struct {
	kind, target, err string
	cursor            int
}

type gitWriteDone struct {
	id     int
	notice string
	err    error
}

var gitActions = []string{
	"Commit staged changes",
	"Switch branch",
	"Create and switch to a new branch",
	"Stage all changes",
	"Unstage all changes",
}

func (m Model) openGitDialog(kind, target string) (tea.Model, tea.Cmd) {
	m.notice = ""
	m.dialog = gitDialog{kind: kind, target: target}
	m.gitInput.SetValue("")
	if kind == "commit" || kind == "create" {
		m.gitInput.Placeholder = "Commit message"
		if kind == "create" {
			m.gitInput.Placeholder = "Branch name"
		}
		m.gitInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) closeGitDialog() Model {
	m.dialog = gitDialog{}
	m.gitInput.Blur()
	return m
}

func (m Model) startGitWrite(operation, target string, change gitlocal.Change) (tea.Model, tea.Cmd) {
	m.gitBusy = true
	m.writeID++
	m.notice = ""
	m.dialog.err = ""
	id, root, service, ctx := m.writeID, m.status.Root, m.opts.Git, m.ctx
	branch, head := m.status.Branch, m.status.Head
	message := m.gitInput.Value()
	return m, func() tea.Msg {
		var err error
		notice := ""
		// A form must not commit or create a branch against a checkout that
		// changed externally while the user was typing.
		current, err := service.Status(ctx, root)
		if err == nil && (current.Branch != branch || current.Head != head) {
			err = fmt.Errorf("repository HEAD changed; cancel and refresh before retrying")
		}
		if err == nil {
			switch operation {
			case "stage":
				err = service.Stage(ctx, root, change)
				notice = "Staged " + pathLabel(change.Path)
			case "unstage":
				err = service.Unstage(ctx, root, change)
				notice = "Unstaged " + pathLabel(change.Path)
			case "stage-all":
				err = service.StageAll(ctx, root)
				notice = "Staged all changes"
			case "unstage-all":
				err = service.UnstageAll(ctx, root)
				notice = "Unstaged all changes"
			case "commit":
				err = service.CommitStaged(ctx, root, message)
				notice = "Committed staged changes"
			case "switch":
				err = service.SwitchBranch(ctx, root, target)
				notice = "Switched to " + target
			case "create":
				target = strings.TrimSpace(message)
				err = service.CreateBranch(ctx, root, target)
				notice = "Created and switched to " + target
			}
		}
		return gitWriteDone{id: id, notice: notice, err: err}
	}
}

func (m Model) gitShortcut(key string) (tea.Model, tea.Cmd, bool) {
	if m.opts.Mode != "git" || m.opts.Git == nil || m.status.Root == "" || m.err != nil {
		return m, nil, false
	}
	switch key {
	case "a":
		next, cmd := m.openGitDialog("menu", "")
		return next, cmd, true
	case "c":
		next, cmd := m.openGitDialog("commit", "")
		return next, cmd, true
	case "n":
		next, cmd := m.openGitDialog("create", "")
		return next, cmd, true
	case "b":
		m.opts.Page, m.cursor, m.offset = 3, 0, 0
		m.preview = false
		m.search.SetValue("")
		next, cmd := m.refresh()
		return next, cmd, true
	case "A", "shift+a":
		next, cmd := m.startGitWrite("stage-all", "", gitlocal.Change{})
		return next, cmd, true
	case "U", "shift+u":
		next, cmd := m.startGitWrite("unstage-all", "", gitlocal.Change{})
		return next, cmd, true
	case "space", " ":
		indices := m.visible()
		if m.opts.Page != 1 || len(indices) == 0 {
			return m, nil, false
		}
		index := indices[min(m.cursor, len(indices)-1)]
		if index >= len(m.status.Changes) {
			return m, nil, true
		}
		change := m.status.Changes[index]
		operation := "stage"
		if m.staged || !change.Untracked && change.Worktree == "." {
			operation = "unstage"
		}
		next, cmd := m.startGitWrite(operation, "", change)
		return next, cmd, true
	case "enter":
		indices := m.visible()
		if m.opts.Page != 3 || len(indices) == 0 {
			return m, nil, false
		}
		index := indices[min(m.cursor, len(indices)-1)]
		if index >= len(m.branches) || m.branches[index].Current {
			return m, nil, true
		}
		next, cmd := m.openGitDialog("switch", m.branches[index].Name)
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) updateGitDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			return m.closeGitDialog(), nil
		case "enter":
			if m.loading {
				return m, nil
			}
			if m.dialog.kind == "menu" {
				choice := m.dialog.cursor
				m = m.closeGitDialog()
				switch choice {
				case 0:
					return m.openGitDialog("commit", "")
				case 1:
					m.opts.Page, m.cursor, m.offset = 3, 0, 0
					m.search.SetValue("")
					m.preview = false
					return m.refresh()
				case 2:
					return m.openGitDialog("create", "")
				case 3:
					return m.startGitWrite("stage-all", "", gitlocal.Change{})
				case 4:
					return m.startGitWrite("unstage-all", "", gitlocal.Change{})
				}
			}
			if m.dialog.kind == "commit" && m.status.Staged == 0 {
				m.dialog.err = "No staged files. Cancel and use Space or A in Changes."
				return m, nil
			}
			if m.dialog.kind != "switch" && strings.TrimSpace(m.gitInput.Value()) == "" {
				m.dialog.err = "Enter a value before continuing."
				return m, nil
			}
			return m.startGitWrite(m.dialog.kind, m.dialog.target, gitlocal.Change{})
		}
		if m.dialog.kind == "menu" {
			switch key.String() {
			case "up", "k":
				m.dialog.cursor = max(0, m.dialog.cursor-1)
			case "down", "j":
				m.dialog.cursor = min(len(gitActions)-1, m.dialog.cursor+1)
			}
			return m, nil
		}
	}
	if m.dialog.kind == "commit" || m.dialog.kind == "create" {
		var cmd tea.Cmd
		m.gitInput, cmd = m.gitInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) gitDialogView(width int) (string, string) {
	rows := []string{}
	footer := "enter confirm · esc cancel"
	switch m.dialog.kind {
	case "menu":
		rows = append(rows, m.title("Git actions"), "")
		for i, label := range gitActions {
			if i == m.dialog.cursor {
				rows = append(rows, m.accent("> "+label))
			} else {
				rows = append(rows, "  "+label)
			}
		}
		footer = "↑↓ select · enter open · esc cancel"
	case "commit":
		rows = append(rows, m.title("Commit staged changes"), m.muted(fmt.Sprintf("%s · %d staged files", oneLine(m.status.Branch), m.status.Staged)), "", m.gitInput.View())
		footer = "enter commit · esc cancel"
	case "create":
		rows = append(rows, m.title("Create and switch branch"), m.muted("From "+oneLine(m.status.Branch)+" · "+shortHead(m.status.Head)), "", m.gitInput.View())
		footer = "enter create · esc cancel"
	case "switch":
		rows = append(rows, m.title("Switch branch"), m.muted(oneLine(m.status.Branch)+" → ")+m.title(oneLine(m.dialog.target)), "", fmt.Sprintf("%d staged · %d unstaged · %d untracked", m.status.Staged, m.status.Unstaged, m.status.Untracked), m.muted("Git will stop if local changes would be overwritten."))
		footer = "enter switch · esc cancel"
	}
	if m.dialog.err != "" {
		rows = append(rows, "", oneLine(m.dialog.err))
	}
	for i := range rows {
		if m.opts.NoColor {
			rows[i] = ansi.Strip(rows[i])
		}
		rows[i] = ansi.Truncate(rows[i], width, "…")
	}
	return strings.Join(rows, "\n"), footer
}
