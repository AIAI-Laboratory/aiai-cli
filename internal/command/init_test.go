package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

func TestCLIContracts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		extra       []string
		input       string
		interactive bool
		preexisting bool
		code        int
		writes      bool
	}{
		{"dry run", []string{"--dry-run"}, "", false, false, 0, false},
		{"missing yes", nil, "", false, false, 2, false},
		{"yes", []string{"--yes"}, "", false, false, 0, true},
		{"interactive clean applies directly", nil, "", true, false, 0, true},
		{"interactive yes applies overwrite without prompt", []string{"--force", "--yes"}, "", true, true, 0, true},
		{"interactive overwrite confirmed", []string{"--force"}, "yes\n", true, true, 0, true},
		{"interactive overwrite declined", []string{"--force"}, "n\n", true, true, 130, false},
		{"interactive overwrite eof cancels", []string{"--force"}, "", true, true, 130, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "generated")
			if tc.preexisting {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"init", "python", "demo", "--target", dir, "--json"}, tc.extra...)
			code, out, _ := run(t, args, tc.interactive, tc.input)
			if code != tc.code {
				t.Fatalf("code %d: %s", code, out)
			}
			var e output.Envelope
			if err := json.Unmarshal([]byte(out), &e); err != nil {
				t.Fatal(err, out)
			}
			if e.SchemaVersion != 1 || e.Plan == nil {
				t.Fatalf("bad envelope: %s", out)
			}
			_, err := os.Stat(filepath.Join(dir, "pyproject.toml"))
			if (err == nil) != tc.writes {
				t.Fatalf("unexpected write: %v", err)
			}
		})
	}
}

func TestInteractiveConflictResolutionCLI(t *testing.T) {
	// Default Enter skips conflicting file while creating remaining files.
	dirSkip := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dirSkip, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirSkip, "docs/CONTRIBUTING.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, []string{"init", "agent", dirSkip, "--json"}, true, "\n")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	gotDoc, _ := os.ReadFile(filepath.Join(dirSkip, "docs/CONTRIBUTING.md"))
	if string(gotDoc) != "custom" {
		t.Fatal("skipped conflict was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dirSkip, "AGENTS.md")); err != nil {
		t.Fatal("non-conflicting AGENTS.md was not written")
	}

	// 'r' replaces conflicting file.
	dirReplace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dirReplace, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirReplace, "docs/CONTRIBUTING.md"), []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run(t, []string{"init", "agent", dirReplace, "--json"}, true, "r\n")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	gotDoc2, _ := os.ReadFile(filepath.Join(dirReplace, "docs/CONTRIBUTING.md"))
	if string(gotDoc2) == "custom" {
		t.Fatal("replace choice did not overwrite conflict")
	}

	// 'c' or EOF cancels without writing.
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"cancel", "c\n"},
		{"eof", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dirCancel := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dirCancel, "docs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dirCancel, "docs/CONTRIBUTING.md"), []byte("custom"), 0o644); err != nil {
				t.Fatal(err)
			}
			code, out, _ := run(t, []string{"init", "agent", dirCancel, "--json"}, true, tc.input)
			if code != 130 {
				t.Fatalf("expected 130, got %d: %s", code, out)
			}
			if _, err := os.Stat(filepath.Join(dirCancel, "AGENTS.md")); err == nil {
				t.Fatal("cancelled conflict resolution wrote files")
			}
		})
	}
}

func TestNamedAndCurrentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	code, out, stderr := run(t, []string{"init", "python", "My_Project", "--yes"}, false, "")
	if code != 0 {
		t.Fatal(out, stderr)
	}
	if _, err := os.Stat("my-project/my_project/__init__.py"); err != nil {
		t.Fatal(err)
	}
	code, out, stderr = run(t, []string{"init", "python", ".", "--name", "here", "--yes"}, false, "")
	if code != 0 {
		t.Fatal(out, stderr)
	}
	if _, err := os.Stat("here/__init__.py"); err != nil {
		t.Fatal(err)
	}
}

func TestConflictJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, []string{"init", "python", "demo", "--target", dir, "--yes", "--json"}, false, "")
	if code != 3 {
		t.Fatalf("%d: %s", code, out)
	}
	var e output.Envelope
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatal(err)
	}
	if e.Error == nil || e.Error.Code != 3 || e.Plan == nil || !e.Plan.HasConflicts() {
		t.Fatal(out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(data) != "mine" {
		t.Fatal("clobbered file")
	}
}

func TestInitAgentCLI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-repo")
	code, out, stderr := run(t, []string{"init", "agent", dir, "--yes", "--json"}, false, "")
	if code != 0 {
		t.Fatalf("code %d: %s %s", code, out, stderr)
	}
	var e output.Envelope
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatal(err, out)
	}
	if e.Result == nil || len(e.Result.Completed) != 8 {
		t.Fatalf("expected 8 written files: %s", out)
	}

	// Re-run with a custom skill/rule collision and existing AGENTS.md in interactive mode.
	existingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(existingDir, "AGENTS.md"), []byte("# Mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(existingDir, ".agents/rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, ".agents/rules/docstring.md"), []byte("custom rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Empty line chooses default Append for AGENTS.md, then "yes" confirms applying the plan.
	code, out, stderr = run(t, []string{"init", "agent", existingDir, "--force", "--json"}, true, "\nyes\n")
	if code != 0 {
		t.Fatalf("code %d: %s %s", code, out, stderr)
	}
	var e2 output.Envelope
	if err := json.Unmarshal([]byte(out), &e2); err != nil {
		t.Fatal(err, out)
	}
	if e2.Result == nil || len(e2.Result.Protected) != 1 || e2.Result.Protected[0] != ".agents/rules/docstring.md" {
		t.Fatalf("expected protected rule in JSON result: %s", out)
	}
	agentsContent, _ := os.ReadFile(filepath.Join(existingDir, "AGENTS.md"))
	if !strings.HasPrefix(string(agentsContent), "# Mine\n") || !strings.Contains(string(agentsContent), "# Agent Guidelines") {
		t.Fatalf("expected AGENTS.md append, got:\n%s", agentsContent)
	}
	ruleContent, _ := os.ReadFile(filepath.Join(existingDir, ".agents/rules/docstring.md"))
	if string(ruleContent) != "custom rule" {
		t.Fatal("user rule was overwritten")
	}

	// Subset via --components flag.
	subsetDir := t.TempDir()
	code, out, stderr = run(t, []string{"init", "agent", subsetDir, "--components", "agents-md,skill:dev-flow", "--yes"}, false, "")
	if code != 0 {
		t.Fatalf("code %d: %s %s", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Join(subsetDir, ".agents/skills/dev-flow/SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(subsetDir, "docs/CONTRIBUTING.md")); err == nil {
		t.Fatal("unselected component docs was written")
	}
}

func TestInitPythonWithAndWithoutAgent(t *testing.T) {
	withAgent := filepath.Join(t.TempDir(), "with-agent")
	code, out, stderr := run(t, []string{"init", "python", "demo", "--target", withAgent, "--yes"}, false, "")
	if code != 0 {
		t.Fatalf("code %d: %s %s", code, out, stderr)
	}
	for _, rel := range []string{".pre-commit-config.yaml", "AGENTS.md", ".agents/skills/dev-flow/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(withAgent, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(withAgent, "package.json")); err == nil {
		t.Fatal("Husky/Node package.json should not be generated")
	}

	withoutAgent := filepath.Join(t.TempDir(), "without-agent")
	code, out, stderr = run(t, []string{"init", "python", "demo", "--target", withoutAgent, "--no-agent", "--yes"}, false, "")
	if code != 0 {
		t.Fatalf("code %d: %s %s", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Join(withoutAgent, ".pre-commit-config.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(withoutAgent, "AGENTS.md")); err == nil {
		t.Fatal("AGENTS.md should not be generated with --no-agent")
	}
}
