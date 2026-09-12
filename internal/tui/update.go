package tui

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
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
	case authCachedMsg:
		if msg.err == nil && msg.result.Authenticated {
			user := msg.result.User
			m.authUser, m.authStore = &user, msg.result.Storage
		}
	case authStartedMsg:
		if m.screen != authStarting {
			return m, nil
		}
		if msg.err != nil {
			return m.authFailure(msg.err)
		}
		if msg.start.Existing != nil {
			m.setAuthResult(*msg.start.Existing)
			m.screen = profile
			return m, nil
		}
		device := msg.start.Device
		m.authDevice = &device
		m.screen = authWaiting
		return m, scheduleAuthPoll(device.IntervalSeconds)
	case authPollTickMsg:
		if m.screen != authWaiting || m.authDevice == nil {
			return m, nil
		}
		attempt := *m.authDevice
		return m, func() tea.Msg {
			poll, err := m.auth.PollLogin(m.ctx, attempt)
			var result auth.Result
			if err == nil && !poll.Pending {
				result, err = m.auth.Cached()
			}
			return authPollMsg{poll: poll, result: result, err: err}
		}
	case authPollMsg:
		if m.screen != authWaiting {
			return m, nil
		}
		if msg.err != nil {
			return m.authFailure(msg.err)
		}
		if msg.poll.Pending {
			seconds := msg.poll.RetryAfterSeconds
			if seconds < 1 && m.authDevice != nil {
				seconds = m.authDevice.IntervalSeconds
			}
			return m, scheduleAuthPoll(seconds)
		}
		m.setAuthResult(msg.result)
		m.authText = "Signed in as @" + msg.result.User.Login + "."
		m.authOK, m.screen = true, authResult
	case authWhoamiMsg:
		if m.screen != authStarting {
			return m, nil
		}
		if msg.err != nil {
			return m.authFailure(msg.err)
		}
		m.setAuthResult(msg.result)
		m.screen = profile
	case authLogoutMsg:
		if m.screen != loggingOut {
			return m, nil
		}
		m.authUser, m.authDevice, m.authStore = nil, nil, ""
		if m.canceling {
			m.err = context.Canceled
			return m, tea.Quit
		}
		m.authText, m.authWarn, m.authOK = "Signed out.", msg.result.Warning, msg.err == nil
		if msg.err != nil && msg.result.Warning == "" {
			m.authText = msg.err.Error()
		}
		m.screen = authResult
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.stop()
		}
		if m.screen == planning || m.screen == applying || m.screen == loggingOut {
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

func (m Model) authFailure(err error) (tea.Model, tea.Cmd) {
	m.authText, m.authWarn, m.authOK = err.Error(), "", false
	if errors.Is(err, auth.ErrUnauthenticated) {
		m.authText = "Not signed in. Run /login to continue."
		m.authUser, m.authStore = nil, ""
	}
	m.screen = authResult
	return m, nil
}

func (m *Model) setAuthResult(result auth.Result) {
	if result.Authenticated {
		user := result.User
		m.authUser = &user
	}
	m.authStore, m.authWarn = result.Storage, result.Warning
}

func scheduleAuthPoll(seconds int) tea.Cmd {
	if seconds < 1 {
		seconds = 1
	}
	return tea.Tick(time.Duration(seconds)*time.Second, func(time.Time) tea.Msg { return authPollTickMsg{} })
}

func (m Model) stop() (tea.Model, tea.Cmd) {
	m.cancel()
	m.canceling = true
	if m.screen == applying || m.screen == planning || m.screen == loggingOut {
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
