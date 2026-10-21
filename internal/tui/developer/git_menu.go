package developer

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
)

var gitCategories = []string{
	"Changes", "Sync", "Branches", "Stash", "History", "Recovery",
	"Tags", "Remotes", "Worktrees", "Submodules", "Undo", "Advanced",
}

type gitMenuItem struct {
	label, category string
	action          int
}

type gitMenuPosition struct {
	category             string
	cursor, parentCursor int
	query                string
}

func actionTitle(action gitAction) string {
	if action.category != "" {
		return action.category + " / " + action.label
	}
	return action.label
}

func (m Model) gitMenuItems() []gitMenuItem {
	items := []gitMenuItem{}
	if m.dialog.category == "" && m.gitInput.Value() == "" {
		for _, category := range gitCategories {
			count := 0
			for _, action := range gitActions {
				if action.category == category {
					count++
				}
			}
			if count > 0 {
				items = append(items, gitMenuItem{label: fmt.Sprintf("%s  (%d)", category, count), category: category, action: -1})
			}
		}
		return items
	}
	for _, index := range m.matchingActions() {
		action := gitActions[index]
		label := action.label
		if m.gitInput.Value() != "" {
			label = actionTitle(action)
		}
		items = append(items, gitMenuItem{label: label, action: index})
	}
	return items
}

func (m Model) gitMenuBack() Model {
	m.gitInput.Blur()
	m.dialog.filtering = false
	if m.gitInput.Value() != "" {
		m.gitInput.SetValue("")
		m.dialog.cursor = 0
		return m
	}
	if m.dialog.category != "" {
		m.dialog.category = ""
		m.dialog.cursor = m.dialog.parentCursor
		return m
	}
	return m.closeGitDialog()
}

func (m Model) openMenuAction(action gitAction) (tea.Model, tea.Cmd) {
	origin := &gitMenuPosition{category: m.dialog.category, cursor: m.dialog.cursor, parentCursor: m.dialog.parentCursor, query: m.gitInput.Value()}
	m = m.closeGitDialog()
	next, cmd := m.activateGitAction(action)
	model := next.(Model)
	if model.dialog.kind != "" {
		model.dialog.origin = origin
	}
	return model, cmd
}

func (m Model) activateGitAction(action gitAction) (tea.Model, tea.Cmd) {
	switch action.kind {
	case "commit":
		return m.openGitDialog("commit", "")
	case "branches":
		m.opts.Page, m.cursor, m.offset = 3, 0, 0
		m.search.SetValue("")
		m.preview = false
		return m.refresh()
	case "create":
		return m.openGitDialog("create", "")
	case "stage-all", "unstage-all":
		return m.startGitWrite(action.kind, "", gitlocal.Change{})
	default:
		return m.openNativeAction(action)
	}
}

func (m Model) returnToGitMenu() Model {
	if m.dialog.origin == nil {
		return m.closeGitDialog()
	}
	position := *m.dialog.origin
	m.gitInput.Blur()
	m.gitInput.SetValue(position.query)
	m.dialog = gitDialog{kind: "menu", category: position.category, cursor: position.cursor, parentCursor: position.parentCursor}
	return m
}

func (m Model) gitMenuTitle(count int) string {
	location := "Git actions"
	if m.dialog.category != "" {
		location += " / " + m.dialog.category
	}
	if strings.TrimSpace(m.gitInput.Value()) != "" {
		location = "Git / Search all actions"
	}
	return fmt.Sprintf("%s  %d / %d", location, min(m.dialog.cursor+1, count), count)
}
