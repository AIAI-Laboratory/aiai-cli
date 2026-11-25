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
	if len(p.Operations) != 9 || len(goldens) != len(p.Operations) {
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
	if len(r.Completed) != 9 {
		t.Fatalf("completed: %v", r.Completed)
	}
	for _, op := range p.Operations {
		data, err := os.ReadFile(filepath.Join(target, op.Path))
		if err != nil || string(data) != string(goldens[op.Path]) {
			t.Fatalf("generated %s: %v", op.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, ".pre-commit-config.yaml")); err != nil {
		t.Fatalf("missing .pre-commit-config.yaml: %v", err)
	}
	for _, removed := range []string{".husky/pre-commit", "package.json", "pnpm-workspace.yaml"} {
		if _, err := os.Stat(filepath.Join(target, removed)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("obsolete file still generated: %s", removed)
		}
	}
	joinedSteps := strings.Join(r.NextSteps, "\n")
	if !strings.Contains(joinedSteps, "uv run pre-commit install") || strings.Contains(joinedSteps, "pnpm") {
		t.Fatalf("unexpected next steps: %v", r.NextSteps)
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
	if err != nil || len(r.Completed) != 0 || len(r.Skipped) != 9 {
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

func TestAgentKitDefaultSubsetAndValidation(t *testing.T) {
	// Agent kit should work on arbitrary existing folder names without Python naming constraints.
	target := filepath.Join(t.TempDir(), "My Existing Workspace (2026)")
	p, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID: templates.AgentKitID,
		TargetDir:  target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Operations) != 8 {
		t.Fatalf("expected 8 default agent-kit files, got %d", len(p.Operations))
	}
	r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Completed) != 8 {
		t.Fatalf("expected 8 completed files, got %v", r.Completed)
	}
	for _, rel := range []string{
		"AGENTS.md",
		".gitignore",
		".agents/rules/docstring.md",
		".agents/rules/terminal-logging.md",
		".agents/skills/commit-and-pr/SKILL.md",
		".agents/skills/dev-flow/SKILL.md",
		"docs/CONTRIBUTING.md",
		".github/pull_request_template.md",
	} {
		if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
			t.Errorf("missing generated file %s: %v", rel, err)
		}
	}

	subsetDir := filepath.Join(t.TempDir(), "subset")
	p, err = initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           subsetDir,
		Components:          []string{"agents-md", "rule:docstring"},
		ComponentsSpecified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Operations) != 2 {
		t.Fatalf("expected 2 subset operations, got %+v", p.Operations)
	}

	if _, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           subsetDir,
		Components:          nil,
		ComponentsSpecified: true,
	}); !errors.Is(err, scaffold.ErrValidation) {
		t.Fatalf("expected validation error for empty component selection, got %v", err)
	}
}

func TestAgentKitAgentsMDAppendReplaceSkipAndGitignore(t *testing.T) {
	dir := t.TempDir()
	originalAgents := "# Existing Repo Guide\n\nKeep my existing notes.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(originalAgents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Default mode appends to AGENTS.md and .gitignore without overwriting user content.
	p, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           dir,
		Components:          []string{"agents-md", "rule:terminal-logging"},
		ComponentsSpecified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]scaffold.Action{}
	for _, op := range p.Operations {
		actions[op.Path] = op.Action
	}
	if actions["AGENTS.md"] != scaffold.Append || actions[".gitignore"] != scaffold.Append {
		t.Fatalf("expected append actions, got %+v", actions)
	}
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	agentsData, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(agentsData), originalAgents) || !strings.Contains(string(agentsData), "<!-- aiai:agent-kit:start -->") {
		t.Fatalf("AGENTS.md did not preserve user content and append block:\n%s", agentsData)
	}
	gitignoreData, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignoreData), "dist/\n.log/\n") {
		t.Fatalf(".gitignore did not append .log/:\n%s", gitignoreData)
	}

	// Second append run must be idempotent (Skip).
	p2, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           dir,
		Components:          []string{"agents-md", "rule:terminal-logging"},
		ComponentsSpecified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range p2.Operations {
		if (op.Path == "AGENTS.md" || op.Path == ".gitignore") && op.Action != scaffold.Skip {
			t.Fatalf("expected idempotent skip for %s, got %s", op.Path, op.Action)
		}
	}

	// Skip mode leaves existing AGENTS.md untouched.
	skipDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skipDir, "AGENTS.md"), []byte(originalAgents), 0o644); err != nil {
		t.Fatal(err)
	}
	pSkip, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           skipDir,
		Components:          []string{"agents-md"},
		ComponentsSpecified: true,
		AgentsMDMode:        scaffold.ExistingSkip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pSkip.Operations) != 1 || pSkip.Operations[0].Action != scaffold.Skip {
		t.Fatalf("expected skip operation, got %+v", pSkip.Operations)
	}

	// Replace mode overwrites AGENTS.md with the template content.
	pReplace, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:          templates.AgentKitID,
		TargetDir:           skipDir,
		Components:          []string{"agents-md"},
		ComponentsSpecified: true,
		AgentsMDMode:        scaffold.ExistingReplace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pReplace.Operations) != 1 || pReplace.Operations[0].Action != scaffold.Overwrite {
		t.Fatalf("expected overwrite operation, got %+v", pReplace.Operations)
	}
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), pReplace); err != nil {
		t.Fatal(err)
	}
	replaced, _ := os.ReadFile(filepath.Join(skipDir, "AGENTS.md"))
	if strings.Contains(string(replaced), "Keep my existing notes.") || !strings.Contains(string(replaced), "# Agent Guidelines") {
		t.Fatalf("AGENTS.md was not replaced:\n%s", replaced)
	}
}

func TestAgentKitProtectsExistingSkillsAndRulesEvenWithForce(t *testing.T) {
	dir := t.TempDir()
	customRule := "# My custom docstring rule\n"
	customSkill := "# My custom dev-flow skill\n"
	if err := os.MkdirAll(filepath.Join(dir, ".agents/rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agents/rules/docstring.md"), []byte(customRule), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agents/skills/dev-flow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agents/skills/dev-flow/custom.md"), []byte(customSkill), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID: templates.AgentKitID,
		TargetDir:  dir,
		Force:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.HasConflicts() {
		t.Fatalf("protected skills/rules must not block apply: %+v", p.Operations)
	}
	actions := map[string]scaffold.Action{}
	for _, op := range p.Operations {
		actions[op.Path] = op.Action
	}
	if actions[".agents/rules/docstring.md"] != scaffold.Protected || actions[".agents/skills/dev-flow/SKILL.md"] != scaffold.Protected {
		t.Fatalf("expected protected actions for colliding skill and rule, got %+v", actions)
	}
	if actions[".agents/skills/commit-and-pr/SKILL.md"] != scaffold.Create || actions[".agents/rules/terminal-logging.md"] != scaffold.Create {
		t.Fatalf("expected non-colliding skill/rule to create, got %+v", actions)
	}
	if len(p.Warnings) < 2 {
		t.Fatalf("expected protection warnings, got %v", p.Warnings)
	}

	r, err := (scaffold.FileExecutor{}).Apply(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Protected) != 2 {
		t.Fatalf("expected 2 protected paths in result, got %v", r.Protected)
	}
	gotRule, _ := os.ReadFile(filepath.Join(dir, ".agents/rules/docstring.md"))
	if string(gotRule) != customRule {
		t.Fatal("existing user rule was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/dev-flow/SKILL.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("existing user skill folder was modified")
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/commit-and-pr/SKILL.md")); err != nil {
		t.Fatal("non-colliding skill was not created")
	}
}

func TestPythonMinimalComposedWithAgentKit(t *testing.T) {
	target := filepath.Join(t.TempDir(), "composed")
	p, err := initializer(t).Plan(context.Background(), project.InitRequest{
		TemplateID:   templates.PythonMinimalID,
		ProjectName:  "example-project",
		TargetDir:    target,
		IncludeAgent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 9 python-minimal files + 7 non-overlapping agent-kit files (.gitignore is merged).
	if len(p.Operations) != 16 {
		t.Fatalf("expected 16 composed operations, got %d", len(p.Operations))
	}
	if _, err := (scaffold.FileExecutor{}).Apply(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	gitignore, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(gitignore), ".log/") != 1 {
		t.Fatalf("expected single .log/ entry in .gitignore:\n%s", gitignore)
	}
	for _, rel := range []string{".pre-commit-config.yaml", "AGENTS.md", ".agents/rules/docstring.md", ".agents/skills/dev-flow/SKILL.md", "docs/CONTRIBUTING.md", ".github/pull_request_template.md"} {
		if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
			t.Errorf("missing composed file %s: %v", rel, err)
		}
	}
}
