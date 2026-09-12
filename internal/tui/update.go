package tui

import (
	"context"
	"errors"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cancelMsg:
		return m.stop()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case plannedMsg:
		if m.canceling {
			m.err = context.Canceled
			return m, tea.Quit
		}
		if msg.err != nil {
			m.formError = msg.err.Error()
			m.screen = form
			return m, m.setFocus(m.focus)
		}
		m.plan = &msg.plan
		m.screen = preview
		m.viewport.GotoTop()
	case appliedMsg:
		m.result, m.err = &msg.result, msg.err
		m.screen = result
		m.viewport.GotoTop()
		if m.canceling || errors.Is(msg.err, context.Canceled) {
			m.err = context.Canceled
			return m, tea.Quit
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.stop()
		}
		if m.screen == planning || m.screen == applying {
			return m, nil
		}
		return m.key(msg)
	}
	if m.screen == form && m.focus < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}
	if m.screen == home {
		return m.updateCommand(msg)
	}
	return m, nil
}

func (m Model) stop() (tea.Model, tea.Cmd) {
	m.cancel()
	m.canceling = true
	if m.screen == applying || m.screen == planning {
		return m, nil
	}
	m.err = context.Canceled
	return m, tea.Quit
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

func (m Model) updateCommand(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.command.Value()
	var cmd tea.Cmd
	m.command, cmd = m.command.Update(msg)
	if before != m.command.Value() {
		m.cursor = 0
		m.viewport.GotoTop()
	}
	return m, cmd
}
