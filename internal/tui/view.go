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
	if m.screen == form || m.screen == home || m.screen == selection || m.screen == confirmation || m.screen == updatePrompt || m.screen == updateResult {
		for line, text := range strings.Split(content, "\n") {
			if strings.HasPrefix(text, "> ") {
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
		body = screens.Home(m.cursor, m.command.Value(), t)
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
		body = t.Heading("Building your preview…", "Rendering the template and checking destination files.")
	case preview:
		body = screens.Preview(*m.plan, m.opts.DryRun, t)
	case confirmation:
		body = t.Heading("Ready to create?", "Apply the file operations you just reviewed.") + t.Muted(m.plan.TargetDir) + "\n\n" +
			t.Choice("No, return to preview", "", !m.confirmYes) + t.Choice("Yes, generate files", "", m.confirmYes)
	case applying:
		body = t.Heading("Creating your project…", "Writing the files from your approved plan.")
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
		body = t.Heading("Connecting to AIAI…", "Preparing your authentication session.")
	case authWaiting:
		if m.authDevice == nil {
			body = t.Heading("Connecting to AIAI…", "Preparing your authentication session.")
		} else {
			body = screens.DeviceLogin(*m.authDevice, t)
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
		body = t.Heading("Signing out…", "Revoking the server session and removing local credentials.")
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
		body = screens.Updating(tag, m.updateStep, t)
	case updateResult:
		tag := ""
		if m.updateRelease != nil {
			tag = m.updateRelease.TagName
		}
		body = screens.UpdateResult(tag, m.updateResultOK, m.updateErr, m.updateResultCursor, t)
	}
	return lipgloss.NewStyle().Width(t.Width()).Render(body)
}

func (m Model) chrome() (string, string) {
	fit := lipgloss.NewStyle().Width(m.theme().Width())
	return fit.Render(m.header() + "\n" + m.toolbar()), fit.Render("\n" + m.footer())
}

// The open A and red i² reproduce the supplied AIAI mark in terminal cells.
// Use the terminal foreground for the A so it remains visible on dark themes.
func (m Model) logo() string {
	a := []string{
		"      ▄██▄    ",
		"     ██████   ",
		"    ▐██▌▐██▌  ",
		"   ▄██▀  ▀██▄ ",
		"  ▄██▀    ▀██▄",
		" ▐██▌      ▐██",
		" ▀▀▀        ▀▀",
	}
	i := []string{" ▄▀▀▄ ", "  ▄▀  ", " ▀▀▀▀ ", " ████ ", " ████ ", " ████ ", " ▀▀▀▀ "}
	for row := range a {
		a[row] += m.theme().Brand(i[row])
	}
	return strings.Join(a, "\n")
}

func (m Model) header() string {
	t := m.theme()
	version := m.opts.Version
	if version == "" {
		version = "dev"
	}
	name := t.Title("AIAI CLI") + "  " + t.Muted(version)
	if m.height < 22 || t.Width() < 55 || (m.screen != home && m.screen != about) {
		return name + "\n" + t.Rule()
	}
	authStatus := "Not signed in"
	if m.authUser != nil {
		authStatus = "Signed in as @" + m.authUser.Login
	}
	detail := "\n" + name + "\n" + t.Muted("Your next project starts here.") + "\n\n" +
		t.Muted(m.workspace) + "\n" + t.Good("●") + t.Muted(" Offline templates · "+authStatus)
	detail = lipgloss.NewStyle().Width(max(1, t.Width()-24)).MaxWidth(max(1, t.Width()-24)).Render(detail)
	return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, m.logo(), "    ", detail) + "\n\n" + t.Rule()
}

func (m Model) toolbar() string {
	t := m.theme()
	if m.screen == home {
		return t.Accent("❯ ") + m.command.View() + "\n" + t.Rule() + "\n"
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
		return t.Step(step) + "\n\n"
	}
	labels := map[screen]string{
		help: "Help", about: "About", authStarting: "Authentication", authWaiting: "GitHub login",
		authResult: "Authentication", profile: "Account", logoutConfirmation: "Sign out", loggingOut: "Sign out",
		updatePrompt: "Update", updating: "Update", updateResult: "Update",
	}
	return t.Muted("AIAI / "+labels[m.screen]) + "\n\n"
}

func (m Model) footer() string {
	t := m.theme()
	hint := "↑/↓ navigate · enter select · esc back"
	switch m.screen {
	case home:
		hint = "↑/↓ navigate · enter select · tab complete"
	case form:
		hint = "tab next · shift+tab previous · enter continue"
	case preview:
		hint = "↑/↓ scroll · enter continue · esc edit"
		if m.plan.HasConflicts() {
			hint = "↑/↓ scroll · esc edit options"
		}
	case confirmation:
		hint = "↑/↓ choose · enter confirm · esc preview"
	case result:
		hint = "↑/↓ scroll · enter finish"
	case help, about:
		hint = "↑/↓ scroll · esc back"
	case authWaiting:
		hint = "esc back · ctrl+c cancel"
	case authResult, profile:
		hint = "enter or esc back"
	case logoutConfirmation:
		hint = "↑/↓ choose · enter confirm · esc back"
	case updatePrompt:
		hint = "↑/↓ choose · enter confirm · esc back"
	case updateResult:
		hint = "↑/↓ choose · enter confirm · esc back"
	case planning, applying, authStarting, loggingOut, updating:
		hint = "ctrl+c cancel safely"
	}
	if t.Width() < 48 {
		hint = "↑/↓ · tab · enter · esc · ?"
	}
	status := "? help · ctrl+c cancel"
	if m.screen == home {
		status = "Type / to browse commands · esc quit"
	}
	if m.opts.DryRun {
		status = "Preview only · No files will be written"
	}
	if t.Width() < 48 {
		status = "ctrl+c cancel"
		if m.screen == home {
			status = "/ commands · esc quit"
		}
	}
	return t.Keys(hint) + "\n" + t.Muted(status)
}
