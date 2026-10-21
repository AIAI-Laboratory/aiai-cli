package developer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
)

func actionGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = path
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(path, "no-global-config"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func actionModel(t *testing.T, page int) Model {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	path := t.TempDir()
	actionGit(t, path, "init", "--initial-branch=main")
	actionGit(t, path, "config", "user.name", "Local Tester")
	actionGit(t, path, "config", "user.email", "local@example.invalid")
	actionGit(t, path, "config", "commit.gpgsign", "false")
	m := NewModel(context.Background(), Options{Git: &gitlocal.Service{}, Mode: "git", Path: path, Page: page, NoColor: true})
	return actionRefresh(t, m)
}

func actionRefresh(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(m.load()())
	return next.(Model)
}

func actionKey(m Model, code rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return next.(Model), cmd
}

func finishWrite(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil || !m.gitBusy {
		t.Fatal("Git mutation did not run asynchronously")
	}
	result, ok := cmd().(gitWriteDone)
	if !ok || result.err != nil {
		t.Fatalf("Git action failed: %+v", result)
	}
	next, reload := m.Update(result)
	m = next.(Model)
	if reload == nil || !m.loading {
		t.Fatal("Git action did not refresh live data")
	}
	next, _ = m.Update(reload())
	return next.(Model)
}

func TestQuickStageCommitAndFormTyping(t *testing.T) {
	m := actionModel(t, 1)
	file := filepath.Join(m.opts.Path, "file.txt")
	if err := os.WriteFile(file, []byte("content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = actionRefresh(t, m)
	m, cmd := actionKey(m, tea.KeySpace)
	m = finishWrite(t, m, cmd)
	if m.status.Staged != 1 {
		t.Fatal("Space did not stage the selected file")
	}
	m, _ = press(m, 'c')
	for _, char := range "feat: quick a/c/b/n?" {
		m, _ = press(m, char)
	}
	if m.dialog.kind != "commit" || m.gitInput.Value() != "feat: quick a/c/b/n?" || m.searching || m.opts.Page != 1 {
		t.Fatal("form text executed navigation shortcuts")
	}
	m, cmd = actionKey(m, tea.KeyEnter)
	busy, ignored := press(m, 'q')
	if ignored != nil || !busy.gitBusy {
		t.Fatal("navigation interrupted an in-flight Git mutation")
	}
	m = finishWrite(t, m, cmd)
	if m.dialog.kind != "" || m.status.Staged != 0 || m.status.LastCommit == nil || m.status.LastCommit.Subject != "feat: quick a/c/b/n?" || !strings.Contains(m.notice, "Committed") {
		t.Fatalf("commit result not reflected in UI: %+v", m.status)
	}
}

func TestBranchSelectionSearchCancelAndCreate(t *testing.T) {
	m := actionModel(t, 3)
	if err := os.WriteFile(filepath.Join(m.opts.Path, "file"), []byte("base"), 0o600); err != nil {
		t.Fatal(err)
	}
	actionGit(t, m.opts.Path, "add", "file")
	actionGit(t, m.opts.Path, "commit", "-m", "base")
	actionGit(t, m.opts.Path, "branch", "other")
	m = actionRefresh(t, m)
	m, _ = actionKey(m, tea.KeyDown)
	if m.cursor != 1 || m.offset != 0 {
		t.Fatal("branch arrows did not select a branch")
	}
	m.search.SetValue("other")
	m.cursor = 0
	m, cmd := actionKey(m, tea.KeyEnter)
	if cmd != nil || m.dialog.target != "other" {
		t.Fatal("filtered selection did not open switch confirmation")
	}
	m, _ = actionKey(m, tea.KeyEscape)
	if got := actionGit(t, m.opts.Path, "branch", "--show-current"); got != "main" {
		t.Fatal("cancelled switch changed Git state")
	}
	m, _ = actionKey(m, tea.KeyEnter)
	m, cmd = actionKey(m, tea.KeyEnter)
	m = finishWrite(t, m, cmd)
	if m.status.Branch != "other" {
		t.Fatal("switch did not refresh current branch")
	}
	m, _ = press(m, 'n')
	for _, char := range "feature/new" {
		m, _ = press(m, char)
	}
	m, cmd = actionKey(m, tea.KeyEnter)
	m = finishWrite(t, m, cmd)
	if m.status.Branch != "feature/new" {
		t.Fatal("new branch was not created and checked out")
	}
}

func TestCommitValidationAndFailureKeepForm(t *testing.T) {
	m := actionModel(t, 0)
	m, _ = press(m, 'c')
	m.gitInput.SetValue("keep this message")
	m, cmd := actionKey(m, tea.KeyEnter)
	if cmd != nil || m.dialog.err == "" || m.gitBusy {
		t.Fatal("empty index was submitted")
	}
	m.gitBusy, m.writeID = true, 1
	next, _ := m.Update(gitWriteDone{id: 0, notice: "stale"})
	if !next.(Model).gitBusy {
		t.Fatal("stale mutation result accepted")
	}
	next, _ = m.Update(gitWriteDone{id: 1, err: os.ErrPermission})
	m = next.(Model)
	if m.gitBusy || m.dialog.kind != "commit" || m.gitInput.Value() != "keep this message" || m.dialog.err == "" {
		t.Fatal("failed mutation discarded commit input")
	}
}

func TestGitDialogsFitAndRetainControls(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 20}, {40, 15}} {
		for _, kind := range []string{"menu", "commit", "create", "switch"} {
			m := NewModel(context.Background(), Options{Mode: "git", Width: size[0], Height: size[1], NoColor: true})
			m.loading = false
			m.status.Branch = strings.Repeat("long-branch-", 20)
			next, _ := m.openGitDialog(kind, "target")
			m = next.(Model)
			m.dialog.err = ""
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || strings.Contains(view, "\x1b") || !strings.Contains(view, "esc cancel") {
				t.Fatalf("%s %v clipped controls: %q", kind, size, view)
			}
		}
	}
}
