package developer

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
)

func TestSyncShortcutsReviewBeforeTerminalHandoff(t *testing.T) {
	for _, shortcut := range []struct {
		key  rune
		text string
	}{{'p', "push"}, {'P', "pull --ff-only"}, {'f', "fetch --all --prune"}} {
		m := actionModel(t, 0)
		m, _ = press(m, shortcut.key)
		if m.dialog.kind != "native" || m.gitInput.Value() != shortcut.text || m.gitBusy {
			t.Fatalf("shortcut %c did not open the expected command", shortcut.key)
		}
		m, cmd := actionKey(m, tea.KeyEnter)
		if cmd != nil || m.dialog.kind != "native-confirm" || m.gitBusy {
			t.Fatal("command executed before its review")
		}
		m, cmd = actionKey(m, tea.KeyEnter)
		if cmd == nil || !m.gitBusy {
			t.Fatal("reviewed command was not prepared asynchronously")
		}
		ready, ok := cmd().(gitNativeReady)
		if !ok || ready.err != nil || ready.cmd == nil {
			t.Fatalf("could not prepare native Git: %+v", ready)
		}
		if !strings.Contains(strings.Join(ready.cmd.Args, " "), shortcut.text) {
			t.Fatal("native command did not preserve arguments")
		}
		next, handoff := m.Update(ready)
		if handoff == nil || !next.(Model).gitBusy {
			t.Fatal("native process was not handed terminal ownership")
		}
	}
}

func TestActionCatalogSearchAndScrollingReachEveryCommand(t *testing.T) {
	for index, action := range gitActions {
		m := NewModel(context.Background(), Options{Mode: "git", Width: 40, Height: 15, NoColor: true})
		m.loading = false
		next, _ := m.openGitDialog("menu", "")
		m = next.(Model)
		m.dialog.category = action.category
		for position, match := range m.matchingActions() {
			if match == index {
				m.dialog.cursor = position
				break
			}
		}
		view := m.View().Content
		if !strings.Contains(view, "> ") || !strings.Contains(view, "esc back") || lipgloss.Width(view) > 40 || lipgloss.Height(view) > 15 {
			t.Fatalf("menu action %d unreachable or clipped: %q", index, view)
		}
		m.gitInput.SetValue(actionTitle(action))
		m.dialog.cursor = 0
		indices := m.matchingActions()
		found := false
		for position, match := range indices {
			if match == index {
				m.dialog.cursor = position
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("action search did not find %q: %v", action.label, indices)
		}
		if action.kind == "native" {
			m, _ = actionKey(m, tea.KeyEnter)
			if m.dialog.kind != "native" || m.gitInput.Value() != action.command || m.gitBusy {
				t.Fatalf("menu action %q executed without input/review", action.label)
			}
		}
	}
}

func TestDestructiveCommandRequiresTypedConfirmationAndCanBeEdited(t *testing.T) {
	m := actionModel(t, 0)
	next, _ := m.openNativeAction(gitAction{label: "Hard reset", command: "reset --hard HEAD"})
	m = next.(Model)
	m, _ = actionKey(m, tea.KeyEnter)
	m, cmd := actionKey(m, tea.KeyEnter)
	if cmd != nil || m.gitBusy || m.dialog.err == "" || m.dialog.warning == "" {
		t.Fatal("destructive command ran without typed confirmation")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if m.dialog.kind != "native" || m.gitInput.Value() != "reset --hard HEAD" {
		t.Fatal("editing command discarded its original input")
	}
	m, _ = actionKey(m, tea.KeyEnter)
	m.gitInput.SetValue("confirm")
	m, cmd = actionKey(m, tea.KeyEnter)
	if cmd == nil || !m.gitBusy {
		t.Fatal("explicitly confirmed command was blocked")
	}
	for _, command := range []string{"push --force-with-lease", "stash drop stash@{0}", "branch --delete old", "clean --force -d", "restore --worktree -- .", "commit --amend"} {
		args, _ := gitlocal.ParseCommand(command)
		if commandWarning(args) == "" {
			t.Fatalf("missing explanation for %q", command)
		}
	}
}

func TestNativeReviewKeepsConfirmationVisibleForLongCommands(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 15}} {
		m := NewModel(context.Background(), Options{Mode: "git", NoColor: true, Width: size[0], Height: size[1]})
		m.loading = false
		m.dialog = gitDialog{kind: "native-confirm", args: []string{"reset", "--hard", strings.Repeat("very-long-ref-", 30)}, warning: "May replace files. Type confirm.", err: "Type confirm to continue."}
		m.gitInput.Placeholder = "Type confirm to continue"
		view := m.View().Content
		if !strings.Contains(view, "> Type confirm") || !strings.Contains(view, "esc edit") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("long review hid controls: %q", view)
		}
		m, _ = actionKey(m, tea.KeyDown)
		if m.dialog.offset != 1 {
			t.Fatal("long command review cannot scroll")
		}
	}
}

func TestTerminalProcessKeepsOutputReadableAndReturnsFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	for _, fail := range []bool{false, true} {
		args := []string{"--version"}
		if fail {
			args = []string{"definitely-not-a-git-command"}
		}
		process := &gitTerminalProcess{ctx: context.Background(), cmd: exec.Command("git", args...), args: args}
		var output bytes.Buffer
		process.SetStdin(strings.NewReader("\n"))
		process.SetStdout(&output)
		process.SetStderr(&output)
		err := process.Run()
		if (err != nil) != fail || !strings.Contains(output.String(), "Press Enter to return to AIAI") {
			t.Fatalf("terminal result %v: %s", err, output.String())
		}
	}
}
