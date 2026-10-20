package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is not installed")
	}
	dir := t.TempDir()
	gitCmd(t, dir, "init", "--initial-branch=main")
	gitCmd(t, dir, "config", "user.name", "Local Tester")
	gitCmd(t, dir, "config", "user.email", "local@example.invalid")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-config"), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	gitCmd(t, dir, "add", "--all")
	gitCmd(t, dir, "commit", "-m", msg)
}

func TestStatusHistoryDiffAndTracking(t *testing.T) {
	ctx := context.Background()
	dir := fixture(t)
	s := &Service{}
	st, err := s.Status(ctx, dir)
	if err != nil || !st.Unborn || st.Branch != "main" || len(st.Changes) != 0 {
		t.Fatalf("unborn %+v %v", st, err)
	}
	write(t, filepath.Join(dir, "hello world.txt"), "one\n")
	commit(t, dir, "first")
	first, err := s.Status(ctx, dir)
	if err != nil || first.Head == "" || first.Unborn {
		t.Fatalf("first %+v %v", first, err)
	}
	write(t, filepath.Join(dir, "hello world.txt"), "two\n")
	commit(t, dir, "second")
	gitCmd(t, dir, "remote", "add", "origin", "https://token:do-not-print@example.invalid/repo")
	gitCmd(t, dir, "update-ref", "refs/remotes/origin/main", first.Head)
	gitCmd(t, dir, "config", "branch.main.remote", "origin")
	gitCmd(t, dir, "config", "branch.main.merge", "refs/heads/main")
	write(t, filepath.Join(dir, "hello world.txt"), "three\n")
	gitCmd(t, dir, "add", "--", "hello world.txt")
	write(t, filepath.Join(dir, "hello world.txt"), "four\n")
	name := "-odd 日本語.txt"
	if runtime.GOOS != "windows" {
		name += "\nline"
	}
	write(t, filepath.Join(dir, name), "untracked")
	st, err = s.Status(ctx, dir)
	if err != nil || st.Ahead != 1 || st.Behind != 0 || st.Staged != 1 || st.Unstaged != 1 || st.Untracked != 1 {
		t.Fatalf("status %+v %v", st, err)
	}
	if st.Changes[len(st.Changes)-1].Path != name {
		t.Fatalf("filename lost: %+v", st.Changes)
	}
	diff, err := s.Diff(ctx, dir, "hello world.txt", false)
	if err != nil || !strings.Contains(diff, "+four") {
		t.Fatalf("diff %q %v", diff, err)
	}
	diff, err = s.Diff(ctx, dir, "hello world.txt", true)
	if err != nil || !strings.Contains(diff, "+three") {
		t.Fatalf("cached diff %q %v", diff, err)
	}
	history, err := s.History(ctx, dir, Filter{Limit: 2})
	if err != nil || len(history) != 2 || history[0].Subject != "second" || history[1].Hash != first.Head {
		t.Fatalf("history %+v %v", history, err)
	}
	branches, err := s.Branches(ctx, dir)
	if err != nil || len(branches) != 1 || !branches[0].Current || branches[0].Upstream != "origin/main" {
		t.Fatalf("branches %+v %v", branches, err)
	}
	activity, err := s.Activity(ctx, dir, Filter{Limit: 1})
	if err != nil || len(activity.Commits) != 1 || !activity.Truncated {
		t.Fatalf("activity %+v %v", activity, err)
	}
	_, _, summary := s.Observe(ctx, dir, first.Head, "main")
	if !strings.Contains(summary, "1 new reachable") {
		t.Fatal(summary)
	}
	_, _, summary = s.Observe(ctx, dir, first.Head, "other")
	if !strings.Contains(summary, "uncertain") {
		t.Fatal(summary)
	}
	for _, bad := range []string{"../escape", "x/../../escape"} {
		if _, err := s.Diff(ctx, dir, bad, false); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if _, err := s.History(ctx, dir, Filter{Branch: "--all"}); err == nil {
		t.Fatal("accepted option injection")
	}
}

func TestRenameConflictBinaryAndWorktrees(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	write(t, filepath.Join(dir, "old name"), "base\n")
	write(t, filepath.Join(dir, "binary"), "\x00one")
	commit(t, dir, "base")
	gitCmd(t, dir, "mv", "old name", "new name")
	st, err := s.Status(ctx, dir)
	if err != nil || len(st.Changes) != 1 || st.Changes[0].Original != "old name" || st.Changes[0].Path != "new name" {
		t.Fatalf("rename %+v %v", st, err)
	}
	commit(t, dir, "rename")
	other := filepath.Join(t.TempDir(), "linked space")
	gitCmd(t, dir, "worktree", "add", "--detach", other, "HEAD")
	linked, err := s.Status(ctx, other)
	if err != nil || !linked.Detached || linked.RepositoryID != st.RepositoryID || linked.Root == st.Root {
		t.Fatalf("linked %+v %v", linked, err)
	}
	gitCmd(t, dir, "worktree", "lock", "--reason", "test lock", other)
	trees, err := s.Worktrees(ctx, dir)
	if err != nil || len(trees) != 2 || trees[1].Locked != "test lock" {
		t.Fatalf("trees %+v %v", trees, err)
	}
	write(t, filepath.Join(dir, "binary"), "\x00two")
	diff, err := s.Diff(ctx, dir, "binary", false)
	if err != nil || !strings.Contains(diff, "Binary files") {
		t.Fatalf("binary %q %v", diff, err)
	}
	gitCmd(t, dir, "restore", "binary")
	gitCmd(t, dir, "switch", "-c", "other")
	write(t, filepath.Join(dir, "new name"), "other\n")
	commit(t, dir, "other")
	gitCmd(t, dir, "switch", "main")
	write(t, filepath.Join(dir, "new name"), "main\n")
	commit(t, dir, "main")
	cmd := exec.Command("git", "merge", "other")
	cmd.Dir = dir
	_ = cmd.Run()
	st, err = s.Status(ctx, dir)
	if err != nil || st.Conflicts != 1 || !st.Changes[0].Conflict {
		t.Fatalf("conflict %+v %v", st, err)
	}
}

func TestGitSafetyCancellationAndUnavailable(t *testing.T) {
	dir := fixture(t)
	write(t, filepath.Join(dir, "a"), "a\n")
	commit(t, dir, "base")
	write(t, filepath.Join(dir, "a"), "b\n")
	marker := filepath.Join(dir, "should-not-exist")
	gitCmd(t, dir, "config", "diff.external", "touch "+marker)
	s := &Service{}
	if _, err := s.Diff(context.Background(), dir, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("external diff ran")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Status(ctx, dir); err == nil {
		t.Fatal("cancellation ignored")
	}
	if _, err := (&Service{Executable: "nonexistent-aiai-git"}).Status(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "install Git") {
		t.Fatal(err)
	}
	if _, err := s.Status(context.Background(), t.TempDir()); err == nil {
		t.Fatal("accepted non-repository")
	}
	commits, err := s.History(context.Background(), dir, Filter{Since: time.Now().Add(24 * time.Hour)})
	if err != nil || len(commits) != 0 {
		t.Fatalf("future query %+v %v", commits, err)
	}
}

func TestBehindAndRewrittenSnapshot(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "base")
	head := gitCmd(t, dir, "rev-parse", "HEAD")
	tree := gitCmd(t, dir, "rev-parse", "HEAD^{tree}")
	remote := gitCmd(t, dir, "commit-tree", tree, "-p", head, "-m", "local tracking snapshot")
	gitCmd(t, dir, "remote", "add", "origin", "https://example.invalid/never-contacted")
	gitCmd(t, dir, "update-ref", "refs/remotes/origin/main", remote)
	gitCmd(t, dir, "config", "branch.main.remote", "origin")
	gitCmd(t, dir, "config", "branch.main.merge", "refs/heads/main")
	service := &Service{}
	st, err := service.Status(ctx, dir)
	if err != nil || st.Behind != 1 || st.Ahead != 0 {
		t.Fatalf("behind %+v %v", st, err)
	}
	gitCmd(t, dir, "commit", "--allow-empty", "--amend", "-m", "rewritten base")
	end, branch, summary := service.Observe(ctx, dir, head, "main")
	if end == head || branch != "main" || !strings.Contains(summary, "uncertain") {
		t.Fatalf("rewritten comparison %s %s %s", end, branch, summary)
	}
}
