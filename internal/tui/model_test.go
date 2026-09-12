package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

type fakePlanner struct{}

func (fakePlanner) Plan(_ context.Context, req project.InitRequest) (scaffold.Plan, error) {
	return scaffold.Plan{TargetDir: req.TargetDir, Operations: []scaffold.Operation{{Path: "pyproject.toml", Action: scaffold.Create}}}, nil
}

type fakeExecutor struct{ calls *int }

func (f fakeExecutor) Apply(_ context.Context, p scaffold.Plan) (scaffold.Result, error) {
	*f.calls++
	r := scaffold.NewResult(p.TargetDir)
	r.Completed = []string{"pyproject.toml"}
	return r, nil
}

func press(m Model, code rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return next.(Model), cmd
}

func TestWizardRequiresPreviewAndConfirmation(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python-minimal", DisplayName: "Python"}}, Options{})
	defer m.cancel()
	m, _ = press(m, tea.KeyEnter)
	if m.screen != selection {
		t.Fatal(m.screen)
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != form {
		t.Fatal(m.screen)
	}
	m.inputs[0].SetValue("demo")
	m.focus = 5
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != planning || calls != 0 {
		t.Fatal("wrote before planning")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.screen != preview {
		t.Fatal(m.screen)
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != confirmation || m.confirmYes {
		t.Fatal("confirmation must default to no")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != preview || calls != 0 {
		t.Fatal("default confirmation wrote files")
	}
	m, _ = press(m, tea.KeyEnter)
	m, _ = press(m, tea.KeyDown)
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != applying {
		t.Fatal(m.screen)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.screen != result || calls != 1 || len(m.result.Completed) != 1 {
		t.Fatal("generation did not complete")
	}
}

func TestWizardHelpResizeBackAndCancel(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, Options{})
	defer m.cancel()
	m, _ = press(m, '?')
	if m.screen != help {
		t.Fatal(m.screen)
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != home {
		t.Fatal(m.screen)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	m = next.(Model)
	if m.viewport.Width() != 28 || m.viewport.Height() != 6 {
		t.Fatal("resize not applied")
	}
	if !strings.Contains(m.View().Content, "AIAI") {
		t.Fatal("view has no title")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.err != context.Canceled || cmd == nil || calls != 0 {
		t.Fatal("cancel did not stop without writing")
	}
}

func TestDryRunAndConflictNeverApply(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		calls := 0
		m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, Options{DryRun: true})
		m.screen = preview
		action := scaffold.Create
		if conflict {
			action = scaffold.Conflict
		}
		m.plan = &scaffold.Plan{Operations: []scaffold.Operation{{Path: "x", Action: action}}}
		m, cmd := press(m, tea.KeyEnter)
		if calls != 0 {
			t.Fatal("applied dry run")
		}
		if conflict && cmd != nil {
			t.Fatal("conflicting preview accepted")
		}
		m.cancel()
	}
}

func TestPreviewScrollAndPartialCancellation(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, Options{})
	defer m.cancel()
	m.screen = preview
	m.plan = &scaffold.Plan{}
	for range 60 {
		m.plan.Operations = append(m.plan.Operations, scaffold.Operation{Path: "example", Action: scaffold.Create})
	}
	m, _ = press(m, tea.KeyDown)
	if m.viewport.YOffset() == 0 {
		t.Fatal("preview did not scroll")
	}
	m.screen = applying
	next, cmd := m.Update(cancelMsg{})
	m = next.(Model)
	if cmd != nil || !m.canceling {
		t.Fatal("must wait for partial result")
	}
	r := scaffold.NewResult("demo")
	r.Completed = []string{"first.py"}
	next, cmd = m.Update(appliedMsg{result: r, err: context.Canceled})
	m = next.(Model)
	if cmd == nil || m.err != context.Canceled || len(m.result.Completed) != 1 {
		t.Fatal("partial result lost")
	}
}
