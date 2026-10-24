package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

func TestActiveDirectoryStaysVisibleWithHistoricalOutput(t *testing.T) {
	m := NewModel(context.Background(), nil, nil, nil, nil, nil, Options{NoColor: true, Terminal: &platform.ShellSession{Dir: "/projects/BETA"}})
	defer m.cancel()
	m.projectContext = ProjectContext{Project: "BETA"}
	m.shellResults = []shellResult{{line: "pwd", dir: "/projects/ALPHA", output: "/projects/ALPHA"}}
	for _, size := range [][2]int{{24, 12}, {40, 12}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		for _, history := range []bool{true, false} {
			m.shellHistory = history
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "Current: BETA") || !strings.Contains(view, "Dir: /projects/BETA") || lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("current working directory lost with history=%v at %v: %q", history, size, view)
			}
			if size[0] == 80 && !strings.Contains(view, "Ran in: /projects/ALPHA") {
				t.Fatal("old result directory is not labeled separately")
			}
		}
	}
}

func TestHomeMenuAfterSwitchUsesArrowsAndAccurateEscapeHint(t *testing.T) {
	m := switcherModel(t)
	registerSwitchProject(t, m, "beta")
	m.opts.Terminal.History = []string{"echo previous"}
	m.historyCursor = 1
	m.shellResults = []shellResult{{line: "echo previous", dir: m.opts.Terminal.Dir}}
	m = openSwitcher(t, m)
	m, cmd := press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	m, _ = press(m, tea.KeyDown)
	if m.cursor != 1 || m.command.Value() != "" {
		t.Fatal("visible menu arrows still recall command history")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.command.Value() != "echo previous" || !strings.Contains(m.footer(), "esc clear") {
		t.Fatal("history recall or draft-clear hint is incorrect")
	}
	m, _ = press(m, tea.KeyEscape)
	if !strings.Contains(m.footer(), "esc quit") {
		t.Fatal("menu escape hint is incorrect")
	}
	_, cmd = press(m, tea.KeyEscape)
	if cmd == nil {
		t.Fatal("Esc did not quit the root menu")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Esc opened another menu despite the quit hint")
	}
}

func TestLongProjectSearchStaysOnOneLineAfterResizing(t *testing.T) {
	m := switcherModel(t)
	m = openSwitcher(t, m)
	query := "/projects/" + strings.Repeat("目录/nested-directory/", 12) + "backend"
	next, _ := m.Update(tea.PasteMsg{Content: query})
	m = next.(Model)
	m.projects.input.SetCursor(18)
	for _, size := range [][2]int{{24, 12}, {40, 12}, {80, 24}, {120, 30}} {
		next, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		view := ansi.Strip(m.View().Content)
		if lipgloss.Height(m.toolbar()) != 2 || lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height || !strings.Contains(view, "> Search: ") || !strings.Contains(view, "> Add project") {
			t.Fatalf("long search hid the list or overflowed at %v: %q", size, view)
		}
		if m.projects.input.Value() != query || m.projects.input.Position() != 18 {
			t.Fatal("resizing changed the query or cursor position")
		}
	}
}

func TestInitValidationRemainsVisibleWhileFormScrolls(t *testing.T) {
	m := NewModel(context.Background(), nil, nil, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	m.screen, m.focus = form, 5
	m.inputs[0].SetValue("invalid/name")
	m, _ = press(m, tea.KeyEnter)
	if m.focus != 0 {
		t.Fatal("invalid name did not return focus to the name field")
	}
	for _, size := range [][2]int{{24, 12}, {40, 12}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "project name") || !strings.Contains(view, "> Project name") || lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("validation or invalid field disappeared at %v: %q", size, view)
		}
	}
	m.focus = 5
	next, _ := m.Update(plannedMsg{err: errors.New("Destination is not writable")})
	m = next.(Model)
	if !strings.Contains(m.View().Content, "Destination is not writable") {
		t.Fatal("asynchronous planning error scrolled outside the form")
	}
}

func TestAddProjectLongPathKeepsErrorCauseVisible(t *testing.T) {
	m := switcherModel(t)
	m = openSwitcher(t, m)
	m, _ = press(m, tea.KeyEnter)
	parent := filepath.Join(t.TempDir(), strings.Repeat("long-existing-directory/", 4))
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "missing")
	_, err := workspace.Canonical(path)
	var pathError *os.PathError
	if !errors.As(err, &pathError) {
		t.Fatalf("missing path did not produce a filesystem error: %v", err)
	}
	next, _ := m.Update(tea.PasteMsg{Content: path})
	m = next.(Model)
	m, _ = press(m, tea.KeyEnter)
	m, cmd := press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	for _, size := range [][2]int{{40, 12}, {80, 24}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(strings.Join(strings.Fields(view), " "), pathError.Err.Error()) || lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height || m.projects.input.Value() != path {
			t.Fatalf("missing-directory cause or form value lost at %v: %q", size, view)
		}
	}
}
