package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	command    textinput.Model
	workspace  string
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
	m.workspace, _ = os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(m.workspace, home+string(os.PathSeparator)) {
		m.workspace = "~" + strings.TrimPrefix(m.workspace, home)
	}
	m.command = textinput.New()
	m.command.Prompt = ""
	m.command.Placeholder = "Type a command, or choose below…"
	m.command.CharLimit = 64
	m.styleInput(&m.command)
	m.command.Focus()
	for i, value := range []string{opts.Request.ProjectName, opts.Request.TargetDir, opts.Request.PackageName} {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = []string{"my-project", "./my-project or .", "my_project"}[i]
		input.CharLimit = 240
		input.SetWidth(64)
		input.SetVirtualCursor(true)
		input.SetValue(value)
		m.styleInput(&input)
		m.inputs = append(m.inputs, input)
	}
	if opts.StartInit {
		m.screen = selection
	}
	m.resize()
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

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m Model) theme() screens.Theme {
	return screens.Theme{Width: max(1, min(100, m.width-4)), NoColor: m.opts.NoColor}
}

func (m Model) styleInput(input *textinput.Model) {
	styles := textinput.Styles{}
	if !m.opts.NoColor {
		styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
		styles.Blurred.Placeholder = styles.Focused.Placeholder
		styles.Cursor.Color = lipgloss.Color("#FF5F56")
	}
	styles.Cursor.Blink = true
	input.SetStyles(styles)
}

func (m *Model) resize() {
	width := m.theme().Width
	m.command.SetWidth(max(1, width-2))
	for i := range m.inputs {
		m.inputs[i].SetWidth(max(1, width-4))
	}
	m.viewport.SetWidth(width)
	// Measure the actual chrome, including wrapped help on narrow terminals.
	top, bottom := m.chrome()
	m.viewport.SetHeight(max(1, m.height-lipgloss.Height(top)-lipgloss.Height(bottom)-2))
}

func (m Model) chrome() (string, string) {
	fit := lipgloss.NewStyle().Width(m.theme().Width)
	return fit.Render(m.header() + "\n" + m.toolbar()), fit.Render("\n" + m.footer())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cancelMsg:
		return m.stop()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
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
	if m.screen == home {
		return m.updateCommand(msg)
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
	t := m.theme()
	var body string
	switch m.screen {
	case home:
		body = screens.Home(m.cursor, m.command.Value(), t)
	case selection:
		body = t.Heading("Choose a template", "A solid starting point, ready to make your own.")
		if len(m.templates) == 0 {
			body += t.Muted("No templates available. Esc to return.")
		}
		for i, t := range m.templates {
			body += m.theme().Choice(t.DisplayName, "", i == m.cursor) + "  " + m.theme().Muted(t.Description) + "\n\n"
		}
	case form:
		values := make([]string, len(m.inputs))
		for i := range m.inputs {
			values[i] = m.inputs[i].View()
		}
		body = screens.InitForm(values, m.focus, m.opts.Request.Force, m.opts.DryRun, t)
		if m.formError != "" {
			body = t.Accent("! "+m.formError) + "\n\n" + body
		}
	case planning:
		body = t.Heading("Building your preview…", "Rendering the template and checking destination files.")
	case preview:
		body = screens.Preview(*m.plan, m.opts.DryRun, t)
	case confirmation:
		body = t.Heading("Ready to create?", "Apply the file operations you just reviewed.") + t.Muted(m.plan.TargetDir) + "\n\n" +
			t.Choice("No, return to preview", "", !m.confirmYes) + t.Choice("Yes, generate files", "", m.confirmYes)
	case applying:
		body = t.Heading("Creating your project…", "Writing the files from your approved plan.")
		if m.canceling {
			body = t.Warn("Canceling; waiting for the current file to finish…")
		}
	case result:
		body = screens.Result(*m.result, m.err, t)
	case help:
		body = screens.Help(t)
	case about:
		body = t.Heading("Small tool. Solid foundations.", "AIAI CLI · "+m.opts.Version) + "Deterministic project scaffolding.\nEmbedded templates. Offline generation.\n\n" + t.Muted("Choose a template, review the plan, and start building.")
	}
	return lipgloss.NewStyle().Width(t.Width).Render(body)
}

func (m Model) View() tea.View {
	if m.width < 24 || m.height < 12 {
		v := tea.NewView(lipgloss.NewStyle().Width(max(1, m.width)).MaxHeight(max(1, m.height)).Render("AIAI\nEnlarge terminal\nCtrl+C to exit"))
		v.AltScreen = true
		return v
	}
	m.resize()
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
	top, bottom := m.chrome()
	view := top + "\n" + m.viewport.View() + "\n" + bottom
	v := tea.NewView(lipgloss.NewStyle().PaddingLeft(2).Render(view))
	v.AltScreen = true
	return v
}
