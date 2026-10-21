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
	kind, target, err, title, description, command, warning string
	args                                                    []string
	cursor                                                  int
	offset                                                  int
	filtering                                               bool
	category                                                string
	parentCursor                                            int
	origin                                                  *gitMenuPosition
}

type gitWriteDone struct {
	id     int
	notice string
	err    error
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

func (m Model) dialogHasInput() bool {
	return m.dialog.kind == "commit" || m.dialog.kind == "create" || m.dialog.kind == "native" || m.dialog.kind == "native-confirm" && m.dialog.warning != "" || m.dialog.kind == "menu" && m.dialog.filtering
}

func (m Model) matchingActions() []int {
	indices := []int{}
	query := strings.ToLower(m.gitInput.Value())
	for i, action := range gitActions {
		if query == "" && m.dialog.category != "" && action.category != m.dialog.category {
			continue
		}
		if strings.Contains(strings.ToLower(actionTitle(action)+" "+action.description+" "+action.command), query) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m Model) openNativeAction(action gitAction) (tea.Model, tea.Cmd) {
	m.notice = ""
	m.dialog = gitDialog{kind: "native", title: actionTitle(action), description: action.description}
	m.gitInput.Placeholder = "Git command, for example pull --rebase"
	m.gitInput.SetValue(action.command)
	m.gitInput.CursorEnd()
	m.gitInput.Focus()
	return m, textinput.Blink
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
	case "p":
		next, cmd := m.openNativeAction(gitAction{label: "Push", command: "push", description: "Push using the configured upstream."})
		return next, cmd, true
	case "P", "shift+p":
		next, cmd := m.openNativeAction(gitAction{label: "Pull", command: "pull --ff-only", description: "Update from upstream; stop if histories diverge."})
		return next, cmd, true
	case "f":
		next, cmd := m.openNativeAction(gitAction{label: "Fetch", command: "fetch --all --prune", description: "Refresh remote tracking branches."})
		return next, cmd, true
	case ":":
		next, cmd := m.openNativeAction(gitAction{label: "Run Git command", description: "Enter Git arguments; quotes work without shell expansion."})
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
		if m.dialog.kind == "native-confirm" {
			switch key.String() {
			case "up", "pgup":
				m.dialog.offset = max(0, m.dialog.offset-1)
				return m, nil
			case "down", "pgdown":
				m.dialog.offset++
				return m, nil
			}
		}
		switch key.String() {
		case "esc":
			if m.dialog.kind == "menu" && m.dialog.filtering {
				m.dialog.filtering = false
				m.dialog.cursor = 0
				m.gitInput.SetValue("")
				m.gitInput.Blur()
				return m, nil
			}
			if m.dialog.kind == "menu" {
				return m.gitMenuBack(), nil
			}
			if m.dialog.kind == "native-confirm" {
				m.dialog.kind, m.dialog.err = "native", ""
				m.gitInput.Placeholder = "Git command, for example pull --rebase"
				m.gitInput.SetValue(m.dialog.command)
				m.gitInput.CursorEnd()
				m.gitInput.Focus()
				return m, textinput.Blink
			}
			return m.returnToGitMenu(), nil
		case "enter":
			if m.loading {
				return m, nil
			}
			if m.dialog.kind == "menu" {
				if m.dialog.filtering {
					m.dialog.filtering = false
					m.gitInput.Blur()
					return m, nil
				}
				items := m.gitMenuItems()
				if len(items) == 0 {
					return m, nil
				}
				choice := items[min(m.dialog.cursor, len(items)-1)]
				if choice.category != "" {
					m.dialog.category = choice.category
					m.dialog.parentCursor, m.dialog.cursor = m.dialog.cursor, 0
					return m, nil
				}
				return m.openMenuAction(gitActions[choice.action])
			}
			if m.dialog.kind == "native" {
				args, err := gitlocal.ParseCommand(m.gitInput.Value())
				if err != nil {
					m.dialog.err = err.Error()
					return m, nil
				}
				m.dialog.command = m.gitInput.Value()
				m.dialog.kind, m.dialog.args, m.dialog.err = "native-confirm", args, ""
				m.dialog.offset = 0
				m.dialog.warning = commandWarning(args)
				m.gitInput.SetValue("")
				if m.dialog.warning != "" {
					m.gitInput.Placeholder = "Type confirm to continue"
					return m, textinput.Blink
				}
				m.gitInput.Blur()
				return m, nil
			}
			if m.dialog.kind == "native-confirm" {
				if m.dialog.warning != "" && m.gitInput.Value() != "confirm" {
					m.dialog.err = "Type confirm to run this operation."
					return m, nil
				}
				return m.prepareGitCommand()
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
			if m.dialog.filtering {
				var cmd tea.Cmd
				m.gitInput, cmd = m.gitInput.Update(msg)
				m.dialog.cursor = 0
				return m, cmd
			}
			last := max(0, len(m.gitMenuItems())-1)
			switch key.String() {
			case "up", "k":
				m.dialog.cursor = max(0, m.dialog.cursor-1)
			case "down", "j":
				m.dialog.cursor = min(last, m.dialog.cursor+1)
			case "pgup":
				m.dialog.cursor = max(0, m.dialog.cursor-max(1, m.bodyHeight()-2))
			case "pgdown":
				m.dialog.cursor = min(last, m.dialog.cursor+max(1, m.bodyHeight()-2))
			case "home":
				m.dialog.cursor = 0
			case "end":
				m.dialog.cursor = last
			case "left":
				return m.gitMenuBack(), nil
			case "/":
				m.dialog.filtering = true
				m.gitInput.Placeholder = "Find Git action"
				m.gitInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
	}
	if m.dialogHasInput() {
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
		items := m.gitMenuItems()
		rows = append(rows, m.title(m.gitMenuTitle(len(items))))
		if m.dialog.filtering {
			rows = append(rows, m.gitInput.View())
		} else {
			rows = append(rows, m.muted("/ search all actions  "+m.gitInput.Value()))
		}
		capacity := max(1, m.bodyHeight()-2)
		start := max(0, m.dialog.cursor-capacity+1)
		for i := start; i < min(len(items), start+capacity); i++ {
			label := items[i].label
			if i == m.dialog.cursor {
				rows = append(rows, m.accent("> "+label))
			} else {
				rows = append(rows, "  "+label)
			}
		}
		if len(items) == 0 {
			rows = append(rows, "No actions match this search.")
		}
		footer = "↑↓ · enter open · / find · esc cancel"
		if m.dialog.category != "" || m.gitInput.Value() != "" {
			footer = "↑↓ · enter open · / find · esc back"
		}
	case "commit":
		rows = append(rows, m.title("Commit staged changes"), m.muted(fmt.Sprintf("%s · %d staged files", oneLine(m.status.Branch), m.status.Staged)), "", m.gitInput.View())
		footer = "enter commit · esc cancel"
	case "create":
		rows = append(rows, m.title("Create and switch branch"), m.muted("From "+oneLine(m.status.Branch)+" · "+shortHead(m.status.Head)), "", m.gitInput.View())
		footer = "enter create · esc cancel"
	case "switch":
		rows = append(rows, m.title("Switch branch"), m.muted(oneLine(m.status.Branch)+" → ")+m.title(oneLine(m.dialog.target)), "", fmt.Sprintf("%d staged · %d unstaged · %d untracked", m.status.Staged, m.status.Unstaged, m.status.Untracked), m.muted("Git will stop if local changes would be overwritten."))
		footer = "enter switch · esc cancel"
	case "native":
		rows = append(rows, m.title(m.dialog.title), m.muted(oneLine(m.status.Branch)+" · "+oneLine(m.status.Upstream)), "", m.gitInput.View(), m.muted(m.dialog.description))
		footer = "enter review · esc cancel"
	case "native-confirm":
		command := strings.Split(ansi.Wrap("git "+gitlocal.FormatArguments(m.dialog.args), width, " "), "\n")
		capacity := m.bodyHeight() - 2
		if m.dialog.warning != "" {
			capacity -= 2
		}
		if m.dialog.err != "" {
			capacity--
		}
		capacity = max(1, capacity)
		start := min(m.dialog.offset, max(0, len(command)-capacity))
		end := min(len(command), start+capacity)
		rows = append(rows, m.title(fmt.Sprintf("Run Git command  %d–%d/%d", start+1, end, len(command))), m.muted(pathTail(m.status.Root, width)))
		rows = append(rows, command[start:end]...)
		if m.dialog.warning != "" {
			rows = append(rows, m.dialog.warning, m.gitInput.View())
		}
		footer = "↑↓ scroll · enter run · esc edit"
	}
	if m.dialog.err != "" {
		rows = append(rows, oneLine(m.dialog.err))
	}
	for i := range rows {
		if m.opts.NoColor {
			rows[i] = ansi.Strip(rows[i])
		}
		rows[i] = ansi.Truncate(rows[i], width, "…")
	}
	return strings.Join(rows, "\n"), footer
}
