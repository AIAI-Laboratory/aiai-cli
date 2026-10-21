package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIndexActionsAndCommitPreserveUnstagedContents(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	name := "-odd [日本語].txt"
	file := filepath.Join(dir, name)
	write(t, file, "staged version\n")
	write(t, filepath.Join(dir, "other.txt"), "untracked\n")
	change := Change{Path: name, Untracked: true}
	if err := s.Stage(ctx, dir, change); err != nil {
		t.Fatal(err)
	}
	write(t, file, "keep working version\n")
	if err := s.Unstage(ctx, dir, change); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, dir)
	contents, _ := os.ReadFile(file)
	if st.Staged != 0 || st.Untracked != 2 || string(contents) != "keep working version\n" {
		t.Fatalf("unstaging an unborn repository lost files: %+v %q", st, contents)
	}
	write(t, file, "staged version\n")
	if err := s.Stage(ctx, dir, change); err != nil {
		t.Fatal(err)
	}
	write(t, file, "keep working version\n")
	message := "feat: preserve literal $() and --options"
	if err := s.CommitStaged(ctx, dir, message); err != nil {
		t.Fatal(err)
	}
	if got := gitCmd(t, dir, "show", "HEAD:"+name); got != "staged version" {
		t.Fatal(got)
	}
	st, _ = s.Status(ctx, dir)
	if st.Staged != 0 || st.Unstaged != 1 || st.Untracked != 1 || st.LastCommit.Subject != message {
		t.Fatalf("commit included unstaged or untracked content: %+v", st)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.StageAll(ctx, nested); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Status(ctx, dir)
	if st.Staged != 2 {
		t.Fatalf("stage all did not use repository root: %+v", st)
	}
	if err := s.UnstageAll(ctx, nested); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Status(ctx, dir)
	if st.Staged != 0 || st.Untracked != 1 || st.Unstaged != 1 {
		t.Fatalf("unstage all changed working files: %+v", st)
	}
	if err := s.CommitStaged(ctx, dir, "nothing staged"); err == nil {
		t.Fatal("committed an empty index")
	}
	for _, path := range []string{"", ".", "..", "../escape", "x/../../escape", file, "nul\x00file"} {
		if err := s.Stage(ctx, dir, Change{Path: path}); err == nil {
			t.Fatalf("accepted path %q", path)
		}
	}
}

func TestRenameStagingUpdatesBothPaths(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	write(t, filepath.Join(dir, "old.txt"), "content\n")
	commit(t, dir, "base")
	gitCmd(t, dir, "mv", "old.txt", "new.txt")
	change := Change{Path: "new.txt", Original: "old.txt"}
	if err := s.Unstage(ctx, dir, change); err != nil {
		t.Fatal(err)
	}
	if err := s.Stage(ctx, dir, change); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Status(ctx, dir)
	if len(st.Changes) != 1 || st.Changes[0].Original != "old.txt" || st.Changes[0].Index != "R" {
		t.Fatalf("rename was partially staged: %+v", st)
	}
}

func TestCommitRunsHooksAndRetainsIndexOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixture requires POSIX")
	}
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	write(t, filepath.Join(dir, "file"), "content\n")
	if err := s.StageAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(dir, ".git", "custom-hooks")
	if err := os.Mkdir(hooks, 0o700); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "config", "core.hooksPath", hooks)
	hook := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf ran > hook-ran\necho 'https://token:secret@example.invalid' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := s.CommitStaged(ctx, dir, "blocked")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("hook failure missing or leaked diagnostics: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hook-ran")); err != nil {
		t.Fatal("repository hook was bypassed")
	}
	st, _ := s.Status(ctx, dir)
	if !st.Unborn || st.Staged != 1 {
		t.Fatalf("failed commit changed HEAD or index: %+v", st)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitStaged(ctx, dir, "allowed"); err != nil {
		t.Fatal(err)
	}
}

func TestBranchActionsRespectLocalChangesAndWorktrees(t *testing.T) {
	dir := fixture(t)
	ctx := context.Background()
	s := &Service{}
	file := filepath.Join(dir, "file")
	write(t, file, "base\n")
	commit(t, dir, "base")
	if err := s.CreateBranch(ctx, dir, "feature/other"); err != nil {
		t.Fatal(err)
	}
	write(t, file, "other\n")
	commit(t, dir, "other")
	if err := s.SwitchBranch(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	write(t, file, "local work\n")
	if err := s.SwitchBranch(ctx, dir, "feature/other"); err == nil {
		t.Fatal("overwrote conflicting local changes")
	}
	contents, _ := os.ReadFile(file)
	if got := gitCmd(t, dir, "branch", "--show-current"); got != "main" || string(contents) != "local work\n" {
		t.Fatalf("failed switch changed checkout: %s %q", got, contents)
	}
	if err := s.CreateBranch(ctx, dir, "feature/local"); err != nil {
		t.Fatal(err)
	}
	contents, _ = os.ReadFile(file)
	if string(contents) != "local work\n" {
		t.Fatal("creating a branch lost dirty files")
	}
	for _, name := range []string{"-", "--discard-changes", "HEAD~1", "bad name", "missing"} {
		if err := s.SwitchBranch(ctx, dir, name); err == nil {
			t.Fatalf("accepted invalid/missing branch %q", name)
		}
	}
	if err := s.CreateBranch(ctx, dir, "main"); err == nil {
		t.Fatal("replaced an existing branch")
	}
	linked := filepath.Join(t.TempDir(), "linked")
	gitCmd(t, dir, "worktree", "add", linked, "feature/other")
	if err := s.SwitchBranch(ctx, dir, "feature/other"); err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("ignored worktree ownership: %v", err)
	}
}
