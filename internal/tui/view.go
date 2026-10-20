package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/screens"
)

func (m Model) View() tea.View {
	if m.width < 24 || m.height < 12 {
		v := tea.NewView(lipgloss.NewStyle().Width(max(1, m.width)).MaxHeight(max(1, m.height)).Render("AIAI\nEnlarge terminal\nCtrl+C to exit"))
		v.AltScreen = true
		return v
	}
	m.resize()
	content := m.content()
	m.viewport.SetContent(content)
	if m.screen == form || m.screen == home || m.screen == selection || m.screen == confirmation || m.screen == updatePrompt || m.screen == updateResult || m.screen == serverList || m.screen == keyList || m.screen == keyDeleteConfirmation {
		for line, text := range strings.Split(content, "\n") {
			if strings.Contains(text, "> ") {
				m.viewport.EnsureVisible(line, 0, 1)
				if m.screen == form && m.focus < 3 {
					m.viewport.EnsureVisible(line+1, 0, 1)
				}
				break
			}
		}
	}
	top, bottom := m.chrome()
	view := top + "\n" + m.viewport.View() + "\n" + bottom
	v := tea.NewView(lipgloss.NewStyle().PaddingLeft(2).Render(view))
	v.AltScreen = true
	return v
}

func (m Model) content() string {
	t := m.theme()
	var body string
	switch m.screen {
	case home:
		body = screens.Home(m.cursor, m.command.Value(), m.homeCategory, t)
		if m.projectContext != "" {
			body = t.Muted(t.Truncate(m.projectContext, t.Width())) + "\n\n" + body
		}
	case selection:
		items := make([]screens.TemplateItem, len(m.templates))
		for i, tmpl := range m.templates {
			items[i] = screens.TemplateItem{
				ID:          tmpl.ID,
				DisplayName: tmpl.DisplayName,
				Description: tmpl.Description,
			}
		}
		body = screens.TemplateSelect(items, m.cursor, t)
	case form:
		values := make([]string, len(m.inputs))
		for i := range m.inputs {
			values[i] = m.inputs[i].View()
		}
		body = screens.InitForm(values, m.focus, m.opts.Request.Force, m.opts.DryRun, t)
		if m.formError != "" {
			body = t.Accent("! "+m.formError) + "\n\n" + body
		}
	case planning:
		body = m.loading("Building your preview", "Rendering the template and checking destination files.")
	case preview:
		body = screens.Preview(*m.plan, m.opts.DryRun, t)
	case confirmation:
		body = t.Heading("Ready to create?", "Apply the file operations you just reviewed.") + t.Muted(m.plan.TargetDir) + "\n\n" +
			t.Choice("No, return to preview", "", !m.confirmYes) + t.Choice("Yes, generate files", "", m.confirmYes)
	case applying:
		body = m.loading("Creating your project", "Writing files from the approved plan.")
		if m.canceling {
			body = t.Warn("Canceling; waiting for the current file to finish…")
		}
	case result:
		body = screens.Result(*m.result, m.err, t)
	case help:
		body = screens.Help(t)
	case about:
		body = t.Heading("Small tool. Solid foundations.", "AIAI CLI · "+m.opts.Version) + "Deterministic project scaffolding.\nEmbedded templates. Offline generation.\n\n" + t.Muted("Choose a template, review the plan, and start building.")
	case authStarting:
		body = m.loading("Connecting to AIAI", "Preparing your authentication session.")
	case authWaiting:
		if m.authDevice == nil {
			body = m.loading("Connecting to AIAI", "Preparing your authentication session.")
		} else {
			body = screens.DeviceLogin(*m.authDevice, t) + "\n\n" + m.progress.View() + " " + t.Muted("Waiting for authorization…")
		}
	case authResult:
		body = screens.AuthResult(m.authText, m.authWarn, m.authOK, t)
	case profile:
		if m.authUser == nil {
			body = screens.AuthResult("Not signed in. Run /login to continue.", "", false, t)
		} else {
			body = screens.AuthProfile(*m.authUser, m.authStore, t)
		}
	case logoutConfirmation:
		body = t.Heading("Sign out?", "This removes the AIAI credential stored on this device.") +
			t.Choice("No, keep me signed in", "", !m.logoutYes) + t.Choice("Yes, sign out", "", m.logoutYes)
	case loggingOut:
		body = m.loading("Signing out", "Revoking the server session and removing local credentials.")
	case updatePrompt:
		tag := ""
		bodyText := ""
		if m.updateRelease != nil {
			tag = m.updateRelease.TagName
			bodyText = m.updateRelease.Body
		}
		body = screens.UpdatePrompt(m.opts.Version, tag, bodyText, m.cursor, t)
	case updating:
		tag := ""
		if m.updateRelease != nil {
			tag = m.updateRelease.TagName
		}
		body = m.progress.View() + " " + screens.Updating(tag, m.updateStep, t)
	case updateResult:
		tag := ""
		if m.updateRelease != nil {
			tag = m.updateRelease.TagName
		}
		body = screens.UpdateResult(tag, m.updateResultOK, m.updateErr, m.updateResultCursor, t)
	case serverLoading:
		body = m.loading("Loading SSH servers", "Reading local profiles from this computer.")
	case serverList:
		body = screens.ServerList(m.visibleServers(), m.cursor, m.serverConnect, m.search.Value(), t)
	case serverDetail:
		servers := m.visibleServers()
		if len(servers) == 0 {
			body = screens.SSHFailure(nil, t)
		} else {
			body = screens.ServerDetail(servers[m.cursor], t)
		}
	case logLoading:
		body = m.loading("Loading SSH history", "Reading local access logs.")
	case accessLogs:
		body = screens.AccessLogs(m.logs, t)
	case sshFailure:
		body = screens.SSHFailure(m.sshErr, t)
	case keyLoading:
		body = m.loading("Finding SSH keys", "Scanning AIAI's managed key directory and ~/.ssh.")
	case keyList:
		body = screens.KeyList(m.visibleKeys(), m.keyQuery, m.search.Value(), m.keyNotice, m.cursor, t)
	case keyDetail:
		keys := m.visibleKeys()
		if len(keys) == 0 {
			body = screens.SSHFailure(nil, t)
		} else {
			body = screens.KeyDetail(keys[m.cursor], m.keyNotice, t)
		}
	case keyDeleteConfirmation:
		keys := m.visibleKeys()
		if len(keys) == 0 {
			body = screens.SSHFailure(nil, t)
		} else {
			body = screens.KeyDeleteConfirmation(keys[m.cursor], m.keyDeleteYes, t)
		}
	}
	return lipgloss.NewStyle().Width(t.Width()).Render(body)
}

func (m Model) loading(title, subtitle string) string {
	return m.progress.View() + " " + m.theme().Title(title) + "\n" + m.theme().Muted(subtitle)
}

func (m Model) chrome() (string, string) {
	fit := lipgloss.NewStyle().Width(m.theme().Width())
	return fit.Render(m.header() + "\n" + m.toolbar()), fit.Render("\n" + m.footer())
}

func (m Model) header() string {
	t := m.theme()
	return t.Title("AIAI CLI") + "\n" + t.Rule()
}

func (m Model) toolbar() string {
	t := m.theme()
	if m.screen == home {
		return t.Accent("> ") + m.command.View() + "\n" + t.Rule()
	}
	if m.screen == serverList || m.screen == keyList {
		if !m.searching && m.search.Value() == "" {
			return ""
		}
		return m.search.View() + "\n" + t.Rule()
	}
	step := -1
	switch m.screen {
	case selection:
		step = 0
	case form, planning:
		step = 1
	case preview, confirmation:
		step = 2
	case applying, result:
		step = 3
	}
	if step >= 0 {
		return t.Step(step) + "\n"
	}
	return ""
}

func (m Model) footer() string {
	t := m.theme()
	hint := "↑/↓ navigate · enter select · esc back"
	switch m.screen {
	case home:
		if m.homeCategory == "" && m.command.Value() == "" {
			hint = "↑/↓ choose · enter open"
		} else {
			hint = "↑/↓ choose · enter select · tab complete"
		}
	case form:
		hint = "tab next · shift+tab previous · enter continue"
	case preview:
		hint = "↑/↓ scroll · enter continue · esc edit"
		if m.plan.HasConflicts() {
			hint = "↑/↓ scroll · esc edit options"
		}
	case confirmation:
		hint = "←/→ choose · enter confirm · esc preview"
	case result:
		hint = "↑/↓ scroll · enter finish"
	case help, about:
		hint = "↑/↓ scroll · esc back"
	case authWaiting:
		hint = "esc back · ctrl+c cancel"
	case authResult, profile:
		hint = "enter or esc back"
	case logoutConfirmation:
		hint = "←/→ choose · enter confirm · esc back"
	case updatePrompt:
		hint = "↑/↓ choose · enter confirm · esc back"
	case updateResult:
		hint = "↑/↓ choose · enter confirm · esc back"
	case keyDetail:
		hint = "c copy public key · d delete managed key · esc back"
	case keyDeleteConfirmation:
		hint = "←/→ choose · enter confirm · esc back"
	case serverList:
		if m.serverConnect {
			hint = "↑/↓ choose · / search · enter connect · esc back"
		} else {
			hint = "↑/↓ choose · / search · enter details · esc back"
		}
	case serverDetail, sshFailure:
		hint = "enter or esc back"
	case accessLogs:
		hint = "↑/↓ scroll · esc back"
	case keyList:
		hint = "↑/↓ choose · / search · enter details · esc back"
	case planning, applying, authStarting, loggingOut, updating, serverLoading, logLoading, keyLoading:
		hint = "ctrl+c cancel safely"
	}
	if t.Width() < 48 {
		switch m.screen {
		case home, selection:
			hint = "↑/↓ choose · enter open · esc back"
		case form:
			hint = "tab next · enter continue · esc back"
		case serverList, keyList:
			hint = "↑/↓ choose · / search · enter open · esc back"
		case confirmation, logoutConfirmation, keyDeleteConfirmation:
			hint = "←/→ choose · enter confirm · esc back"
		case keyDetail:
			hint = "c copy · d delete · esc back"
		case planning, applying, authStarting, loggingOut, updating, serverLoading, logLoading, keyLoading:
			hint = "ctrl+c cancel"
		default:
			hint = "↑/↓ scroll · enter open · esc back"
		}
	}
	status := " · ? help"
	if m.screen == home {
		status = " · esc quit"
		if m.homeCategory != "" {
			status = " · esc categories"
		}
	}
	if m.opts.DryRun {
		status = " · preview only"
	}
	if t.Width() < 48 {
		status = ""
	}
	return t.Keys(hint) + t.Muted(status)
}
