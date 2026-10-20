package developer

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
)

func changeViewModel() Model {
	m := NewModel(context.Background(), Options{Mode: "git", Page: 1, NoColor: true})
	m.loading = false
	m.status.Changes = []gitlocal.Change{{Path: "internal/tui/developer/model.go", Worktree: "M", Index: "."}, {Path: "docs/new file.md", Untracked: true}, {Path: "old.go", Index: "D"}}
	for _, c := range m.status.Changes {
		m.rows = append(m.rows, c.Path)
	}
	m.diff = "diff --git a/model.go b/model.go\nindex abc..def 100644\n--- a/model.go\n+++ b/model.go\n@@ -1,2 +1,2 @@\n-before\n+after\n context\n"
	return m
}

func TestChangesHaveReadableLabelsDividerAndFocusedPane(t *testing.T) {
	m := changeViewModel()
	view := m.diffView(116, 18)
	for _, want := range []string{"> FILES", "Modified", "New", "Deleted", "model.go", " │ ", "DIFF  +1 −1", "@@ -1,2 +1,2 @@", "-before", "+after"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	for _, noise := range []string{"diff --git", "index abc", `"internal/`, ".M"} {
		if strings.Contains(view, noise) {
			t.Fatalf("raw metadata %q remains", noise)
		}
	}
	m.preview = true
	view = m.diffView(56, 12)
	if !strings.Contains(view, "> DIFF") || strings.Contains(view, "FILES") {
		t.Fatal("narrow preview did not get focus")
	}
	for _, size := range [][2]int{{160, 25}, {116, 18}, {90, 12}, {76, 12}, {56, 10}, {36, 7}} {
		for _, preview := range []bool{false, true} {
			m.preview = preview
			view = m.diffView(size[0], size[1])
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("diff %dx%d rendered %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestDiffDoesNotHideContentResemblingMetadata(t *testing.T) {
	lines, added, removed := diffContent("diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n--- content\n+++ content\n")
	text := strings.Join(lines, "\n")
	if added != 1 || removed != 1 || !strings.Contains(text, "--- content") || !strings.Contains(text, "+++ content") || strings.Contains(text, "--- a/a") {
		t.Fatalf("content lost %q +%d -%d", text, added, removed)
	}
	lines, _, _ = diffContent("Binary files a/a and b/a differ")
	if !strings.Contains(lines[0], "Binary") {
		t.Fatal("binary indication lost")
	}
}

func TestPathLabelsAreSafeAndKeepFilename(t *testing.T) {
	if got := pathTail("internal/tui/developer/model.go", 18); !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "model.go") {
		t.Fatal(got)
	}
	got := pathLabel("日本語\nfile.go")
	if strings.ContainsRune(got, '\n') || !strings.Contains(got, `\n`) {
		t.Fatalf("unsafe filename %q", got)
	}
	for _, width := range []int{1, 2, 6, 10} {
		if got := pathTail("very/long/日本語.go", width); lipgloss.Width(got) > width {
			t.Fatalf("width %d: %q", width, got)
		}
	}
}

func TestHistoryAlignsAuthorsGroupsDaysAndAdapts(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: "git", Page: 2, NoColor: true})
	m.loading = false
	now := time.Date(2026, 10, 20, 22, 0, 0, 0, time.Local)
	m.commits = []gitlocal.Commit{{Hash: "12345678abcdef", Date: now, Subject: "Short subject", Author: "Ann"}, {Hash: "98765432abcdef", Date: now, Subject: strings.Repeat("long message ", 15), Author: "Example Author"}, {Hash: "abcdef12345678", Date: now.AddDate(0, 0, -1), Subject: "Yesterday", Author: "Other"}}
	for _, c := range m.commits {
		m.rows = append(m.rows, c.Subject+" "+c.Author)
	}
	view := m.historyView(116, 18)
	if !strings.Contains(view, "DATE / TIME") || !strings.Contains(view, "MESSAGE") || !strings.Contains(view, "AUTHOR") || !strings.Contains(view, "\n\n") {
		t.Fatal("history lacks columns/day separation")
	}
	authorColumn := -1
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Short subject") {
			authorColumn = ansi.StringWidth(line[:strings.Index(line, "Ann")])
		}
		if strings.Contains(line, "Example Author") && authorColumn != ansi.StringWidth(line[:strings.Index(line, "Example Author")]) {
			t.Fatalf("author columns differ: %s", view)
		}
	}
	for _, size := range [][2]int{{116, 18}, {96, 18}, {76, 12}, {56, 10}, {36, 7}} {
		view = m.historyView(size[0], size[1])
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("history %dx%d overflow", size[0], size[1])
		}
		if size[0] < 70 && (!strings.Contains(view, "Short subject") || strings.Contains(view, "AUTHOR")) {
			t.Fatal("narrow history is not compact")
		}
	}
	m.search.SetValue("Yesterday")
	if view = m.historyView(116, 18); !strings.Contains(view, "Yesterday") || strings.Contains(view, "Short subject") {
		t.Fatal("history search ignored")
	}
}
