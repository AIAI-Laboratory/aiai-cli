package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
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
	authStarting
	authWaiting
	authResult
	profile
	logoutConfirmation
	loggingOut
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
	auth       auth.Authenticator
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
	authUser   *auth.User
	authDevice *auth.DeviceAuthorization
	authStore  string
	authText   string
	authWarn   string
	authOK     bool
	logoutYes  bool
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
	cancelMsg     struct{}
	authCachedMsg struct {
		result auth.Result
		err    error
	}
	authStartedMsg struct {
		start auth.LoginStart
		err   error
	}
	authPollMsg struct {
		poll   auth.PollResult
		result auth.Result
		err    error
	}
	authPollTickMsg struct{}
	authWhoamiMsg   struct {
		result auth.Result
		err    error
	}
	authLogoutMsg struct {
		result auth.LogoutResult
		err    error
	}
)

func NewModel(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, authenticator auth.Authenticator, opts Options) Model {
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:       ctx,
		cancel:    cancel,
		planner:   planner,
		executor:  executor,
		auth:      authenticator,
		templates: metadata,
		opts:      opts,
		width:     80,
		height:    30,
		viewport:  viewport.New(viewport.WithWidth(76), viewport.WithHeight(24)),
	}
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

func Run(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, authenticator auth.Authenticator, in io.Reader, out io.Writer, opts Options) (*scaffold.Result, *scaffold.Plan, error) {
	m := NewModel(ctx, planner, executor, metadata, authenticator, opts)
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

func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{textinput.Blink}
	if m.auth != nil && !m.opts.StartInit {
		commands = append(commands, func() tea.Msg {
			result, err := m.auth.Cached()
			return authCachedMsg{result: result, err: err}
		})
	}
	return tea.Batch(commands...)
}

func (m Model) theme() Theme {
	return NewTheme(max(1, min(100, m.width-4)), m.opts.NoColor)
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
	width := m.theme().Width()
	m.command.SetWidth(max(1, width-2))
	for i := range m.inputs {
		m.inputs[i].SetWidth(max(1, width-4))
	}
	m.viewport.SetWidth(width)
	top, bottom := m.chrome()
	m.viewport.SetHeight(max(1, m.height-lipgloss.Height(top)-lipgloss.Height(bottom)-2))
}
