package developer

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

func press(m Model, key rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return next.(Model), cmd
}

func TestResponsiveWorkspaceAndGitViews(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 15}, {20, 8}} {
		for _, mode := range []string{"", "git"} {
			m := NewModel(context.Background(), Options{Width: size[0], Height: size[1], NoColor: true, Mode: mode, Page: 1})
			m.loading = false
			m.items = []workspace.Overview{{Project: workspace.Project{ID: "id", Name: "aiai", Root: "/a/very/long/path/" + strings.Repeat("project", 30)}, Available: true, Git: &gitlocal.Status{Branch: "main"}}}
			m.rows = []string{strings.Repeat("changed-file", 50)}
			m.status.Changes = []gitlocal.Change{{Path: "file"}}
			m.diff = "+" + strings.Repeat("added content ", 50) + "\n-previous"
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s %dx%d became %dx%d: %q", mode, size[0], size[1], lipgloss.Width(view), lipgloss.Height(view), view)
			}
			if strings.Contains(view, "\x1b") {
				t.Fatal("NoColor emitted escapes")
			}
		}
	}
}

func TestSearchDoesNotExecuteShortcuts(t *testing.T) {
	m := NewModel(context.Background(), Options{})
	m.loading = false
	m.items = []workspace.Overview{{Project: workspace.Project{ID: "1", Name: "example"}, Available: true}, {Project: workspace.Project{ID: "2", Name: "z-project"}, Available: true}}
	m, cmd := press(m, '/')
	if cmd == nil || !m.searching {
		t.Fatal("search did not focus")
	}
	m, _ = press(m, 'z')
	if m.action.Focus != nil || m.search.Value() != "z" || len(m.visible()) != 1 {
		t.Fatalf("shortcut intercepted search %+v", m)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.searching {
		t.Fatal("search did not blur")
	}
	m, cmd = press(m, 'z')
	if cmd == nil || m.action.Focus == nil || m.action.Focus.ID != "2" {
		t.Fatal("project focus not routed")
	}
}

func TestLoadingErrorHelpAndStaleMessages(t *testing.T) {
	m := NewModel(context.Background(), Options{})
	if !strings.Contains(m.View().Content, "Loading") {
		t.Fatal("loading state missing")
	}
	next, _ := m.Update(loaded{token: 0, err: errors.New("stale")})
	m = next.(Model)
	if m.err != nil {
		t.Fatal("accepted stale response")
	}
	next, _ = m.Update(loaded{token: 1, err: errors.New("unavailable")})
	m = next.(Model)
	if !strings.Contains(m.View().Content, "unavailable") {
		t.Fatal("error state missing")
	}
	m, _ = press(m, '?')
	if !m.help || !strings.Contains(m.View().Content, "Read-only") {
		t.Fatal("help missing")
	}
	m, _ = press(m, '?')
	if m.help {
		t.Fatal("help did not close")
	}
}

func TestGitNavigationAndSafeRendering(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: "git", NoColor: true})
	m.loading = false
	m.rows = []string{"subject\x1b]52;clipboard\x07"}
	if strings.ContainsAny(m.View().Content, "\x1b\x07") {
		t.Fatal("untrusted terminal escapes rendered")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if cmd == nil || m.opts.Page != 1 || !m.loading {
		t.Fatal("Git page did not load asynchronously")
	}
}

func TestFilteredDiffUsesSelectedFileAndKeepsFailuresInPreview(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: "git", Page: 1})
	m.search.SetValue("second")
	next, cmd := m.Update(loaded{token: 1, rows: []string{"first", "second"}, status: gitlocal.Status{Changes: []gitlocal.Change{{Path: "first"}, {Path: "second", Untracked: true}}}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("selected preview was not loaded lazily")
	}
	msg := cmd().(diffMsg)
	if !strings.Contains(msg.text, "Untracked") {
		t.Fatal("preview did not follow filtered selection")
	}
	next, _ = m.Update(diffMsg{token: m.token, err: errors.New("diff too large")})
	m = next.(Model)
	if m.err != nil || m.diff != "diff too large" {
		t.Fatal("one preview failure hid the file list")
	}
}
