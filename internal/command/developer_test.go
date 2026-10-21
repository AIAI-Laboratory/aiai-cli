package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceCommandsJSONAndRemoval(t *testing.T) {
	t.Setenv("AIAI_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	for _, args := range [][]string{{"workspace", "add", dir, "--name", "example", "--json"}, {"workspace", "list", "--json"}, {"workspace", "show", "example", "--json"}, {"workspace", "path", "example", "--json"}, {"workspace", "activity", "example", "--json"}, {"workspace", "today", "--json"}, {"workspace", "remove", "example", "--json"}} {
		code, out, stderr := run(t, args, false, "")
		if code != 0 || stderr != "" {
			t.Fatalf("%v: %d %s %s", args, code, out, stderr)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil || envelope["workspace"] == nil || envelope["status"] != "ok" {
			t.Fatalf("JSON %s %v", out, err)
		}
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("workspace removal deleted files")
	}
}

func TestDeveloperErrorsAreJSON(t *testing.T) {
	t.Setenv("AIAI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{{"git", "status", "--repo", t.TempDir(), "--json"}, {"git", "history", "--since", "invalid", "--json"}, {"workspace", "activity", "missing", "--json"}, {"git", "repo", "add", t.TempDir(), "--json"}, {"workspace", "add", filepath.Join(t.TempDir(), "missing"), "--json"}} {
		code, out, _ := run(t, args, false, "")
		var env map[string]any
		if err := json.Unmarshal([]byte(out), &env); err != nil || code == 0 || env["error"] == nil {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
}

func TestDateParsingAndDeveloperHelp(t *testing.T) {
	for _, name := range []string{"git", "workspace"} {
		code, out, _ := run(t, []string{name, "--help"}, false, "")
		if code != 0 || !strings.Contains(out, "activity") {
			t.Fatalf("help %s %d %s", name, code, out)
		}
	}
	start, err := parseDate("2026-10-20", false)
	if err != nil {
		t.Fatal(err)
	}
	end, err := parseDate("2026-10-20", true)
	if err != nil || !end.After(start) || end.Day() != 20 {
		t.Fatalf("dates %v %v %v", start, end, err)
	}
}
