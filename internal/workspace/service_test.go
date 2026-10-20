package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

func service(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	repo, _ := zen.NewJSONRepository(filepath.Join(root, "zen.json"))
	s, err := New(filepath.Join(root, "workspaces.json"), &gitlocal.Service{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "no-config"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitCmd(t, root, "init", "--initial-branch=main")
	gitCmd(t, root, "config", "user.name", "Example")
	gitCmd(t, root, "config", "user.email", "example@local.invalid")
	gitCmd(t, root, "config", "commit.gpgsign", "false")
	gitCmd(t, root, "commit", "--allow-empty", "-m", "Initial")
	return root
}

func TestRegistrySafetyAndMissingProject(t *testing.T) {
	s := service(t)
	ctx := context.Background()
	dir := t.TempDir()
	p, err := s.Add(ctx, dir, "local", "description", []string{"Go"}, false)
	if err != nil || p.RepositoryID != "" {
		t.Fatalf("nonGit %+v %v", p, err)
	}
	if _, err = s.Add(ctx, filepath.Join(dir, "."), "other", "", nil, false); err == nil {
		t.Fatal("duplicate path accepted")
	}
	if _, err = s.Add(ctx, t.TempDir(), "LOCAL", "", nil, false); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err = s.Add(ctx, filepath.Join(dir, "missing"), "missing", "", nil, false); err == nil {
		t.Fatal("invalid directory accepted")
	}
	if _, err = s.Add(ctx, dir, "git", "", nil, true); err == nil {
		t.Fatal("nonGit repository accepted")
	}
	if err = s.Open(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Find(ctx, "LOCAL", false)
	if err != nil || loaded.LastOpenedAt.IsZero() {
		t.Fatalf("open %+v %v", loaded, err)
	}
	moved := dir + "-moved"
	if err = os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(moved, dir) })
	o := s.Overview(ctx, p)
	if o.Available || o.Warning == "" {
		t.Fatal("missing project not reported")
	}
	if err = s.Remove(ctx, p.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(moved); err != nil {
		t.Fatal("unregister deleted directory")
	}
}

func TestTimelineAssociationsAndRewrittenHistory(t *testing.T) {
	s := service(t)
	ctx := context.Background()
	root := gitRepo(t)
	p, err := s.Add(ctx, root, "app", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-2 * time.Minute)
	end := start.Add(time.Minute)
	session := zen.Session{ID: "focus-one", Type: zen.SessionFocus, ProjectID: p.ID, RepositoryID: p.RepositoryID, WorktreePath: root, BranchAtStart: "main", BranchAtEnd: "feature", StartedAt: start, FinishedAt: end, PlannedSeconds: 60, ActiveSeconds: 40, Status: zen.StatusCompleted}
	if err = s.Zen.AddSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err = s.Zen.AddSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	timeline, err := s.Timeline(ctx, p, gitlocal.Filter{})
	if err != nil || len(timeline.Events) != 3 || timeline.Commits != 1 {
		t.Fatalf("timeline %+v %v", timeline, err)
	}
	ids := map[string]bool{}
	for i, e := range timeline.Events {
		if ids[e.ID] {
			t.Fatal("duplicate event")
		}
		ids[e.ID] = true
		if i > 0 && e.Time.Before(timeline.Events[i-1].Time) {
			t.Fatal("not chronological")
		}
	}
	filtered, err := s.Timeline(ctx, p, gitlocal.Filter{Branch: "main", Since: start.Add(10 * time.Second), Until: end.Add(time.Second)})
	if err != nil || len(filtered.Events) != 1 || filtered.Events[0].ID != "zen:focus-one:end" {
		t.Fatalf("date filtering %+v %v", filtered, err)
	}
	old := gitCmd(t, root, "rev-parse", "HEAD")
	gitCmd(t, root, "commit", "--allow-empty", "--amend", "-m", "Rewritten")
	timeline, err = s.Timeline(ctx, p, gitlocal.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range timeline.Events {
		if e.Source == "git" && strings.HasSuffix(e.ID, old) {
			t.Fatal("old Git event was cached")
		}
	}
	if err = os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(root+"-moved", root) })
	timeline, err = s.Timeline(ctx, p, gitlocal.Filter{})
	if err != nil || len(timeline.Events) != 2 || timeline.Warning == "" {
		t.Fatalf("stored history unavailable %+v %v", timeline, err)
	}
}

func TestWorktreesAreDistinctContexts(t *testing.T) {
	s := service(t)
	ctx := context.Background()
	root := gitRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gitCmd(t, root, "worktree", "add", "-b", "feature", linked)
	a, err := s.Add(ctx, root, "main-worktree", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Add(ctx, linked, "feature-worktree", "", nil, false)
	if err != nil || a.RepositoryID != b.RepositoryID || a.Root == b.Root || a.ID == b.ID {
		t.Fatalf("worktrees %+v %+v %v", a, b, err)
	}
	items, err := s.Board(ctx, false)
	if err != nil || len(items) != 2 || items[0].Git.Branch == items[1].Git.Branch {
		t.Fatalf("board %+v %v", items, err)
	}
}

func TestRegistryConcurrentWritesAndCorruption(t *testing.T) {
	s := service(t)
	ctx := context.Background()
	dirs := []string{t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()}
	var group sync.WaitGroup
	for i := range dirs {
		group.Go(func() {
			if _, err := s.Add(ctx, dirs[i], "project-"+string(rune('a'+i)), "", nil, false); err != nil {
				t.Errorf("add: %v", err)
			}
		})
	}
	group.Wait()
	items, err := s.List(ctx, false)
	if err != nil || len(items) != len(dirs) {
		t.Fatalf("lost registration %+v %v", items, err)
	}
	bad := []byte(`{"schema_version":999}`)
	if err = os.WriteFile(s.Store.Path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = s.Remove(ctx, "project-a", false); err == nil {
		t.Fatal("unknown schema accepted")
	}
	payload, _ := os.ReadFile(s.Store.Path)
	if string(payload) != string(bad) {
		t.Fatal("invalid data overwritten")
	}
}
