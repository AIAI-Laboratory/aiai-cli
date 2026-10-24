package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/screens"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
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
	if m.showingShellOutput() && !m.shellHistory && m.shellFollow {
		m.viewport.GotoBottom()
	}
	if m.showingShellOutput() && m.shellHistory {
		m.viewport.EnsureVisible(m.shellSelected*3, 0, 2)
	}
	if m.screen == form || m.screen == home && !m.showingShellOutput() || m.screen == selection || m.screen == confirmation || m.screen == updatePrompt || m.screen == updateResult || m.screen == serverList || m.screen == keyList || m.screen == keyDeleteConfirmation || m.screen == projectPicker || m.screen == projectAdd {
		for line, text := range strings.Split(content, "\n") {
			if strings.Contains(text, "> ") {
				m.viewport.EnsureVisible(line, 0, 1)
				if m.screen == form && m.focus < 3 || m.screen == projectPicker || m.screen == projectAdd {
					if m.viewport.Height() > 1 && line+1 >= m.viewport.YOffset()+m.viewport.Height() {
						m.viewport.SetYOffset(line + 2 - m.viewport.Height())
					}
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
	case projectPicker, projectAdd:
		body = m.projectSwitchView()
	case home:
		if m.showingShellOutput() {
			body = m.shellContent()
		} else if shellInput(m.command.Value()) {
			body = t.Title(shellPath(m.opts.Terminal.Dir)) + "\n\n" + t.Keys("enter run · alt+enter interactive")
		} else {
			body = screens.Home(m.cursor, m.command.Value(), m.homeCategory, t)
		}
		if m.shellNotice != "" {
			body = t.Warn(m.shellNotice) + "\n\n" + body
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
	case planning:
		body = m.loading("Building preview…")
	case preview:
		body = screens.Preview(*m.plan, m.opts.DryRun, t)
	case confirmation:
		body = t.Heading("Create project?", "") + t.Muted(m.plan.TargetDir) + "\n\n" +
			t.Choice("No, return to preview", "", !m.confirmYes) + t.Choice("Yes, generate files", "", m.confirmYes)
	case applying:
		body = m.loading("Creating project…")
		if m.canceling {
			body = t.Warn("Canceling; waiting for the current file to finish…")
		}
	case result:
		body = screens.Result(*m.result, m.err, t)
	case help:
		body = screens.Help(t)
	case about:
		body = t.Heading("AIAI CLI · "+m.opts.Version, "") + "Projects · Git · SSH · Zen · Monitor"
	case authStarting:
		body = m.loading("Connecting to AIAI…")
	case authWaiting:
		if m.authDevice == nil {
			body = m.loading("Connecting to AIAI…")
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
		body = m.loading("Signing out…")
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
		if m.updateResultOK && m.updateNotice != "" {
			body = t.Heading("Update check", m.updateNotice) + t.Choice("Return to home", "", true)
		}
	case serverLoading:
		body = m.loading("Loading SSH servers…")
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
		body = m.loading("Loading SSH history…")
	case accessLogs:
		body = screens.AccessLogs(m.logs, t)
	case sshFailure:
		body = screens.SSHFailure(m.sshErr, t)
	case keyLoading:
		body = m.loading("Finding SSH keys…")
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

func (m Model) loading(title string) string {
	return m.progress.View() + " " + m.theme().Title(title)
}

func (m Model) chrome() (string, string) {
	fit := lipgloss.NewStyle().Width(m.theme().Width())
	top, bottom := m.header()+"\n"+m.toolbar(), "\n"+m.footer()
	if m.showingShellOutput() {
		top, bottom = m.shellHeader(), m.theme().Rule()+"\n"+m.toolbar()+"\n"+m.footer()
		if m.shellHistory {
			bottom = m.theme().Rule() + "\n" + m.footer()
		}
	}
	panel := m.completionView(m.height - lipgloss.Height(fit.Render(top)) - lipgloss.Height(fit.Render(bottom)) - 3)
	if panel != "" {
		if m.showingShellOutput() {
			bottom = m.theme().Rule() + "\n" + m.toolbar() + "\n" + panel + "\n" + m.footer()
		} else {
			top += "\n" + panel
		}
	}
	return fit.Render(top), fit.Render(bottom)
}

func (m Model) header() string {
	t := m.theme()
	header := t.Title("AIAI CLI")
	if m.screen == projectPicker {
		header = t.Title("AIAI CLI / Switch project")
		if t.Width() < 32 {
			header = t.Title("Switch project")
		}
	}
	if m.screen == home && !m.showingShellOutput() && m.homeCategory == "" && strings.TrimSpace(m.command.Value()) == "" && m.height >= 18 {
		header = m.welcomeHeader()
	}
	if m.screen == home && m.projectContext.Project != "" {
		return header + "\n" + m.contextView() + "\n" + t.Rule()
	}
	return header + "\n" + t.Rule()
}

func (m Model) contextView() string {
	t := m.theme()
	width := t.Width()
	c := m.projectContext
	project := strings.Join(strings.Fields(ansi.Strip(c.Project)), " ")
	branch := strings.Join(strings.Fields(ansi.Strip(c.Branch)), " ")
	if branch == "" {
		return t.Truncate(t.Title("Project "+project), width)
	}
	status, color := "Clean", style.Good
	if c.Changes > 0 {
		status, color = fmt.Sprintf("%d changes", c.Changes), style.Warn
		if c.Changes == 1 {
			status = "1 change"
		}
	}
	status = style.Paint("["+status+"]", color, true, t.NoColor())
	project = t.Truncate(t.Title("Project "+project), width)
	branch = style.Paint("├─ "+branch, style.Info, true, t.NoColor())
	if width < 32 {
		return project + "\n" + t.Truncate(branch, width) + "\n" + t.Truncate(status, width)
	}
	branch = t.Truncate(branch, max(1, width-lipgloss.Width(status)-3))
	return project + "\n" + branch + "   " + status
}

func (m Model) welcomeHeader() string {
	t := m.theme()
	version := strings.TrimPrefix(m.opts.Version, "v")
	title := t.Title("AIAI CLI")
	if version != "" {
		title += "  " + t.Muted(version)
	}

	path, err := os.Getwd()
	if err != nil || path == "" {
		path = m.opts.Terminal.Dir
	}
	status := t.Good("●") + " " + t.Muted("Offline templates")
	if m.authUser != nil && m.authUser.Login != "" {
		status += t.Muted(" · Signed in as @" + m.authUser.Login)
	}

	if t.Width() < 60 {
		return truncate(t.Accent("◆")+" "+title, t.Width()) + "\n" +
			truncate(t.Muted("Your next project starts here."), t.Width()) + "\n" +
			truncate(t.Muted(path), t.Width()) + "\n" + truncate(status, t.Width())
	}

	logo := strings.Split(m.logo(), "\n")
	info := []string{
		"",
		title,
		t.Muted("Your next project starts here."),
		"",
		t.Muted(path),
		status,
		"",
	}
	logoWidth := 0
	for _, logoRow := range logo {
		logoWidth = max(logoWidth, lipgloss.Width(logoRow))
	}
	textWidth := max(1, t.Width()-logoWidth-3)
	rows := make([]string, len(logo))
	for i, logoRow := range logo {
		padding := strings.Repeat(" ", logoWidth-lipgloss.Width(logoRow))
		rows[i] = logoRow + padding + "   " + truncate(info[i], textWidth)
	}
	return strings.Join(rows, "\n")
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

func (m Model) toolbar() string {
	t := m.theme()
	if m.screen == projectPicker || m.screen == projectAdd {
		var rows []string
		if m.screen == projectPicker {
			rows = append(rows, style.Search(m.projects.input, t.Width(), t.NoColor()), t.Rule())
		}
		if m.projects.notice != "" {
			rows = append(rows, m.noticeView(m.projects.notice))
		}
		return strings.Join(rows, "\n")
	}
	if m.screen == home {
		if m.showingShellOutput() {
			return t.Accent("❯ ") + m.command.View()
		}
		return t.Accent("> ") + m.command.View() + "\n" + t.Rule()
	}
	if m.screen == serverList || m.screen == keyList {
		if !m.searching && m.search.Value() == "" {
			return ""
		}
		return style.Search(m.search, t.Width(), t.NoColor()) + "\n" + t.Rule()
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
		if m.screen == form && m.formError != "" {
			return t.Step(step) + "\n" + m.noticeView(m.formError)
		}
		return t.Step(step) + "\n"
	}
	return ""
}

func (m Model) noticeView(message string) string {
	t := m.theme()
	message = strings.Join(strings.Fields(ansi.Strip(message)), " ")
	lines := strings.Split(ansi.Wrap(message, t.Width(), " "), "\n")
	limit := min(3, max(1, m.height/6))
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = t.Truncate(lines[limit-1]+"…", t.Width())
	}
	return t.Warn(strings.Join(lines, "\n"))
}

func (m Model) footer() string {
	t := m.theme()
	hint := "↑/↓ navigate · enter select · esc back"
	switch m.screen {
	case projectPicker:
		hint = "↑/↓ select · enter switch · ctrl+r refresh · esc back"
	case projectAdd:
		hint = "tab field · enter next · esc back"
		if m.projects.focus == 1 {
			hint = "tab field · enter add & switch · esc back"
		}
	case home:
		if m.shellHistory {
			hint = "↑/↓ select · enter view output · F2 back"
		} else if m.shellJob != nil {
			hint = "pgup/pgdown scroll · ctrl+c stop"
		} else if m.showingShellOutput() {
			hint = "tab complete · alt+←/→ runs · pgup/pgdown scroll"
		} else if shellInput(m.command.Value()) {
			hint = "tab complete · ↑/↓ history · enter run"
		} else if m.homeCategory == "" && m.command.Value() == "" {
			hint = "↑/↓ choose · enter open · ctrl+k projects"
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
	if (m.screen == projectPicker || m.screen == projectAdd) && m.projects.busy {
		hint = "ctrl+c cancel"
		if m.screen == projectPicker {
			hint = "esc back · ctrl+c cancel"
		}
	}
	if t.Width() < 48 {
		switch m.screen {
		case projectPicker:
			hint = "↑/↓ select · enter · esc back"
			if m.projects.busy {
				hint = "esc back · ctrl+c cancel"
			}
		case projectAdd:
			hint = "tab field · enter · esc back"
			if m.projects.busy {
				hint = "ctrl+c cancel"
			}
		case home, selection:
			hint = "↑/↓ choose · enter open · esc back"
			if m.screen == home {
				hint = "↑↓ · enter · esc quit"
				if m.command.Value() != "" {
					hint = "↑↓ · enter · esc clear"
				} else if m.homeCategory != "" {
					hint = "↑↓ · enter · esc back"
				}
				if shellInput(m.command.Value()) && !m.showingShellOutput() {
					hint = "↑↓ history · enter run"
				}
			}
			if m.screen == home && m.showingShellOutput() {
				hint = "alt+←/→ runs · F2 history"
				if m.shellHistory {
					hint = "↑/↓ select · enter view · F2 back"
				}
				if m.shellJob != nil {
					hint = "pgup/pgdown scroll · ctrl+c stop"
				}
			}
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
	if m.screen == projectPicker || m.screen == projectAdd {
		status = ""
	}
	if m.screen == home {
		status = " · esc quit"
		if m.command.Value() != "" {
			status = " · esc clear"
		} else if m.showingShellOutput() {
			status = ""
		} else if m.homeCategory != "" {
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
