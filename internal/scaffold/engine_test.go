package scaffold_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

func initializer(t *testing.T) project.Initializer {
	t.Helper()
	r, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return project.Initializer{Engine: scaffold.Engine{Registry: r}}
}

func plan(t *testing.T, dir string, force bool) scaffold.Plan {
	t.Helper()
	p, err := initializer(t).Plan(context.Background(), project.InitRequest{TargetDir: dir, ProjectName: "example-project", Force: force})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGoldenTemplate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project")
	p := plan(t, target, false)
	if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("planning created the destination")
	}
	fixture := "../../testdata/golden/python-minimal"
	goldens := map[string][]byte{}
	err := filepath.WalkDir(fixture, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		goldens[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Operations) != 11 || len(goldens) != len(p.Operations) {
		t.Fatalf("file count: plan=%d fixtures=%d", len(p.Operations), len(goldens))
	}
	for _, op := range p.Operations {
		want, ok := goldens[op.Path]
		if !ok || string(op.Content) != string(want) {
			t.Errorf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", op.Path, op.Content, want)
		}
	}
	r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Completed) != 11 {
		t.Fatalf("completed: %v", r.Completed)
	}
	for _, op := range p.Operations {
		data, err := os.ReadFile(filepath.Join(target, op.Path))
		if err != nil || string(data) != string(goldens[op.Path]) {
			t.Fatalf("generated %s: %v", op.Path, err)
		}
	}
	info, err := os.Stat(filepath.Join(target, ".husky/pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Errorf("hook mode: %o", info.Mode().Perm())
	}
}

func TestConflictsForceAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := plan(t, dir, false)
	if !p.HasConflicts() {
		t.Fatal("missing conflict")
	}
	r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
	if !errors.Is(err, scaffold.ErrConflict) || len(r.Completed) != 0 {
		t.Fatalf("%v: %v", r, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("conflicting plan partially applied")
	}
	p = plan(t, dir, true)
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if string(notes) != "keep me" {
		t.Fatal("unrelated file modified")
	}
	r, err = (scaffold.FileExecutor{}).Apply(context.Background(), plan(t, dir, false))
	if err != nil || len(r.Completed) != 0 || len(r.Skipped) != 11 {
		t.Fatalf("rerun: %+v, %v", r, err)
	}
}

func TestStalePlanRefusedBeforeWrites(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "new file", true: "changed file"}[force], func(t *testing.T) {
			dir := t.TempDir()
			if force {
				if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := plan(t, dir, force)
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed after preview"), 0o644); err != nil {
				t.Fatal(err)
			}
			r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
			if !errors.Is(err, scaffold.ErrConflict) || len(r.Completed) != 0 {
				t.Fatalf("%v: %v", r, err)
			}
		})
	}
}

func TestSymlinksAndDirectoriesRefused(t *testing.T) {
	for _, relative := range []string{".github", "README.md"} {
		t.Run(relative, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(dir, relative)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			_, err := initializer(t).Plan(context.Background(), project.InitRequest{TargetDir: dir, ProjectName: "demo", Force: true})
			if !errors.Is(err, scaffold.ErrConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatal("escaped root")
			}
		})
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := initializer(t).Plan(context.Background(), project.InitRequest{TargetDir: dir, ProjectName: "demo", Force: true}); !errors.Is(err, scaffold.ErrConflict) {
		t.Fatal(err)
	}
}

func TestSymlinkInsertedAfterPlan(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	p := plan(t, dir, true)
	if err := os.Symlink(outside, filepath.Join(dir, ".github")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
	if !errors.Is(err, scaffold.ErrConflict) || len(r.Completed) != 0 {
		t.Fatalf("%v: %v", r, err)
	}
}

func TestCancelledPlanDoesNotWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	p := plan(t, dir, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (scaffold.FileExecutor{}).Apply(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("cancelled apply created target")
	}
}

func TestTraversalInPlanRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	p := plan(t, dir, false)
	p.Operations[0].Path = "../escape"
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), p); !errors.Is(err, scaffold.ErrValidation) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("invalid plan created target")
	}
}

func TestNoTemporaryFilesRemain(t *testing.T) {
	dir := t.TempDir()
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), plan(t, dir, false)); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".aiai-") {
			t.Errorf("temporary file remained: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Cancellation at the second file boundary exercises real executor bookkeeping.
type cancelAtBoundary struct {
	context.Context
	calls int
}

func (c *cancelAtBoundary) Err() error {
	c.calls++
	if c.calls >= 3 {
		return context.Canceled
	}
	return nil
}

func TestPartialCancellationReportsCompletedFiles(t *testing.T) {
	dir := t.TempDir()
	p := plan(t, dir, false)
	ctx := &cancelAtBoundary{Context: context.Background()}
	r, err := (scaffold.FileExecutor{}).Apply(ctx, p)
	if !errors.Is(err, context.Canceled) || len(r.Completed) != 1 {
		t.Fatalf("%+v: %v", r, err)
	}
	if r.Completed[0] != p.Operations[0].Path {
		t.Fatal("wrong completed file")
	}
	if _, err := os.Stat(filepath.Join(dir, r.Completed[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, p.Operations[1].Path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("second file was written")
	}
}

func TestOverwritePreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), plan(t, dir, true)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions changed to %o", info.Mode().Perm())
	}
}
