package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
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

type unreadableZen struct{ zen.Repository }

func (unreadableZen) Load(context.Context) (zen.Config, []zen.Session, error) {
	return zen.Config{}, nil, errors.New("damaged Zen data")
}

func TestHomeToolsReturnResultsWithoutCancelingHome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := newSSHManager(t, &commandRunner{})
	manager.KeyTool = commandKeyTool{}
	manager.KeyDir = t.TempDir()
	if _, err := manager.Add(ctx, sshmanager.AddRequest{Alias: "example", Host: "example.invalid", Username: "tester", Port: 22}); err != nil {
		t.Fatal(err)
	}
	s := &state{deps: Dependencies{SSH: manager, Zen: unreadableZen{}, Interactive: func() bool { return true }}}
	notice, err := s.homeAction(ctx, tui.Action{SSHAlias: "example"})
	if err != nil || !strings.Contains(notice, "SSH session ended") {
		t.Fatalf("SSH did not return a Home result: %s %v", notice, err)
	}
	notice, err = s.homeAction(ctx, tui.Action{GenerateKey: &sshmanager.KeyGenerateRequest{Name: "laptop", NoPassphrase: true}})
	if err != nil || !strings.Contains(notice, "laptop.pub") || !strings.Contains(notice, "/keys") {
		t.Fatalf("key generation did not return a Home result: %s %v", notice, err)
	}
	_, err = s.homeAction(ctx, tui.Action{Zen: true})
	if err == nil || !strings.Contains(err.Error(), "damaged Zen data") || ctx.Err() != nil {
		t.Fatalf("Zen failure was hidden or canceled Home: %v", err)
	}
}
