package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

func switcherModel(t *testing.T) Model {
	t.Helper()
	t.Chdir(t.TempDir())
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	service, err := workspace.New(filepath.Join(t.TempDir(), "workspaces.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(context.Background(), nil, nil, nil, nil, nil, Options{NoColor: true, Workspace: service, Terminal: &platform.ShellSession{Dir: dir}})
	t.Cleanup(m.cancel)
	return m
}

func registerSwitchProject(t *testing.T, m Model, name string, tags ...string) workspace.Project {
	t.Helper()
	p, err := m.opts.Workspace.Add(m.ctx, t.TempDir(), name, "", tags, false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func openSwitcher(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.screen != projectPicker || cmd == nil || !m.projects.busy {
		t.Fatal("Ctrl+K did not start the asynchronous project picker")
	}
	m, _ = runCmd(m, m.loadProjects())
	return m
}

func TestProjectSwitcherSearchRecencyAndCancelPreservesDraft(t *testing.T) {
	m := switcherModel(t)
	first := registerSwitchProject(t, m, "alpha", "frontend")
	recent := registerSwitchProject(t, m, "backend", "api")
	if err := m.opts.Workspace.Open(m.ctx, recent.ID); err != nil {
		t.Fatal(err)
	}
	m.command.SetValue("npm run build")
	m.command.SetCursor(4)
	m.homeCategory, m.cursor = "SSH", 2
	m = openSwitcher(t, m)
	if m.projects.items[0].ID != recent.ID {
		t.Fatal("recent project was not listed first")
	}
	for query, id := range map[string]string{"ALPHA": first.ID, first.Root: first.ID, "api": recent.ID} {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = next.(Model)
		m.projects.input.SetValue("")
		next, _ = m.Update(tea.PasteMsg{Content: query})
		m = next.(Model)
		items := m.matchingProjects()
		if len(items) != 1 || items[0].ID != id {
			t.Fatalf("query %q did not match name, path or tag", query)
		}
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != home || m.command.Value() != "npm run build" || m.command.Position() != 4 || m.homeCategory != "SSH" || m.cursor != 2 {
		t.Fatal("cancel lost the Home draft or menu selection")
	}
}

func TestProjectSwitchChangesDirectoryPreservesOutputAndRejectsOldContext(t *testing.T) {
	m := switcherModel(t)
	p := registerSwitchProject(t, m, "outside-project")
	oldDir := m.opts.Terminal.Dir
	m.opts.Context = func(_ context.Context, dir string) ProjectContext {
		if dir != p.Root {
			t.Fatalf("context query used wrong directory: %s", dir)
		}
		return ProjectContext{Project: p.Name, Branch: "main", Changes: 2}
	}
	m.opts.Terminal.History = []string{"echo saved"}
	m.command.SetValue("git status")
	m.shellResults = []shellResult{{line: "echo saved", dir: oldDir, output: "saved output"}}
	m = openSwitcher(t, m)
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != home || m.opts.Terminal.Dir != p.Root || m.opts.Terminal.PreviousDir != oldDir {
		t.Fatal("project switch did not update the shell session")
	}
	wd, _ := os.Getwd()
	if wd != p.Root || m.command.Value() != "git status" || len(m.shellResults) != 1 || m.opts.Terminal.History[0] != "echo saved" {
		t.Fatal("switch lost directory, draft, output, or history")
	}
	if m.showingShellOutput() {
		t.Fatal("old output obscures the new project context")
	}
	m, _ = runCmd(m, cmd)
	if m.projectContext.Branch != "main" || m.projectContext.Changes != 2 {
		t.Fatal("Git context was not refreshed")
	}
	next, _ := m.Update(contextMsg{dir: oldDir, context: ProjectContext{Project: "stale", Branch: "old"}})
	m = next.(Model)
	if m.projectContext.Project != p.Name {
		t.Fatal("late Git response replaced the new project context")
	}
	stored, err := m.opts.Workspace.Find(m.ctx, p.ID, false)
	if err != nil || stored.LastOpenedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("last-opened time was not persisted")
	}
	m, _ = press(m, tea.KeyF2)
	if !m.shellHistory || m.shellHidden || !strings.Contains(m.View().Content, "echo saved") {
		t.Fatal("previous command output is no longer accessible")
	}
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
		process := platform.ShellCommand{Context: m.ctx, Session: m.opts.Terminal, Line: "cd -", Capture: true}
		canonical, _ := workspace.Canonical(oldDir)
		if err := process.Run(); err != nil || m.opts.Terminal.Dir != canonical {
			t.Fatalf("cd - could not return: got %q, want %q, previous %q, error %v", m.opts.Terminal.Dir, canonical, m.opts.Terminal.PreviousDir, err)
		}
	}
}

func TestProjectSwitcherMissingDirectoryAndRunningCommand(t *testing.T) {
	m := switcherModel(t)
	p := registerSwitchProject(t, m, "disconnected-drive")
	if err := os.Remove(p.Root); err != nil {
		t.Fatal(err)
	}
	oldDir := m.opts.Terminal.Dir
	m = openSwitcher(t, m)
	m, _ = press(m, tea.KeyEnter)
	wd, _ := os.Getwd()
	if m.screen != projectPicker || m.opts.Terminal.Dir != oldDir || wd != oldDir || !strings.Contains(m.toolbar(), "Cannot open project") {
		t.Fatal("unavailable directory changed context or hid the error")
	}
	m, _ = press(m, tea.KeyEscape)
	m.shellJob = &shellJob{cancel: func() { t.Fatal("switcher canceled a running command") }}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.screen != home || cmd != nil || !strings.Contains(m.shellNotice, "Stop the running command") {
		t.Fatal("project switch was allowed while a command was running")
	}
}

func TestProjectSwitcherSlashCommandAndAddFromHomePath(t *testing.T) {
	m := switcherModel(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	path := filepath.Join(homeDir, "folder with spaces")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	m = typeCommand(m, "/switch")
	m, cmd := press(m, tea.KeyEnter)
	if cmd == nil || m.screen != projectPicker {
		t.Fatal("/switch did not open picker")
	}
	m, _ = runCmd(m, m.loadProjects())
	m, _ = press(m, tea.KeyEnter)
	if m.screen != projectAdd {
		t.Fatal("empty picker does not offer project registration")
	}
	next, _ := m.Update(tea.PasteMsg{Content: "~/folder with spaces"})
	m = next.(Model)
	m, _ = press(m, tea.KeyEnter)
	next, _ = m.Update(tea.PasteMsg{Content: "custom-alias"})
	m = next.(Model)
	m, cmd = press(m, tea.KeyEnter)
	if !m.projects.busy || cmd == nil {
		t.Fatal("registration did not run asynchronously")
	}
	m, follow := runCmd(m, cmd)
	if m.screen != home || m.projectContext.Project != "custom-alias" {
		t.Fatalf("registered project was not selected: %s", m.projects.notice)
	}
	if follow != nil {
		m, _ = runCmd(m, follow)
	}
	p, err := m.opts.Workspace.Find(m.ctx, "custom-alias", false)
	canonical, _ := workspace.Canonical(path)
	if err != nil || p.Root != canonical || m.opts.Terminal.Dir != canonical {
		t.Fatalf("tilde or spaces were not preserved: %+v %v", p, err)
	}
	if strings.Contains(m.contextView(), "Clean") || strings.Contains(m.contextView(), "├─") {
		t.Fatal("non-Git project shows false Git status")
	}
}

func TestProjectSwitcherLoadFailureCancelAndStaleResults(t *testing.T) {
	m := switcherModel(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	request := m.projects.request
	m, _ = press(m, tea.KeyEscape)
	m = openSwitcher(t, m)
	next, _ = m.Update(projectsLoadedMsg{request: request, items: []workspace.Project{{Name: "stale"}}})
	m = next.(Model)
	if len(m.projects.items) != 0 {
		t.Fatal("accepted results from a canceled picker")
	}
	next, _ = m.Update(projectsLoadedMsg{request: m.projects.request, err: errors.New("invalid workspace schema")})
	m = next.(Model)
	if m.projects.busy || !strings.Contains(m.toolbar(), "invalid workspace schema") {
		t.Fatal("load error was not visible or recoverable")
	}
}

func TestProjectSwitcherCanSearchWhileLoading(t *testing.T) {
	m := switcherModel(t)
	p := registerSwitchProject(t, m, "backend")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	m = next.(Model)
	next, _ = m.Update(tea.PasteMsg{Content: "back"})
	m = next.(Model)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != projectPicker || !m.projects.busy {
		t.Fatal("Enter submitted before the project list loaded")
	}
	m, _ = runCmd(m, m.loadProjects())
	items := m.matchingProjects()
	if m.projects.input.Value() != "back" || len(items) != 1 || items[0].ID != p.ID {
		t.Fatal("loading discarded the search entered by the user")
	}
}

func TestProjectSwitcherRegistersCurrentFolder(t *testing.T) {
	for _, alias := range []string{"", "my-project"} {
		t.Run("alias="+alias, func(t *testing.T) {
			m := switcherModel(t)
			dir := m.opts.Terminal.Dir
			m.opts.Terminal.PreviousDir = "previous-directory"
			m = openSwitcher(t, m)
			m.projects.input.SetValue("unmatched search")
			m, _ = press(m, tea.KeyDown)
			if !strings.Contains(m.projectSwitchView(), "> Add current folder as project") {
				t.Fatal("current folder registration is not selectable")
			}
			m, _ = press(m, tea.KeyEnter)
			if m.screen != projectAdd || m.projects.input.Value() != dir || m.projects.name.Value() != filepath.Base(dir) || m.projects.focus != 1 {
				t.Fatal("current folder and name were not prefilled with name focused")
			}
			m, _ = press(m, tea.KeyEscape)
			items, err := m.opts.Workspace.List(m.ctx, false)
			if err != nil || len(items) != 0 || m.projects.input.Value() != "unmatched search" {
				t.Fatal("cancel registered the folder or discarded the search")
			}
			m, _ = press(m, tea.KeyEnter)
			name := filepath.Base(dir)
			if alias != "" {
				m.projects.name.SetValue(alias)
				name = alias
			}
			m, cmd := press(m, tea.KeyEnter)
			m, follow := runCmd(m, cmd)
			m, _ = runCmd(m, follow)
			stored, err := m.opts.Workspace.Find(m.ctx, name, false)
			wd, _ := os.Getwd()
			canonical, _ := workspace.Canonical(dir)
			wd, _ = workspace.Canonical(wd)
			if err != nil || stored.Root != canonical || m.screen != home || m.projectContext.Project != name || wd != canonical || m.opts.Terminal.PreviousDir != "previous-directory" {
				t.Fatalf("current folder registration changed directory or failed: %+v, %v", stored, err)
			}
			m.opts.Terminal.Dir = dir
			m = openSwitcher(t, m)
			m.projects.input.SetValue("unmatched search")
			m, _ = press(m, tea.KeyDown)
			if strings.Contains(m.projectSwitchView(), "Add current folder") || m.projects.cursor != 0 {
				t.Fatal("registered current folder can still be added from filtered results")
			}
		})
	}
}

func TestProjectSwitcherAddErrorKeepsFieldsAndAllowsCorrection(t *testing.T) {
	m := switcherModel(t)
	registerSwitchProject(t, m, "backend")
	m = openSwitcher(t, m)
	m.projects.input.SetValue("new project")
	m, _ = press(m, tea.KeyEnter)
	path := t.TempDir()
	m.projects.input.SetValue(path)
	m, _ = press(m, tea.KeyEnter)
	m.projects.name.SetValue("backend")
	m, cmd := press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	if m.screen != projectAdd || m.projects.busy || m.projects.input.Value() != path || m.projects.name.Value() != "backend" || !strings.Contains(m.toolbar(), "choose another name") {
		t.Fatal("duplicate name error lost form values or hid its recovery")
	}
	for _, size := range [][2]int{{24, 12}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		if !strings.Contains(view, "backend") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("focused name or error form clipped at %v: %q", size, view)
		}
	}
	m.projects.name.SetValue("new-backend")
	m, cmd = press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	if m.screen != home || m.projectContext.Project != "new-backend" {
		t.Fatalf("corrected name could not be submitted: %s", m.projects.notice)
	}
}

func TestProjectSwitcherViewsFitAndKeepSelectedProjectVisible(t *testing.T) {
	m := switcherModel(t)
	m = openSwitcher(t, m)
	for i := range 50 {
		m.projects.items = append(m.projects.items, workspace.Project{Name: fmt.Sprintf("project-%02d", i), Root: "/projects/" + strings.Repeat("目录/", 20)})
	}
	for _, size := range [][2]int{{24, 12}, {40, 12}, {80, 24}, {120, 30}} {
		m.width, m.height = size[0], size[1]
		m.projects.cursor = 49
		view := m.View().Content
		if !strings.Contains(view, "project-49") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || strings.Contains(view, "38;") || strings.Contains(view, "48;") {
			t.Fatalf("%v selected project clipped or unsafe: %q", size, view)
		}
		m.projects.cursor = 50
		view = m.View().Content
		if !strings.Contains(view, "Add project") {
			t.Fatal("Add project action is inaccessible")
		}
		m.projects.cursor = 51
		view = m.View().Content
		if !strings.Contains(view, "Add current") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("current folder action is inaccessible at %v: %q", size, view)
		}
	}
}
