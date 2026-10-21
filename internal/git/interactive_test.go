package git

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func nativeGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd, err := (&Service{}).InteractiveCommand(context.Background(), dir, args)
	if err != nil {
		return "", err
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-config"), "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestParseCommandPreservesArgumentsWithoutShellExpansion(t *testing.T) {
	for _, args := range [][]string{
		{"commit", "-m", "feat: quoted ' and \" message"},
		{"config", "test.value", "$(touch /tmp/not-a-shell) ; && | * $HOME"},
		{"worktree", "add", "../folder with spaces", "日本語"},
		{"config", "test.value", ""},
		{"push", "--force-with-lease", "origin", "HEAD:feature/new"},
	} {
		parsed, err := ParseCommand("git " + FormatArguments(args))
		if err != nil || !reflect.DeepEqual(parsed, args) {
			t.Fatalf("round trip: %q => %q (%v)", args, parsed, err)
		}
	}
	parsed, err := ParseCommand(`commit -m "feat: quick git" -- 'file with spaces'`)
	if err != nil || !reflect.DeepEqual(parsed, []string{"commit", "-m", "feat: quick git", "--", "file with spaces"}) {
		t.Fatalf("quotes lost: %q %v", parsed, err)
	}
	for _, bad := range []string{"", "git", "commit 'unfinished", "log \\", "status\nreset --hard", "status\x00"} {
		if _, err := ParseCommand(bad); err == nil {
			t.Fatalf("accepted malformed command %q", bad)
		}
	}
	dir := fixture(t)
	literal := "$(touch never-execute) ; echo not-a-shell"
	if out, err := nativeGit(t, dir, "config", "test.literal", literal); err != nil {
		t.Fatalf("config failed: %v %s", err, out)
	}
	if got := gitCmd(t, dir, "config", "test.literal"); got != literal {
		t.Fatalf("literal input changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "never-execute")); !os.IsNotExist(err) {
		t.Fatal("command substitution executed")
	}
}

func TestNativeFetchPushPublishAndPullWithBareRemote(t *testing.T) {
	dir := fixture(t)
	remote := t.TempDir()
	gitCmd(t, remote, "init", "--bare", "--initial-branch=main")
	write(t, filepath.Join(dir, "file"), "base\n")
	commit(t, dir, "base")
	gitCmd(t, dir, "remote", "add", "origin", remote)
	if out, err := nativeGit(t, dir, "push", "--set-upstream", "origin", "HEAD"); err != nil {
		t.Fatalf("publish: %v %s", err, out)
	}
	if got := gitCmd(t, dir, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/main" {
		t.Fatal("publish did not set upstream:", got)
	}
	other := fixture(t)
	gitCmd(t, other, "remote", "add", "origin", remote)
	for _, args := range [][]string{{"fetch", "--all", "--prune"}, {"pull", "--ff-only", "origin", "main"}, {"branch", "--set-upstream-to=origin/main"}} {
		if out, err := nativeGit(t, other, args...); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	write(t, filepath.Join(other, "file"), "from remote\n")
	commit(t, other, "remote update")
	if out, err := nativeGit(t, other, "push"); err != nil {
		t.Fatalf("push: %v %s", err, out)
	}
	if out, err := nativeGit(t, dir, "pull", "--ff-only"); err != nil {
		t.Fatalf("pull: %v %s", err, out)
	}
	if got := gitCmd(t, dir, "log", "-1", "--format=%s"); got != "remote update" {
		t.Fatal("pull did not update local HEAD:", got)
	}
	write(t, filepath.Join(dir, "local"), "local\n")
	commit(t, dir, "local divergence")
	head := gitCmd(t, dir, "rev-parse", "HEAD")
	write(t, filepath.Join(other, "remote"), "remote\n")
	commit(t, other, "remote divergence")
	if out, err := nativeGit(t, other, "push"); err != nil {
		t.Fatalf("remote push: %v %s", err, out)
	}
	if out, err := nativeGit(t, dir, "pull", "--ff-only"); err == nil {
		t.Fatal("divergent pull unexpectedly rewrote history:", out)
	}
	if got := gitCmd(t, dir, "rev-parse", "HEAD"); got != head {
		t.Fatal("failed pull changed HEAD")
	}
	if out, err := nativeGit(t, dir, "pull", "--rebase"); err != nil {
		t.Fatalf("rebase pull: %v %s", err, out)
	}
	if out, err := nativeGit(t, dir, "tag", "--annotate", "v1.0.0", "-m", "release"); err != nil {
		t.Fatalf("tag: %v %s", err, out)
	}
	if out, err := nativeGit(t, dir, "push", "--tags"); err != nil {
		t.Fatalf("tag push: %v %s", err, out)
	}
	if got := gitCmd(t, remote, "tag", "--list"); got != "v1.0.0" {
		t.Fatal("tag was not published:", got)
	}
}

func TestNativeStashMergeConflictAndAbort(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	file := filepath.Join(dir, "file")
	write(t, file, "base\n")
	commit(t, dir, "base")
	write(t, file, "saved work\n")
	write(t, filepath.Join(dir, "untracked"), "new\n")
	if out, err := nativeGit(t, dir, "stash", "push", "--include-untracked", "-m", "work"); err != nil {
		t.Fatalf("stash: %v %s", err, out)
	}
	st, _ := s.Status(ctx, dir)
	if len(st.Changes) != 0 {
		t.Fatal("stash did not save and clean working files")
	}
	if out, err := nativeGit(t, dir, "stash", "pop", "stash@{0}"); err != nil {
		t.Fatalf("pop: %v %s", err, out)
	}
	contents, _ := os.ReadFile(file)
	if !bytes.Equal(contents, []byte("saved work\n")) {
		t.Fatal("stash lost working content")
	}
	commit(t, dir, "saved work")
	gitCmd(t, dir, "switch", "-c", "other")
	write(t, file, "other\n")
	commit(t, dir, "other")
	gitCmd(t, dir, "switch", "main")
	write(t, file, "main\n")
	commit(t, dir, "main")
	head := gitCmd(t, dir, "rev-parse", "HEAD")
	if out, err := nativeGit(t, dir, "merge", "--no-edit", "other"); err == nil || !strings.Contains(out, "CONFLICT") {
		t.Fatalf("expected merge conflict: %v %s", err, out)
	}
	st, _ = s.Status(ctx, dir)
	if st.Operation != "merge" || st.Conflicts != 1 {
		t.Fatalf("merge state missing: %+v", st)
	}
	if out, err := nativeGit(t, dir, "merge", "--abort"); err != nil {
		t.Fatalf("merge abort: %v %s", err, out)
	}
	st, _ = s.Status(ctx, dir)
	if st.Operation != "" || st.Head != head || st.Conflicts != 0 {
		t.Fatalf("abort did not restore checkout: %+v", st)
	}
}
