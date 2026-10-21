package developer

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGitMenuCategoriesAndReturningFromAnAction(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: "git", NoColor: true})
	m.loading = false
	next, _ := m.openGitDialog("menu", "")
	m = next.(Model)
	items := m.gitMenuItems()
	if len(items) != len(gitCategories) || len(items) >= len(gitActions) {
		t.Fatal("root menu did not group actions into categories")
	}
	syncCursor := -1
	for i, item := range items {
		if item.category == "Sync" {
			syncCursor = i
		}
		if item.category == "" || item.action != -1 {
			t.Fatal("root menu contains a command instead of a category")
		}
	}
	if syncCursor < 0 {
		t.Fatal("Sync category missing")
	}
	m.dialog.cursor = syncCursor
	m, cmd := actionKey(m, tea.KeyEnter)
	if cmd != nil || m.dialog.kind != "menu" || m.dialog.category != "Sync" {
		t.Fatal("opening a category executed an operation")
	}
	pushCursor := -1
	for i, item := range m.gitMenuItems() {
		action := gitActions[item.action]
		if action.category != "Sync" || strings.Contains(item.label, "Sync /") {
			t.Fatal("category view repeats prefixes or contains unrelated actions")
		}
		if action.command == "push" {
			pushCursor = i
		}
	}
	if pushCursor < 0 {
		t.Fatal("push is not in Sync")
	}
	m.dialog.cursor = pushCursor
	m, _ = actionKey(m, tea.KeyEnter)
	if m.dialog.kind != "native" || m.gitInput.Value() != "push" {
		t.Fatal("selected action did not open its form")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if m.dialog.kind != "menu" || m.dialog.category != "Sync" || m.dialog.cursor != pushCursor {
		t.Fatal("cancelled form did not restore category and selection")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if m.dialog.kind != "menu" || m.dialog.category != "" || m.dialog.cursor != syncCursor {
		t.Fatal("back did not restore category selection")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if m.dialog.kind != "" {
		t.Fatal("root menu did not close")
	}
}

func TestGitMenuSearchFindsActionsAcrossCategories(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: "git", NoColor: true})
	m.loading = false
	next, _ := m.openGitDialog("menu", "")
	m = next.(Model)
	m.dialog.category = "Changes"
	m, _ = press(m, '/')
	for _, char := range "push" {
		m, _ = press(m, char)
	}
	m, _ = actionKey(m, tea.KeyEnter)
	found := false
	for _, item := range m.gitMenuItems() {
		if gitActions[item.action].command == "push" {
			found = true
			if item.label != "Sync / Push" {
				t.Fatal("global search result lost category context")
			}
		}
	}
	if !found || !strings.Contains(m.View().Content, "Search all actions") {
		t.Fatal("search was restricted to the open category")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if m.dialog.category != "Changes" || m.gitInput.Value() != "" || len(m.gitMenuItems()) == 0 {
		t.Fatal("clearing search did not return to the category")
	}
}

func TestEveryGitActionBelongsToExactlyOneCategory(t *testing.T) {
	for _, action := range gitActions {
		count := 0
		for _, category := range gitCategories {
			if action.category == category {
				count++
			}
		}
		if count != 1 || action.label == "" {
			t.Fatalf("action has no unique category: %+v", action)
		}
	}
}
