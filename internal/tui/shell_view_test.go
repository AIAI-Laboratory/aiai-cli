package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestShellResultsStaySeparateAndCanBeSelected(t *testing.T) {
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	m.shellResults = []shellResult{
		{line: "ls", dir: "/project", output: "Documents\nMusic\n", elapsed: 20 * time.Millisecond},
		{line: "pwd", dir: "/project", output: "/project\n", elapsed: 10 * time.Millisecond},
	}
	m.shellSelected = 1
	if strings.Contains(m.content(), "Documents") || !strings.Contains(m.shellHeader(), "pwd") {
		t.Fatal("results from different commands were mixed together")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	m = next.(Model)
	if m.shellSelected != 0 || !strings.Contains(m.content(), "Documents") || !strings.Contains(m.shellHeader(), "Run 1 / 2") {
		t.Fatal("Alt+Left did not select the previous result")
	}
	m, _ = press(m, tea.KeyF2)
	if !m.shellHistory || !strings.Contains(m.content(), "pwd") || !strings.Contains(m.content(), "ls") {
		t.Fatal("F2 did not show the list of command runs")
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.shellHistory || m.shellSelected != 1 || strings.Contains(m.content(), "Documents") {
		t.Fatal("selecting a run did not isolate its output")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.showingShellOutput() || len(m.shellResults) != 2 {
		t.Fatal("returning to menus deleted the results")
	}
	m, _ = press(m, tea.KeyF2)
	if !m.shellHistory || !m.showingShellOutput() {
		t.Fatal("history was not accessible again from menus")
	}
}

func TestShellListingGridPreservesNamesAndFitsTerminal(t *testing.T) {
	root := t.TempDir()
	names := []string{"Desktop", "Documents", "Music", "My Project", "资料", strings.Repeat("long report ", 8) + ".txt"}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	output := strings.Join(names, "\n") + "\n"
	files := shellListing("ls", root, output)
	if len(files) != len(names) || shellListing("ls -l", root, output) != nil || shellListing("ls | cat", root, output) != nil {
		t.Fatal("listing detection changed other command formats")
	}
	for _, size := range [][2]int{{32, 12}, {80, 24}, {160, 40}} {
		m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{NoColor: true})
		m.width, m.height, m.shellFollow = size[0], size[1], true
		m.shellResults = []shellResult{{line: "ls", dir: root, output: output, files: files}}
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(view, "❯ ls") || !strings.Contains(view, "Done") {
			t.Fatalf("command/status hidden or view overflow at %dx%d:\n%s", size[0], size[1], view)
		}
		if strings.LastIndex(view, "❯") < strings.Index(view, "Done") {
			t.Fatal("input is above the result")
		}
		grid := shellFileGrid(files, size[0]-6, m.theme())
		if lipgloss.Width(grid) > size[0]-6 || !strings.Contains(strings.ReplaceAll(grid, "\n", ""), names[5]) {
			t.Fatal("wide or spaced file name was lost")
		}
		if size[0] >= 80 && !strings.Contains(strings.Split(grid, "\n")[0], "Documents/") {
			t.Fatal("listing was still a single long column")
		}
		m.shellResults[0] = shellResult{line: "npm run build", output: strings.Repeat("build log\n", 100)}
		view = m.View().Content
		if size[0] >= 80 && (!strings.Contains(view, "npm run build") || !strings.Contains(view, "Done")) {
			t.Fatal("long output scrolled the command header out of view")
		}
		m.cancel()
	}
}
