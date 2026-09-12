package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/screens"
)

type screen int

const (
	home screen = iota
	selection
	form
	planning
	preview
	confirmation
	applying
	result
	help
	about
)

type Options struct {
	Request   project.InitRequest
	DryRun    bool
	StartInit bool
	NoColor   bool
	Version   string
}

type Model struct {
	planner    project.Planner
	executor   scaffold.Executor
	templates  []templates.TemplateMetadata
	ctx        context.Context
	cancel     context.CancelFunc
	opts       Options
	screen     screen
	previous   screen
	cursor     int
	focus      int
	confirmYes bool
	inputs     []textinput.Model
	viewport   viewport.Model
	width      int
	height     int
	plan       *scaffold.Plan
	result     *scaffold.Result
	err        error
	formError  string
	canceling  bool
}

type (
	plannedMsg struct {
		plan scaffold.Plan
		err  error
	}
	appliedMsg struct {
		result scaffold.Result
		err    error
	}
	cancelMsg struct{}
)

func NewModel(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, opts Options) Model {
	ctx, cancel := context.WithCancel(ctx)
	m := Model{ctx: ctx, cancel: cancel, planner: planner, executor: executor, templates: metadata, opts: opts, width: 80, height: 30, viewport: viewport.New(viewport.WithWidth(76), viewport.WithHeight(24))}
	for i, value := range []string{opts.Request.ProjectName, opts.Request.TargetDir, opts.Request.PackageName} {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = []string{"my-project", "./my-project or .", "my_project"}[i]
		input.CharLimit = 240
		input.SetWidth(64)
		input.SetVirtualCursor(true)
		input.SetValue(value)
		m.inputs = append(m.inputs, input)
	}
	if opts.StartInit {
		m.screen = selection
	}
	return m
}

func Run(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, in io.Reader, out io.Writer, opts Options) (*scaffold.Result, *scaffold.Plan, error) {
	m := NewModel(ctx, planner, executor, metadata, opts)
	defer m.cancel()
	options := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out), tea.WithoutSignalHandler()}
	if opts.NoColor {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	p := tea.NewProgram(m, options...)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			p.Send(cancelMsg{})
		case <-done:
		}
	}()
	final, err := p.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: terminal: %v", scaffold.ErrExecution, err)
	}
	f := final.(Model)
	return f.result, f.plan, f.err
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cancelMsg:
		return m.stop()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.SetWidth(max(1, msg.Width-4))
		m.viewport.SetHeight(max(1, msg.Height-6))
		for i := range m.inputs {
			m.inputs[i].SetWidth(max(1, msg.Width-8))
		}
	case plannedMsg:
		if m.canceling {
			m.err = context.Canceled
			return m, tea.Quit
		}
		if msg.err != nil {
			m.formError = msg.err.Error()
			m.screen = form
			return m, m.setFocus(m.focus)
		}
		m.plan = &msg.plan
		m.screen = preview
		m.viewport.GotoTop()
	case appliedMsg:
		m.result, m.err = &msg.result, msg.err
		m.screen = result
		m.viewport.GotoTop()
		if m.canceling || errors.Is(msg.err, context.Canceled) {
			m.err = context.Canceled
			return m, tea.Quit
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.stop()
		}
		if m.screen == planning || m.screen == applying {
			return m, nil
		}
		return m.key(msg)
	}
	if m.screen == form && m.focus < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) stop() (tea.Model, tea.Cmd) {
	m.cancel()
	m.canceling = true
	if m.screen == applying || m.screen == planning {
		return m, nil
	}
	m.err = context.Canceled
	return m, tea.Quit
}

func (m Model) content() string {
	var body string
	switch m.screen {
	case home:
		body = screens.Home(m.cursor)
	case selection:
		body = "SELECT TEMPLATE\n\n"
		for i, t := range m.templates {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			body += prefix + t.DisplayName + "\n  " + t.Description + "\n"
		}
	case form:
		values := make([]string, len(m.inputs))
		for i := range m.inputs {
			values[i] = m.inputs[i].View()
		}
		body = screens.InitForm(values, m.focus, m.opts.Request.Force, m.opts.DryRun)
		if m.formError != "" {
			body += "\n" + m.formError
		}
	case planning:
		body = "Building the file plan…"
	case preview:
		body = screens.Preview(*m.plan, m.opts.DryRun)
	case confirmation:
		choices := "> No, return to preview\n  Yes, generate files"
		if m.confirmYes {
			choices = "  No, return to preview\n> Yes, generate files"
		}
		body = "APPLY THIS PLAN?\n\n" + m.plan.TargetDir + "\n\n" + choices
	case applying:
		body = "Generating project files…"
		if m.canceling {
			body = "Canceling; waiting for the current file to finish…"
		}
	case result:
		body = screens.Result(*m.result, m.err)
	case help:
		body = screens.Help
	case about:
		body = "ABOUT AIAI\n\nVersion: " + m.opts.Version + "\nDeterministic project scaffolding.\nEmbedded templates. Offline generation.\n\nEsc to return."
	}
	width := max(1, m.width-4)
	return lipgloss.NewStyle().Width(width).Render(body)
}

func (m Model) View() tea.View {
	content := m.content()
	m.viewport.SetContent(content)
	if m.screen == form || m.screen == home || m.screen == selection || m.screen == confirmation {
		for line, text := range strings.Split(content, "\n") {
			if strings.HasPrefix(text, "> ") {
				m.viewport.EnsureVisible(line, 0, 1)
				if m.screen == form && m.focus < 3 {
					m.viewport.EnsureVisible(line+1, 0, 1)
				}
				break
			}
		}
	}
	v := tea.NewView("\n" + title("AIAI  /  PROJECT TOOLS", m.opts.NoColor) + "\n\n" + m.viewport.View() + "\n\n↑/↓ navigate · Enter select · Esc back · ? help")
	v.AltScreen = true
	return v
}
