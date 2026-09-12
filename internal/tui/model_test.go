package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	if m.viewport.Width() != 28 || m.viewport.Height() < 1 || lipgloss.Height(m.View().Content) > 12 {
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

func typeCommand(m Model, value string) Model {
	for _, r := range value {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	return m
}

func TestCommandPalette(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, Options{})
	defer m.cancel()
	m = typeCommand(m, "/abo")
	if !strings.Contains(m.content(), "/about") || strings.Contains(m.content(), "/init") {
		t.Fatal("command filter did not narrow the menu")
	}
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "/about" {
		t.Fatal("Tab did not complete the command")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != about || m.command.Value() != "" {
		t.Fatal("filtered command did not open About")
	}
	m, _ = press(m, tea.KeyEscape)
	m = typeCommand(m, "/missing")
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != home || !strings.Contains(m.content(), "No matching commands") {
		t.Fatal("empty results must not execute a command")
	}
	m, cmd := press(m, tea.KeyEscape)
	if cmd != nil || m.command.Value() != "" {
		t.Fatal("Esc must clear the filter before quitting")
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	next, _ := m.Update(tea.PasteMsg{Content: "/about"})
	m = next.(Model)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != about {
		t.Fatal("pasted filter retained a stale selection")
	}
	m, _ = press(m, tea.KeyEscape)
	m = typeCommand(m, "/init")
	m, _ = press(m, tea.KeyEnter)
	if m.screen != selection || calls != 0 {
		t.Fatal("init must open the wizard without writing files")
	}
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{18, 8}, {24, 10}, {32, 12}, {60, 22}, {80, 24}, {120, 40}} {
		for _, s := range []screen{home, selection, form, planning, preview, confirmation, applying, result, help, about} {
			t.Run(fmt.Sprintf("%dx%d/screen%d", size[0], size[1], s), func(t *testing.T) {
				calls := 0
				m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python", DisplayName: "Python", Description: "A minimal Python project"}}, Options{NoColor: true})
				defer m.cancel()
				m.screen = s
				m.plan = &scaffold.Plan{TargetDir: strings.Repeat("long-path/", 15)}
				m.result = &scaffold.Result{TargetDir: "demo", Completed: []string{"pyproject.toml"}}
				m.focus = 5
				next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = next.(Model)
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("view is %dx%d, terminal is %dx%d", lipgloss.Width(view), lipgloss.Height(view), size[0], size[1])
				}
				if strings.Contains(view, "[38;") || strings.Contains(view, "[48;") {
					t.Fatal("color emitted with NoColor")
				}
				if s == form && size[0] >= 32 && size[1] >= 12 && !strings.Contains(view, "Preview project") {
					t.Fatal("focused action is hidden")
				}
			})
		}
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
