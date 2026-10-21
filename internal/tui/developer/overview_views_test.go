package developer

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
)

func readableModel() Model {
	m := NewModel(context.Background(), Options{Mode: "git", NoColor: true})
	m.loading = false
	now := time.Now()
	hash := strings.Repeat("abcdef01", 5)
	m.status = gitlocal.Status{Name: "aiai-cli", Root: "/projects/aiai-cli", Branch: "feature/readable", Head: hash, Upstream: "origin/feature/readable", Staged: 2, Unstaged: 3, Untracked: 1, Changes: []gitlocal.Change{{Path: "main.go", Index: "M", Worktree: "."}}, LastCommit: &gitlocal.Commit{Hash: hash, Subject: "Make the interface easier to scan", Author: "Example", Date: now}}
	m.rows = []string{"Make the interface easier to scan Example"}
	m.commits = []gitlocal.Commit{*m.status.LastCommit}
	m.branches = []gitlocal.Branch{{Name: m.status.Branch, Current: true, Head: hash, Upstream: m.status.Upstream}, {Name: "other", Head: hash}}
	m.trees = []gitlocal.Worktree{{Path: m.status.Root, Branch: m.status.Branch, Head: hash}, {Path: "/projects/linked", Detached: true, Head: hash, Locked: "Keep this worktree"}}
	m.activity = &gitlocal.Activity{Commits: m.commits, Days: map[string]int{now.Format("2006-01-02"): 1}}
	return m
}

func TestOverviewUsesLabeledSectionsNotRawGitOutput(t *testing.T) {
	m := readableModel()
	view := m.statusView(76, 30)
	for _, label := range []string{"Working tree", "Branch", "Changes", "Tracking", "local only", "Upstream", "Ahead / behind", "Latest commit", "2 staged", "3 unstaged", "1 new"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %s: %s", label, view)
		}
	}
	if strings.Contains(view, m.status.Head) || strings.Contains(view, "0 conflicts") {
		t.Fatal("verbose raw data remains")
	}
	for _, mutate := range []func(*Model){func(m *Model) { m.status.Unborn = true; m.status.LastCommit = nil; m.status.Upstream = "" }, func(m *Model) { m.status.Detached = true; m.status.Conflicts = 2 }, func(m *Model) { m.status.Changes = nil }} {
		clone := m
		mutate(&clone)
		if got := clone.statusView(36, 30); lipgloss.Width(got) > 36 {
			t.Fatal("status overflow")
		}
	}
}

func TestAllGitPagesKeepFooterAndFitWithoutColor(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 15}} {
		for page := range pages {
			for _, search := range []string{"", "readable"} {
				m := readableModel()
				m.width, m.height = size[0], size[1]
				m.opts.Page = page
				m.search.SetValue(search)
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%v page %d overflow: %q", size, page, view)
				}
				if !strings.Contains(view, "q back") {
					t.Fatalf("%v page %d footer clipped: %q", size, page, view)
				}
				if ansi.Strip(view) != view {
					t.Fatal("NO_COLOR leaked formatting")
				}
				if !strings.Contains(view, "[ "+pages[page]+" ]") {
					t.Fatal("active tab not marked without color")
				}
			}
		}
	}
}

func TestBranchAndWorktreeSearchKeepsContext(t *testing.T) {
	m := readableModel()
	m.search.SetValue("feature")
	if got := m.branchesView(76, 25); !strings.Contains(got, "feature/readable") || !strings.Contains(got, "origin/feature/readable") || strings.Contains(got, "other") {
		t.Fatal(got)
	}
	m.search.SetValue("linked")
	if got := m.worktreesView(76, 25); !strings.Contains(got, "Detached HEAD") || !strings.Contains(got, "Locked:") || strings.Contains(got, "feature/readable") {
		t.Fatal(got)
	}
}

func TestWorkspaceTimelineDoesNotReuseProjectSearchAsEventFilter(t *testing.T) {
	m := readableModel()
	m.opts.Mode = ""
	p := workspace.Project{ID: "p", Name: "app"}
	m.items = []workspace.Overview{{Project: p, Available: true}}
	m.search.SetValue("app")
	m.detail = true
	m.timeline = &workspace.Timeline{Events: []workspace.Event{{ID: "one", Source: "git", Time: time.Now(), Text: "Fix deployment"}}}
	view := m.detailView(76, 20)
	if !strings.Contains(view, "Fix deployment") || !strings.Contains(view, "Git") {
		t.Fatal("project filter hid event context")
	}
	if strings.Contains(view, "\x1b") {
		t.Fatal("unexpected formatting")
	}
}
