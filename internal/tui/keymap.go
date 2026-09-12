package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
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
			if m.command.Value() != "" {
				m.command.SetValue("")
				m.cursor = 0
				m.viewport.GotoTop()
				return m, nil
			}
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
			m.screen, m.cursor = home, 0
		case authStarting, authWaiting, authResult, profile, logoutConfirmation:
			m.screen, m.cursor = home, 0
		case result:
			return m, tea.Quit
		}
		m.viewport.GotoTop()
		return m, nil
	}
	switch m.screen {
	case home:
		return m.homeKey(msg)
	case selection:
		count := len(m.templates)
		if count == 0 {
			return m, nil
		}
		switch key {
		case "up", "k":
			m.cursor = (m.cursor + count - 1) % count
		case "down", "j":
			m.cursor = (m.cursor + 1) % count
		case "enter":
			m.opts.Request.TemplateID = m.templates[m.cursor].ID
			m.screen = form
			return m, m.setFocus(0)
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
	case authResult, profile:
		if key == "enter" {
			m.screen, m.cursor = home, 0
		}
	case logoutConfirmation:
		switch key {
		case "up", "down", "j", "k", "tab":
			m.logoutYes = !m.logoutYes
		case "enter":
			if !m.logoutYes {
				m.screen = home
				return m, nil
			}
			m.screen = loggingOut
			return m, func() tea.Msg {
				result, err := m.auth.Logout(m.ctx)
				return authLogoutMsg{result: result, err: err}
			}
		}
	}
	if m.screen == preview || m.screen == result || m.screen == help || m.screen == about || m.screen == authWaiting || m.screen == authResult || m.screen == profile {
		// Synchronize content before scrolling, since View has a value receiver.
		m.resize()
		m.viewport.SetContent(m.content())
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) homeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	matches := screens.MatchingCommands(m.command.Value())
	key := msg.String()
	// Keep j/k shortcuts until the user starts typing a command filter.
	if m.command.Value() == "" {
		if key == "j" {
			key = "down"
		}
		if key == "k" {
			key = "up"
		}
	}
	switch key {
	case "up", "down":
		if len(matches) > 0 {
			delta := 1
			if key == "up" {
				delta = -1
			}
			m.cursor = (m.cursor + delta + len(matches)) % len(matches)
		}
		return m, nil
	case "tab":
		if len(matches) > 0 {
			m.command.SetValue(screens.HomeItems[matches[m.cursor]].Name)
			m.command.CursorEnd()
			m.cursor = 0
		}
		return m, nil
	case "enter":
		if len(matches) == 0 {
			return m, nil
		}
		selected := matches[m.cursor]
		m.command.SetValue("")
		m.cursor = 0
		m.viewport.GotoTop()
		switch screens.HomeItems[selected].Name {
		case "/init":
			m.screen = selection
		case "/login":
			if m.auth == nil {
				m.authText, m.authOK, m.screen = "Authentication is unavailable.", false, authResult
				break
			}
			m.screen = authStarting
			return m, func() tea.Msg {
				start, err := m.auth.StartLogin(m.ctx, auth.LoginOptions{})
				return authStartedMsg{start: start, err: err}
			}
		case "/whoami":
			if m.auth == nil {
				m.authText, m.authOK, m.screen = "Authentication is unavailable.", false, authResult
				break
			}
			m.screen = authStarting
			return m, func() tea.Msg {
				result, err := m.auth.WhoAmI(m.ctx)
				return authWhoamiMsg{result: result, err: err}
			}
		case "/logout":
			if m.authUser == nil {
				m.authText, m.authOK, m.screen = "Already signed out.", true, authResult
				break
			}
			m.logoutYes, m.screen = false, logoutConfirmation
		case "/help":
			m.previous, m.screen = home, help
		case "/about":
			m.screen = about
		case "/quit":
			return m, tea.Quit
		}
		return m, nil
	default:
		return m.updateCommand(msg)
	}
}
