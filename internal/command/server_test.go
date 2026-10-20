package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

type commandRunner struct {
	exitCode int
	args     []string
}

func (r *commandRunner) Run(_ context.Context, args []string, _ sshmanager.Streams) (sshmanager.RunResult, error) {
	r.args = append([]string(nil), args...)
	return sshmanager.RunResult{ExitCode: r.exitCode}, nil
}

func newSSHManager(t *testing.T, runner sshmanager.Runner) *sshmanager.Service {
	t.Helper()
	repository, err := sshmanager.NewJSONRepository(filepath.Join(t.TempDir(), "ssh.json"))
	if err != nil {
		t.Fatal(err)
	}
	return sshmanager.NewService(repository, runner)
}

type commandKeyTool struct{}

func (commandKeyTool) Generate(_ context.Context, args []string, _ sshmanager.Streams) error {
	target := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" {
			target = args[i+1]
		}
	}
	if err := os.WriteFile(target, []byte("PRIVATE-CONTENT"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(target+".pub", []byte("ssh-ed25519 AQID command-test\n"), 0o644)
}

func runWithSSH(t *testing.T, args []string, interactive bool, input string, manager sshmanager.Manager) (int, string, string) {
	t.Helper()
	registry, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps := Dependencies{
		Planner:     project.Initializer{Engine: scaffold.Engine{Registry: registry}},
		Executor:    scaffold.FileExecutor{},
		Registry:    registry,
		SSH:         manager,
		In:          strings.NewReader(input),
		Out:         &stdout,
		Err:         &stderr,
		Version:     "test",
		Interactive: func() bool { return interactive },
	}
	code := Run(context.Background(), args, deps)
	return code, stdout.String(), stderr.String()
}

func TestServerCommandsAndJSONContract(t *testing.T) {
	manager := newSSHManager(t, &commandRunner{})
	code, stdout, stderr := runWithSSH(t, []string{"server", "add", "Production", "--host", "2001:db8::1", "--user", "ubuntu", "--json"}, false, "", manager)
	if code != 0 {
		t.Fatalf("add code = %d, stderr = %s", code, stderr)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("add JSON: %v: %s", err, stdout)
	}
	if envelope.SchemaVersion != 1 || envelope.SSH == nil || envelope.SSH.Server == nil || envelope.SSH.Server.Alias != "production" {
		t.Fatalf("add envelope = %+v", envelope)
	}

	code, stdout, stderr = runWithSSH(t, []string{"server", "update", "production", "--port", "2222", "--description", "primary", "--json"}, false, "", manager)
	if code != 0 || stderr != "" {
		t.Fatalf("update code = %d, stderr = %s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.SSH.Server.Port != 2222 {
		t.Fatalf("update envelope = %+v, %v", envelope, err)
	}

	code, stdout, stderr = runWithSSH(t, []string{"server", "list", "--json"}, false, "", manager)
	if code != 0 || stderr != "" {
		t.Fatalf("list code = %d, stderr = %s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.SSH.Servers == nil || len(*envelope.SSH.Servers) != 1 {
		t.Fatalf("list envelope = %+v, %v", envelope, err)
	}
}

func TestServerKeyAndRemovalConfirmation(t *testing.T) {
	manager := newSSHManager(t, &commandRunner{})
	code, _, stderr := runWithSSH(t, []string{"server", "add", "prod", "--host", "example.com", "--user", "deploy"}, false, "", manager)
	if code != 0 {
		t.Fatal(stderr)
	}
	key := filepath.Join(t.TempDir(), "id")
	if err := os.WriteFile(key, []byte("reference-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = runWithSSH(t, []string{"server", "key", "set", "prod", key}, false, "", manager)
	if code != 0 {
		t.Fatalf("key set code = %d, stderr = %s", code, stderr)
	}
	code, stdout, _ := runWithSSH(t, []string{"server", "key", "show", "prod"}, false, "", manager)
	if code != 0 || !strings.Contains(stdout, key) {
		t.Fatalf("key show code = %d, stdout = %s", code, stdout)
	}
	code, _, _ = runWithSSH(t, []string{"server", "remove", "prod"}, false, "", manager)
	if code != 2 {
		t.Fatalf("unconfirmed remove code = %d, want 2", code)
	}
	code, _, stderr = runWithSSH(t, []string{"server", "remove", "prod", "--yes"}, false, "", manager)
	if code != 0 {
		t.Fatalf("confirmed remove code = %d, stderr = %s", code, stderr)
	}
}

func TestSSHExitCodeLoggingAndJSONRejection(t *testing.T) {
	runner := &commandRunner{exitCode: 37}
	manager := newSSHManager(t, runner)
	code, _, stderr := runWithSSH(t, []string{"server", "add", "prod", "--host", "example.com", "--user", "deploy"}, false, "", manager)
	if code != 0 {
		t.Fatal(stderr)
	}
	code, _, _ = runWithSSH(t, []string{"ssh", "prod"}, true, "", manager)
	if code != 37 {
		t.Fatalf("ssh exit code = %d, want 37", code)
	}
	code, stdout, _ := runWithSSH(t, []string{"log", "list", "--server", "prod", "--limit", "1", "--json"}, false, "", manager)
	if code != 0 {
		t.Fatalf("log list code = %d", code)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.SSH == nil || envelope.SSH.Logs == nil || len(*envelope.SSH.Logs) != 1 || (*envelope.SSH.Logs)[0].ExitCode == nil || *(*envelope.SSH.Logs)[0].ExitCode != 37 {
		t.Fatalf("log envelope = %+v, %v", envelope, err)
	}
	code, stdout, _ = runWithSSH(t, []string{"ssh", "prod", "--json"}, false, "", manager)
	if code != 2 {
		t.Fatalf("ssh --json code = %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.Error == nil || !strings.Contains(envelope.Error.Message, "not supported") {
		t.Fatalf("ssh --json envelope = %+v, %v", envelope, err)
	}
}

func TestKeyFindJSONAndGenerationSafety(t *testing.T) {
	manager := newSSHManager(t, &commandRunner{})
	manager.KeyTool = commandKeyTool{}
	manager.KeyDir = filepath.Join(t.TempDir(), "managed-keys")
	code, stdout, stderr := runWithSSH(t, []string{"key", "find", "definitely-no-such-aiai-key", "--json"}, false, "", manager)
	if code != 0 || stderr != "" {
		t.Fatalf("key find code = %d, stderr = %s", code, stderr)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.SSH == nil || envelope.SSH.Keys == nil || len(*envelope.SSH.Keys) != 0 {
		t.Fatalf("key find envelope = %+v, %v", envelope, err)
	}
	code, _, stderr = runWithSSH(t, []string{"key", "generate", "unsafe-noninteractive"}, false, "", manager)
	if code != 2 || !strings.Contains(stderr, "--no-passphrase") {
		t.Fatalf("noninteractive generation code = %d, stderr = %s", code, stderr)
	}
	code, stdout, stderr = runWithSSH(t, []string{"key", "generate", "automation", "--no-passphrase", "--json"}, false, "", manager)
	if code != 0 {
		t.Fatalf("safe generation code = %d, stderr = %s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.SSH == nil || envelope.SSH.Key == nil || envelope.SSH.Key.Name != "automation" {
		t.Fatalf("key generation envelope = %+v, %v", envelope, err)
	}
	if strings.Contains(stdout, "PRIVATE-CONTENT") {
		t.Fatal("private key content leaked into JSON")
	}
}
