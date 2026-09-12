package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

func TestCLIContracts(t *testing.T) {
	for _, tc := range []struct {
		name        string
		extra       []string
		input       string
		interactive bool
		code        int
		writes      bool
	}{
		{"dry run", []string{"--dry-run"}, "", false, 0, false},
		{"missing yes", nil, "", false, 2, false},
		{"yes", []string{"--yes"}, "", false, 0, true},
		{"confirmed", nil, "yes\n", true, 0, true},
		{"declined", nil, "n\n", true, 130, false},
		{"yes still asks in terminal", []string{"--yes"}, "\n", true, 130, false},
		{"eof cancels", nil, "", true, 130, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "generated")
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
