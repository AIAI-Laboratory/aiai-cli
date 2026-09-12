package tui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/screens"
)

func (m *Model) setFocus(focus int) tea.Cmd {
	m.focus = (focus + 6) % 6
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	if m.focus < len(m.inputs) {
		return m.inputs[m.focus].Focus()
	}
	return nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "?" {
		if m.screen == help {
			m.screen = m.previous
		} else {
			m.previous = m.screen
			m.screen = help
		}
		m.viewport.GotoTop()
		return m, nil
	}
	if key == "esc" {
		switch m.screen {
		case home:
			return m, tea.Quit
		case selection:
			m.screen, m.cursor = home, 0
		case form:
			m.screen, m.cursor = selection, 0
		case preview:
			m.screen = form
			return m, m.setFocus(m.focus)
		case confirmation:
			m.screen = preview
		case help:
			m.screen = m.previous
		case about:
			m.screen = home
		case result:
			return m, tea.Quit
		}
		m.viewport.GotoTop()
		return m, nil
	}
	switch m.screen {
	case home, selection:
		count := len(screens.HomeItems)
		if m.screen == selection {
			count = len(m.templates)
		}
		if count == 0 {
			return m, nil
		}
		switch key {
		case "up", "k":
			m.cursor = (m.cursor + count - 1) % count
		case "down", "j":
			m.cursor = (m.cursor + 1) % count
		case "enter":
			if m.screen == selection {
				m.opts.Request.TemplateID = m.templates[m.cursor].ID
				m.screen = form
				return m, m.setFocus(0)
			}
			switch m.cursor {
			case 0:
				m.screen, m.cursor = selection, 0
			case 1:
				m.previous, m.screen = home, help
			case 2:
				m.screen = about
			case 3:
				return m, tea.Quit
			}
		}
	case form:
		switch key {
		case "tab", "down":
			return m, m.setFocus(m.focus + 1)
		case "shift+tab", "up":
			return m, m.setFocus(m.focus - 1)
		case "enter":
			switch m.focus {
			case 0, 1, 2:
				return m, m.setFocus(m.focus + 1)
			case 3:
				m.opts.Request.Force = !m.opts.Request.Force
			case 4:
				m.opts.DryRun = !m.opts.DryRun
			case 5:
				return m.prepare()
			}
		case "space":
			if m.focus == 3 {
				m.opts.Request.Force = !m.opts.Request.Force
				return m, nil
			}
			if m.focus == 4 {
				m.opts.DryRun = !m.opts.DryRun
				return m, nil
			}
		}
		if m.focus < len(m.inputs) {
			var cmd tea.Cmd
			m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
			return m, cmd
		}
	case preview:
		if key == "enter" && !m.plan.HasConflicts() {
			if m.opts.DryRun {
				return m, tea.Quit
			}
			m.screen, m.confirmYes = confirmation, false
			return m, nil
		}
	case confirmation:
		switch key {
		case "up", "down", "j", "k", "tab":
			m.confirmYes = !m.confirmYes
		case "enter":
			if !m.confirmYes {
				m.screen = preview
				return m, nil
			}
			m.screen = applying
			plan := *m.plan
			return m, func() tea.Msg { r, err := m.executor.Apply(m.ctx, plan); return appliedMsg{r, err} }
		}
	case result:
		if key == "enter" {
			return m, tea.Quit
		}
	}
	if m.screen == preview || m.screen == result || m.screen == help || m.screen == about {
		// Synchronize content before scrolling, since View has a value receiver.
		m.viewport.SetContent(m.content())
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) prepare() (tea.Model, tea.Cmd) {
	req := m.opts.Request
	req.ProjectName, req.TargetDir, req.PackageName = m.inputs[0].Value(), m.inputs[1].Value(), m.inputs[2].Value()
	if req.TargetDir == "" && req.ProjectName != "" {
		name, err := project.NormalizeName(req.ProjectName)
		if err != nil {
			m.formError = err.Error()
			return m, nil
		}
		req.TargetDir = filepath.Join(".", name)
	}
	if req.TargetDir == "" {
		req.TargetDir = "."
	}
	m.formError = ""
	m.screen = planning
	return m, func() tea.Msg { p, err := m.planner.Plan(m.ctx, req); return plannedMsg{p, err} }
}
