package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

func run(t *testing.T, args []string, interactive bool, input string) (int, string, string) {
	return runWithAuth(t, args, interactive, input, nil)
}

func runWithAuth(t *testing.T, args []string, interactive bool, input string, authenticator auth.Authenticator) (int, string, string) {
	t.Helper()
	r, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	deps := Dependencies{
		Planner:     project.Initializer{Engine: scaffold.Engine{Registry: r}},
		Executor:    scaffold.FileExecutor{},
		Registry:    r,
		In:          strings.NewReader(input),
		Out:         &out,
		Err:         &stderr,
		Version:     "test",
		Auth:        authenticator,
		Interactive: func() bool { return interactive },
	}
	code := Run(context.Background(), args, deps)
	return code, out.String(), stderr.String()
}

func TestExitCodes(t *testing.T) {
	for err, code := range map[error]int{
		nil:                    0,
		scaffold.ErrValidation: 2,
		scaffold.ErrConflict:   3,
		scaffold.ErrExecution:  4,
		context.Canceled:       130,
	} {
		if got := ExitCode(err); got != code {
			t.Errorf("%v: %d", err, got)
		}
	}
}

func TestJSONHelpAndParseErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--help"},
		{"init", "python", "--help", "--json"},
		{"--unknown", "--json"},
		{"missing", "--json"},
	} {
		_, out, _ := run(t, args, false, "")
		var e output.Envelope
		if err := json.Unmarshal([]byte(out), &e); err != nil {
			t.Fatalf("%v: non-JSON output: %s", args, out)
		}
	}
}
