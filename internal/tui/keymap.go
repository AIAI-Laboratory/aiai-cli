package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
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
	if (m.screen == serverList || m.screen == keyList) && m.searching {
		switch key {
		case "esc":
			m.clearSearch()
			m.cursor = 0
			return m, nil
		case "enter":
			m.search.Blur()
			m.searching = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.cursor = 0
			m.viewport.GotoTop()
			return m, cmd
		}
	}
	textEditing := m.screen == home || (m.screen == form && m.focus < len(m.inputs))
	if key == "?" && !textEditing {
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
			if m.homeCategory != "" {
				m.homeCategory = ""
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
		case authStarting, authWaiting, authResult, profile, logoutConfirmation, updatePrompt, updateResult, serverLoading, logLoading, accessLogs, sshFailure, keyLoading:
			m.screen, m.cursor = home, 0
		case serverList:
			if m.search.Value() != "" {
				m.clearSearch()
				m.cursor = 0
				return m, nil
			}
			m.screen, m.cursor = home, 0
		case serverDetail:
			m.screen = serverList
		case keyList:
			if m.search.Value() != "" {
				m.clearSearch()
				m.cursor = 0
				return m, nil
			}
			m.screen, m.cursor = home, 0
		case keyDetail:
			m.screen = keyList
		case keyDeleteConfirmation:
			m.screen, m.keyDeleteYes = keyDetail, false
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
		case "up", "down", "left", "right", "j", "k", "h", "l", "tab":
			m.confirmYes = !m.confirmYes
		case "enter":
			if !m.confirmYes {
				m.screen = preview
				return m, nil
			}
			m.screen = applying
			plan := *m.plan
			return m, m.withProgress(func() tea.Msg { r, err := m.executor.Apply(m.ctx, plan); return appliedMsg{r, err} })
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
		case "up", "down", "left", "right", "j", "k", "h", "l", "tab":
			m.logoutYes = !m.logoutYes
		case "enter":
			if !m.logoutYes {
				m.screen = home
				return m, nil
			}
			m.screen = loggingOut
			return m, m.withProgress(func() tea.Msg {
				result, err := m.auth.Logout(m.ctx)
				return authLogoutMsg{result: result, err: err}
			})
		}
	case updatePrompt:
		switch key {
		case "up", "k":
			m.cursor = (m.cursor + 2) % 3
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % 3
		case "enter":
			if m.updateRelease == nil {
				m.screen, m.cursor = home, 0
				return m, nil
			}
			switch m.cursor {
			case 0:
				m.screen = updating
				m.updateStep = "Downloading update from GitHub…"
				release := *m.updateRelease
				return m, m.withProgress(func() tea.Msg {
					err := m.updater.ApplyUpdate(m.ctx, release)
					return updateFinishedMsg{err: err}
				})
			case 1:
				if m.updater != nil {
					_ = m.updater.Postpone(*m.updateRelease, false)
				}
				m.screen, m.cursor = home, 0
			case 2:
				if m.updater != nil {
					_ = m.updater.Postpone(*m.updateRelease, true)
				}
				m.screen, m.cursor = home, 0
			}
		}
	case updateResult:
		if m.updateResultOK {
			switch key {
			case "up", "down", "left", "right", "j", "k", "h", "l", "tab":
				m.updateResultCursor = (m.updateResultCursor + 1) % 2
			case "enter":
				if m.updateResultCursor == 0 {
					if m.updater != nil {
						_ = m.updater.Restart()
					}
					return m, tea.Quit
				}
				return m, tea.Quit
			}
		} else {
			if key == "enter" {
				m.screen, m.cursor = home, 0
			}
		}
	case serverList:
		servers := m.visibleServers()
		count := len(servers)
		if key == "/" {
			m.searching = true
			return m, m.search.Focus()
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
			if m.serverConnect {
				m.sshAlias = servers[m.cursor].Alias
				return m, tea.Quit
			}
			m.screen = serverDetail
			m.viewport.GotoTop()
		}
	case serverDetail, sshFailure:
		if key == "enter" {
			if m.screen == serverDetail {
				m.screen = serverList
			} else {
				m.screen = home
			}
			if m.screen == home {
				m.cursor = 0
			}
		}
	case keyList:
		keys := m.visibleKeys()
		count := len(keys)
		if key == "/" {
			m.searching = true
			return m, m.search.Focus()
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
			m.keyNotice = ""
			m.screen = keyDetail
			m.viewport.GotoTop()
		}
	case keyDetail:
		keys := m.visibleKeys()
		if len(keys) == 0 {
			return m, nil
		}
		selected := keys[m.cursor]
		switch key {
		case "c":
			m.keyNotice = "Copying public key…"
			return m, func() tea.Msg {
				publicKey, err := m.sshManager.PublicKey(m.ctx, selected.PrivatePath)
				if err == nil {
					err = m.clipboard(publicKey)
				}
				return keyCopiedMsg{name: selected.Name, err: err}
			}
		case "d":
			if selected.Managed {
				m.keyDeleteYes = false
				m.screen = keyDeleteConfirmation
			}
		case "enter":
			m.screen = keyList
		}
	case keyDeleteConfirmation:
		switch key {
		case "up", "down", "left", "right", "j", "k", "h", "l", "tab":
			m.keyDeleteYes = !m.keyDeleteYes
		case "enter":
			if !m.keyDeleteYes {
				m.screen = keyDetail
				return m, nil
			}
			selected := m.visibleKeys()[m.cursor]
			m.keyNotice = "Deleting managed key…"
			return m, func() tea.Msg {
				err := m.sshManager.RemoveKey(m.ctx, selected.PrivatePath)
				return keyDeletedMsg{name: selected.Name, err: err}
			}
		}
	}
	if m.screen == preview || m.screen == result || m.screen == help || m.screen == about || m.screen == authWaiting || m.screen == authResult || m.screen == profile || m.screen == updatePrompt || m.screen == updateResult || m.screen == serverDetail || m.screen == accessLogs || m.screen == sshFailure || m.screen == keyDetail || m.screen == keyDeleteConfirmation {
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
	matches := screens.VisibleCommands(m.command.Value(), m.homeCategory)
	categories := screens.Categories()
	rootCommands := screens.RootCommands()
	browsingRoot := m.command.Value() == "" && m.homeCategory == ""
	key := msg.String()
	if key == "enter" {
		fields := strings.Fields(m.command.Value())
		if len(fields) == 2 && fields[0] == "/keygen" {
			if m.sshManager == nil {
				m.sshErr, m.screen = errors.New("SSH key management is unavailable"), sshFailure
				return m, nil
			}
			request := sshmanager.KeyGenerateRequest{Name: fields[1], Type: "ed25519"}
			m.action.GenerateKey = &request
			m.command.SetValue("")
			return m, tea.Quit
		}
		if len(fields) == 2 && fields[0] == "/keys" {
			if m.sshManager == nil {
				m.sshErr, m.screen = errors.New("SSH key management is unavailable"), sshFailure
				return m, nil
			}
			query := fields[1]
			m.command.SetValue("")
			m.screen = keyLoading
			return m, m.withProgress(func() tea.Msg {
				keys, err := m.sshManager.FindKeys(m.ctx, query)
				return keysLoadedMsg{keys: keys, query: query, err: err}
			})
		}
	}
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
		count := len(matches)
		if browsingRoot {
			count = len(categories) + len(rootCommands)
		}
		if count > 0 {
			delta := 1
			if key == "up" {
				delta = -1
			}
			m.cursor = (m.cursor + delta + count) % count
		}
		return m, nil
	case "tab":
		if !browsingRoot && len(matches) > 0 {
			m.command.SetValue(screens.HomeItems[matches[m.cursor]].Name)
			m.command.CursorEnd()
			m.cursor = 0
		}
		return m, nil
	case "enter":
		if browsingRoot {
			if m.cursor < len(categories) {
				m.homeCategory = categories[m.cursor]
				m.cursor = 0
				m.viewport.GotoTop()
				return m, nil
			}
			matches = []int{rootCommands[m.cursor-len(categories)]}
			m.cursor = 0
		}
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
		case "/servers", "/ssh":
			if m.sshManager == nil {
				m.sshErr, m.screen = errors.New("SSH management is unavailable"), sshFailure
				break
			}
			connect := screens.HomeItems[selected].Name == "/ssh"
			m.screen, m.serverConnect = serverLoading, connect
			return m, m.withProgress(func() tea.Msg {
				servers, err := m.sshManager.List(m.ctx)
				return serversLoadedMsg{servers: servers, err: err, connect: connect}
			})
		case "/logs":
			if m.sshManager == nil {
				m.sshErr, m.screen = errors.New("SSH management is unavailable"), sshFailure
				break
			}
			m.screen = logLoading
			return m, m.withProgress(func() tea.Msg {
				logs, err := m.sshManager.Logs(m.ctx, sshmanager.LogFilter{Limit: 50})
				return logsLoadedMsg{logs: logs, err: err}
			})
		case "/keys":
			if m.sshManager == nil {
				m.sshErr, m.screen = errors.New("SSH key management is unavailable"), sshFailure
				break
			}
			m.screen = keyLoading
			return m, m.withProgress(func() tea.Msg {
				keys, err := m.sshManager.FindKeys(m.ctx, "")
				return keysLoadedMsg{keys: keys, err: err}
			})
		case "/keygen":
			// Keep the user on the command line and make the required argument
			// obvious. This also makes selecting /keygen from the palette useful,
			// instead of turning an incomplete command into an error screen.
			m.command.SetValue("/keygen ")
			m.command.CursorEnd()
			m.cursor = 0
		case "/login":
			if m.auth == nil {
				m.authText, m.authOK, m.screen = "Authentication is unavailable.", false, authResult
				break
			}
			m.screen = authStarting
			return m, m.withProgress(func() tea.Msg {
				start, err := m.auth.StartLogin(m.ctx, auth.LoginOptions{})
				return authStartedMsg{start: start, err: err}
			})
		case "/whoami":
			if m.auth == nil {
				m.authText, m.authOK, m.screen = "Authentication is unavailable.", false, authResult
				break
			}
			m.screen = authStarting
			return m, m.withProgress(func() tea.Msg {
				result, err := m.auth.WhoAmI(m.ctx)
				return authWhoamiMsg{result: result, err: err}
			})
		case "/logout":
			if m.authUser == nil {
				m.authText, m.authOK, m.screen = "Already signed out.", true, authResult
				break
			}
			m.logoutYes, m.screen = false, logoutConfirmation
		case "/update":
			if m.updater == nil {
				m.updateResultOK = false
				m.updateErr = errors.New("updater is unavailable")
				m.screen = updateResult
				break
			}
			m.screen = updating
			m.updateStep = "Checking GitHub for updates…"
			return m, m.withProgress(func() tea.Msg {
				res, err := m.updater.CheckForUpdate(m.ctx, m.opts.Version, true)
				return updateCheckedMsg{result: res, err: err, manual: true}
			})
		case "/zen":
			m.action.Zen = true
			return m, tea.Quit
		case "/git":
			m.action.Git = true
			return m, tea.Quit
		case "/workspace":
			m.action.Workspace = true
			return m, tea.Quit
		case "/monitor":
			m.action.Monitor = true
			return m, tea.Quit
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
