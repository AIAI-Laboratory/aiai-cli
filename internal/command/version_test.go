package command

import (
	"strings"
	"testing"
)

func TestNonTTYAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"init"}, {"init", "python"}} {
		_, out, stderr := run(t, args, false, "")
		if strings.Contains(out+stderr, "\x1b") {
			t.Fatal("terminal UI launched without TTY")
		}
	}
	code, out, _ := run(t, []string{"version", "--json"}, false, "")
	if code != 0 || !strings.Contains(out, `"version": "test"`) {
		t.Fatal(out)
	}
}

func TestVersionPlain(t *testing.T) {
	code, out, _ := run(t, []string{"version"}, false, "")
	if code != 0 || !strings.Contains(out, "aiai test") {
		t.Fatalf("expected version output 'aiai test', got: %q", out)
	}
}
