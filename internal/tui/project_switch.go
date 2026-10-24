package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

type projectSwitchState struct {
	items            []workspace.Project
	input, name      textinput.Model
	cursor, focus    int
	request, returnY int
	query, notice    string
	busy             bool
}

type projectsLoadedMsg struct {
	request int
	items   []workspace.Project
	err     error
}

type projectAddedMsg struct {
	request int
	project workspace.Project
	err     error
}

type projectOpenedMsg struct {
	dir string
	err error
}

func (m Model) loadContext() tea.Cmd {
	if m.opts.Context == nil {
		return nil
	}
	dir := m.opts.Terminal.Dir
	return func() tea.Msg { return contextMsg{dir: dir, context: m.opts.Context(m.ctx, dir)} }
}

func (m Model) openProjectSwitcher() (tea.Model, tea.Cmd) {
	if m.shellJob != nil {
		m.shellNotice = "Stop the running command with Ctrl+C before switching projects."
		m.shellFollow = false
		m.viewport.GotoTop()
		return m, nil
	}
	if m.opts.Workspace == nil {
		m.shellNotice = "Workspace is unavailable."
		return m, nil
	}
	request := m.projects.request + 1
	input, name := textinput.New(), textinput.New()
	input.Prompt, name.Prompt = "", ""
	input.Placeholder = "Name, path or tag…"
	input.CharLimit, name.CharLimit = 8192, 240
	name.Placeholder = "Folder name (optional)"
	input.SetVirtualCursor(true)
	name.SetVirtualCursor(true)
	m.styleInput(&input)
	m.styleInput(&name)
	m.projects = projectSwitchState{input: input, name: name, request: request, returnY: m.viewport.YOffset(), busy: true}
	m.screen = projectPicker
	m.completion = nil
	m.command.Blur()
	m.viewport.GotoTop()
	focus := m.projects.input.Focus()
	return m, tea.Batch(focus, m.progress.Tick, m.loadProjects())
}

func (m Model) loadProjects() tea.Cmd {
	request, service, ctx := m.projects.request, m.opts.Workspace, m.ctx
	return func() tea.Msg {
		items, err := service.List(ctx, false)
		return projectsLoadedMsg{request: request, items: items, err: err}
	}
}

func (m Model) projectsLoaded(msg projectsLoadedMsg) (tea.Model, tea.Cmd) {
	if m.screen != projectPicker || msg.request != m.projects.request {
		return m, nil
	}
	m.projects.busy = false
	if msg.err != nil {
		m.projects.notice = "Cannot load projects: " + msg.err.Error()
		return m, nil
	}
	sort.SliceStable(msg.items, func(i, j int) bool {
		a, b := msg.items[i], msg.items[j]
		if a.LastOpenedAt.Equal(b.LastOpenedAt) {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.LastOpenedAt.After(b.LastOpenedAt)
	})
	m.projects.items, m.projects.cursor, m.projects.notice = msg.items, 0, ""
	return m, nil
}

func (m Model) matchingProjects() []workspace.Project {
	query := strings.ToLower(strings.TrimSpace(m.projects.input.Value()))
	items := []workspace.Project{}
	for _, p := range m.projects.items {
		if query == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Root+" "+strings.Join(p.Tags, " ")), query) {
			items = append(items, p)
		}
	}
	return items
}

func (m Model) currentFolderRegistered() bool {
	dir := m.currentProjectDir()
	for _, p := range m.projects.items {
		if p.Root == dir {
			return true
		}
	}
	return false
}

func (m Model) currentProjectDir() string {
	if dir, err := workspace.Canonical(m.opts.Terminal.Dir); err == nil {
		return dir
	}
	return m.opts.Terminal.Dir
}

func (m Model) closeProjectSwitcher() (tea.Model, tea.Cmd) {
	m.screen = home
	m.projects.input.Blur()
	m.projects.name.Blur()
	m.resize()
	m.viewport.SetContent(m.content())
	m.viewport.SetYOffset(m.projects.returnY)
	return m, m.command.Focus()
}

func (m Model) switchProject(p workspace.Project) (tea.Model, tea.Cmd) {
	currentDir := m.currentProjectDir()
	if err := os.Chdir(p.Root); err != nil {
		m.projects.notice = "Cannot open project: directory is missing or inaccessible."
		return m, m.projects.input.Focus()
	}
	if currentDir != p.Root {
		m.opts.Terminal.PreviousDir = m.opts.Terminal.Dir
	}
	m.opts.Terminal.Dir = p.Root
	m.projectContext = ProjectContext{Project: p.Name}
	m.shellHidden, m.shellHistory = true, false
	m.shellNotice = ""
	next, focus := m.closeProjectSwitcher()
	m = next.(Model)
	service, ctx := m.opts.Workspace, m.ctx
	return m, tea.Batch(focus, m.loadContext(), func() tea.Msg {
		return projectOpenedMsg{dir: p.Root, err: service.Open(ctx, p.ID)}
	})
}

func (m Model) projectSwitchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.projects.busy {
		if m.screen == projectPicker {
			if key == "esc" {
				return m.closeProjectSwitcher()
			}
			if key != "enter" && key != "up" && key != "down" {
				return m.updateProjectInput(msg)
			}
		}
		return m, nil
	}
	if m.screen == projectAdd {
		switch key {
		case "esc":
			m.screen = projectPicker
			m.projects.input.SetValue(m.projects.query)
			m.projects.input.Placeholder = "Name, path or tag…"
			m.projects.name.Blur()
			m.projects.notice = ""
			return m, m.projects.input.Focus()
		case "tab", "shift+tab":
			m.projects.focus = 1 - m.projects.focus
			m.projects.input.Blur()
			m.projects.name.Blur()
			if m.projects.focus == 0 {
				return m, m.projects.input.Focus()
			}
			return m, m.projects.name.Focus()
		case "enter":
			if m.projects.focus == 0 {
				m.projects.focus = 1
				m.projects.input.Blur()
				return m, m.projects.name.Focus()
			}
			return m.addProject()
		}
		return m.updateProjectInput(msg)
	}
	items := m.matchingProjects()
	switch key {
	case "esc":
		return m.closeProjectSwitcher()
	case "ctrl+r":
		m.projects.request++
		m.projects.busy = true
		return m, m.withProgress(m.loadProjects())
	case "up":
		m.projects.cursor = max(0, m.projects.cursor-1)
	case "down":
		last := len(items)
		if !m.currentFolderRegistered() {
			last++
		}
		m.projects.cursor = min(last, m.projects.cursor+1)
	case "enter":
		if m.projects.cursor < len(items) {
			return m.switchProject(items[m.projects.cursor])
		}
		m.screen = projectAdd
		m.projects.query = m.projects.input.Value()
		m.projects.input.SetValue("")
		m.projects.input.Placeholder = "~/work/project or /Volumes/SSD/project"
		m.projects.name.SetValue("")
		m.projects.focus, m.projects.notice = 0, ""
		m.viewport.GotoTop()
		if m.projects.cursor > len(items) {
			m.projects.input.SetValue(m.opts.Terminal.Dir)
			m.projects.name.SetValue(filepath.Base(m.opts.Terminal.Dir))
			m.projects.focus = 1
			m.projects.input.Blur()
			return m, m.projects.name.Focus()
		}
		return m, m.projects.input.Focus()
	default:
		return m.updateProjectInput(msg)
	}
	return m, nil
}

func (m Model) updateProjectInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.projects.busy && m.screen == projectAdd {
		return m, nil
	}
	var cmd tea.Cmd
	if m.screen == projectAdd && m.projects.focus == 1 {
		m.projects.name, cmd = m.projects.name.Update(msg)
	} else {
		before := m.projects.input.Value()
		m.projects.input, cmd = m.projects.input.Update(msg)
		if m.screen == projectPicker && before != m.projects.input.Value() {
			m.projects.cursor = 0
			m.viewport.GotoTop()
		}
	}
	return m, cmd
}

func (m Model) addProject() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.projects.input.Value())
	if path == "" {
		m.projects.notice = "Enter an existing project directory."
		return m, nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			m.projects.notice = err.Error()
			return m, nil
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	} else if strings.HasPrefix(path, "~") {
		m.projects.notice = "Use ~/ for a home-relative directory."
		return m, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.opts.Terminal.Dir, path)
	}
	m.projects.busy = true
	request, name, service, ctx := m.projects.request, m.projects.name.Value(), m.opts.Workspace, m.ctx
	return m, m.withProgress(func() tea.Msg {
		p, err := service.Add(ctx, path, name, "", nil, false)
		return projectAddedMsg{request: request, project: p, err: err}
	})
}

func (m Model) projectAdded(msg projectAddedMsg) (tea.Model, tea.Cmd) {
	if m.screen != projectAdd || msg.request != m.projects.request {
		return m, nil
	}
	m.projects.busy = false
	if msg.err != nil {
		m.projects.notice = strings.ReplaceAll(msg.err.Error(), "choose --name", "choose another name")
		if m.projects.focus == 1 {
			return m, m.projects.name.Focus()
		}
		return m, m.projects.input.Focus()
	}
	m.projects.items = append(m.projects.items, msg.project)
	m.screen = projectPicker
	m.projects.input.SetValue("")
	m.projects.input.Placeholder = "Name, path or tag…"
	m.projects.cursor = len(m.projects.items) - 1
	m.projects.name.Blur()
	return m.switchProject(msg.project)
}

func (m Model) projectSwitchView() string {
	t := m.theme()
	if m.projects.busy {
		if m.screen == projectAdd {
			return m.loading("Adding project…")
		}
		return m.loading("Loading projects…")
	}
	if m.screen == projectAdd {
		body := t.Heading("Add and switch project", "")
		for i, field := range []struct{ label, value string }{{"Directory", m.projects.input.View()}, {"Name (optional)", m.projects.name.View()}} {
			marker := "  "
			if i == m.projects.focus {
				marker = "> "
			}
			body += marker + t.Title(field.label) + "\n  " + field.value + "\n\n"
		}
		return body
	}
	body := ""
	items := m.matchingProjects()
	currentDir := m.currentProjectDir()
	if len(items) == 0 {
		message := "No matching projects."
		if len(m.projects.items) == 0 && m.projects.input.Value() == "" {
			message = "No registered projects."
		}
		body += t.Muted(message) + "\n\n"
	}
	for i, p := range items {
		marker, name := "  ", t.Title(projectLabel(p.Name))
		if i == m.projects.cursor {
			marker, name = "> ", t.Accent(projectLabel(p.Name))
		}
		if p.Root == currentDir {
			name += t.Good(" (current)")
		}
		body += marker + t.Truncate(name, max(1, t.Width()-2)) + "\n  " + t.Muted(t.Truncate(projectLabel(shellPath(p.Root)), max(1, t.Width()-2))) + "\n\n"
	}
	body += t.Choice("Add project from path…", "", m.projects.cursor == len(items))
	if !m.currentFolderRegistered() {
		body += t.Choice("Add current folder as project…", "", m.projects.cursor == len(items)+1)
	}
	return body
}

func projectLabel(text string) string {
	if strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return strconv.QuoteToGraphic(text)
	}
	return text
}
