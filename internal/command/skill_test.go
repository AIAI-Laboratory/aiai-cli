package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	"github.com/AIAI-Laboratory/aiai-cli/internal/skill"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

type fakeSkillService struct {
	result    *skill.Result
	err       error
	action    string
	names     []string
	scope     skill.Scope
	all       bool
	overwrite bool
}

func (f *fakeSkillService) Available(context.Context) (*skill.Result, error) {
	f.action = "available"
	return f.result, f.err
}

func (f *fakeSkillService) List(scope skill.Scope) (*skill.Result, error) {
	f.action, f.scope = "list", scope
	return f.result, f.err
}

func (f *fakeSkillService) Install(_ context.Context, names []string, scope skill.Scope, all, overwrite bool) (*skill.Result, error) {
	f.action, f.names, f.scope, f.all, f.overwrite = "install", names, scope, all, overwrite
	return f.result, f.err
}

func (f *fakeSkillService) Remove(names []string, scope skill.Scope, all bool) (*skill.Result, error) {
	f.action, f.names, f.scope, f.all = "remove", names, scope, all
	return f.result, f.err
}

func (f *fakeSkillService) Update(_ context.Context, names []string, scope skill.Scope, all bool) (*skill.Result, error) {
	f.action, f.names, f.scope, f.all = "update", names, scope, all
	return f.result, f.err
}

func TestSkillAddPassesSelectionAndScope(t *testing.T) {
	manager := &fakeSkillService{result: &skill.Result{Action: "install", Scope: skill.ScopeGlobal, Skills: []skill.Descriptor{{Name: "alpha"}}}}
	code, _, stderr := runWithSkills(t, []string{"skill", "add", "alpha", "--global", "--force"}, manager)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if manager.action != "install" || manager.scope != skill.ScopeGlobal || !manager.overwrite || manager.all {
		t.Fatalf("unexpected call: %+v", manager)
	}
	if len(manager.names) != 1 || manager.names[0] != "alpha" {
		t.Fatalf("names = %v", manager.names)
	}
}

func TestSkillAvailableJSON(t *testing.T) {
	manager := &fakeSkillService{result: &skill.Result{Action: "available", Skills: []skill.Descriptor{{Name: "alpha"}}}}
	code, stdout, stderr := runWithSkills(t, []string{"skills", "add", "--list", "--json"}, manager)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("unmarshal JSON: %v\n%s", err, stdout)
	}
	if envelope.Skill == nil || envelope.Skill.Action != "available" || len(envelope.Skill.Skills) != 1 {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if manager.action != "available" {
		t.Fatalf("action = %q", manager.action)
	}
}

func TestSkillListHumanOutput(t *testing.T) {
	manager := &fakeSkillService{result: &skill.Result{
		Action:      "list",
		Scope:       skill.ScopeProject,
		Destination: ".agents/skills",
		Skills:      []skill.Descriptor{{Name: "alpha", Description: "Alpha skill"}},
	}}
	code, stdout, stderr := runWithSkills(t, []string{"skill", "list"}, manager)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, "alpha — Alpha skill") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func runWithSkills(t *testing.T, args []string, manager skill.Service) (int, string, string) {
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
		Skills:      manager,
		In:          strings.NewReader(""),
		Out:         &stdout,
		Err:         &stderr,
		Version:     "test",
		Interactive: func() bool { return false },
	}
	code := Run(context.Background(), args, deps)
	return code, stdout.String(), stderr.String()
}
