package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/colorprofile"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
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
	updatePrompt
	updating
	updateResult
	serverLoading
	serverList
	serverDetail
	logLoading
	accessLogs
	sshFailure
	keyLoading
	keyList
	keyDetail
	keyDeleteConfirmation
	projectPicker
	projectAdd
)

type Action struct {
	SSHAlias    string
	GenerateKey *sshmanager.KeyGenerateRequest
	Zen         bool
	Monitor     bool
	Git         bool
	Workspace   bool
}

type Options struct {
	Request   project.InitRequest
	DryRun    bool
	StartInit bool
	NoColor   bool
	Version   string
	SSH       sshmanager.Manager
	Clipboard func(string) error
	Context   func(context.Context, string) ProjectContext
	Workspace *workspace.Service
	Terminal  *platform.ShellSession
	Session   *Session
	Notice    string
}

// Session keeps command results when Home hands control to another tool.
type Session struct {
	results  []shellResult
	selected int
	follow   bool
	hidden   bool
}

type ProjectContext struct {
	Project string
	Branch  string
	Changes int
}

type Model struct {
	planner            project.Planner
	executor           scaffold.Executor
	auth               auth.Authenticator
	updater            updater.Service
	templates          []templates.TemplateMetadata
	ctx                context.Context
	cancel             context.CancelFunc
	opts               Options
	screen             screen
	previous           screen
	cursor             int
	focus              int
	confirmYes         bool
	inputs             []textinput.Model
	command            textinput.Model
	homeCategory       string
	search             textinput.Model
	searching          bool
	progress           spinner.Model
	viewport           viewport.Model
	width              int
	height             int
	plan               *scaffold.Plan
	result             *scaffold.Result
	err                error
	formError          string
	canceling          bool
	authUser           *auth.User
	authDevice         *auth.DeviceAuthorization
	authStore          string
	authText           string
	authWarn           string
	authOK             bool
	logoutYes          bool
	updateRelease      *updater.Release
	updateStep         string
	updateErr          error
	updateNotice       string
	updateResultOK     bool
	updateResultCursor int
	sshManager         sshmanager.Manager
	servers            []sshmanager.Server
	logs               []sshmanager.AccessLog
	serverConnect      bool
	sshAlias           string
	sshErr             error
	keys               []sshmanager.Key
	keyQuery           string
	keyNotice          string
	keyDeleteYes       bool
	clipboard          func(string) error
	action             Action
	projectContext     ProjectContext
	shellNotice        string
	historyCursor      int
	historyDraft       string
	shellJob           *shellJob
	shellResults       []shellResult
	shellSelected      int
	shellHistory       bool
	completion         *shellCompletion
	shellFollow        bool
	shellHidden        bool
	projects           projectSwitchState
}

type (
	contextMsg struct {
		dir     string
		context ProjectContext
	}
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
	updateCheckedMsg struct {
		result *updater.CheckResult
		err    error
		manual bool
	}
	updateFinishedMsg struct {
		err error
	}
	serversLoadedMsg struct {
		servers []sshmanager.Server
		err     error
		connect bool
	}
	logsLoadedMsg struct {
		logs []sshmanager.AccessLog
		err  error
	}
	keysLoadedMsg struct {
		keys  []sshmanager.Key
		query string
		err   error
	}
	keyCopiedMsg struct {
		name string
		err  error
	}
	keyDeletedMsg struct {
		name string
		err  error
	}
)

func NewModel(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, authenticator auth.Authenticator, updaterService updater.Service, opts Options) Model {
	if opts.Terminal == nil {
		dir, _ := os.Getwd()
		opts.Terminal = &platform.ShellSession{Dir: dir}
	}
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:           ctx,
		shellNotice:   opts.Notice,
		cancel:        cancel,
		planner:       planner,
		executor:      executor,
		auth:          authenticator,
		updater:       updaterService,
		sshManager:    opts.SSH,
		clipboard:     opts.Clipboard,
		templates:     metadata,
		opts:          opts,
		width:         80,
		height:        30,
		viewport:      viewport.New(viewport.WithWidth(76), viewport.WithHeight(24)),
		historyCursor: len(opts.Terminal.History),
	}
	if opts.Session != nil {
		m.shellResults = opts.Session.results
		m.shellSelected, m.shellFollow = opts.Session.selected, opts.Session.follow
		m.shellHidden = opts.Session.hidden
	}
	if opts.Notice != "" {
		m.shellFollow = false
	}
	m.progress = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	if !opts.NoColor {
		m.progress.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(style.Accent))
	}
	if m.clipboard == nil {
		m.clipboard = clipboard.WriteAll
	}
	m.command = textinput.New()
	m.command.Prompt = ""
	m.command.Placeholder = "Run a command… (/ for AIAI menus)"
	m.command.CharLimit = 8192
	m.styleInput(&m.command)
	m.command.Focus()
	m.search = textinput.New()
	m.search.Prompt = "/ "
	m.search.Placeholder = "Search name, host, user, or fingerprint…"
	m.search.CharLimit = 128
	m.styleInput(&m.search)
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

func Run(ctx context.Context, planner project.Planner, executor scaffold.Executor, metadata []templates.TemplateMetadata, authenticator auth.Authenticator, updaterService updater.Service, in io.Reader, out io.Writer, opts Options) (*scaffold.Result, *scaffold.Plan, Action, error) {
	m := NewModel(ctx, planner, executor, metadata, authenticator, updaterService, opts)
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
		return nil, nil, Action{}, fmt.Errorf("%w: terminal: %v", scaffold.ErrExecution, err)
	}
	f := final.(Model)
	f.saveSession()
	if f.action.SSHAlias == "" {
		f.action.SSHAlias = f.sshAlias
	}
	return f.result, f.plan, f.action, f.err
}

func (m Model) saveSession() {
	if m.opts.Session != nil {
		*m.opts.Session = Session{results: m.shellResults, selected: m.shellSelected, follow: m.shellFollow, hidden: m.shellHidden}
	}
}

func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{textinput.Blink}
	if m.opts.Context != nil {
		commands = append(commands, m.loadContext())
	}
	if m.auth != nil && !m.opts.StartInit {
		commands = append(commands, func() tea.Msg {
			result, err := m.auth.Cached()
			return authCachedMsg{result: result, err: err}
		})
	}
	// Startup is offline. /update remains the explicit network entry point.
	return tea.Batch(commands...)
}

func (m Model) theme() Theme {
	width := min(100, m.width-4)
	if m.showingShellOutput() {
		width = m.width - 4
	}
	return NewTheme(max(1, width), m.opts.NoColor)
}

func (m Model) busy() bool {
	if (m.screen == projectPicker || m.screen == projectAdd) && m.projects.busy {
		return true
	}
	switch m.screen {
	case planning, applying, authStarting, authWaiting, loggingOut, updating, serverLoading, logLoading, keyLoading:
		return true
	default:
		return false
	}
}

func (m Model) withProgress(cmd tea.Cmd) tea.Cmd {
	return tea.Batch(cmd, m.progress.Tick)
}

func (m Model) styleInput(input *textinput.Model) {
	styles := textinput.Styles{}
	if !m.opts.NoColor {
		styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(style.Muted))
		styles.Blurred.Placeholder = styles.Focused.Placeholder
		styles.Cursor.Color = lipgloss.Color(style.Accent)
	}
	styles.Cursor.Blink = true
	input.SetStyles(styles)
}

func (m *Model) resize() {
	width := m.theme().Width()
	m.command.SetWidth(max(1, width-6))
	m.search.SetWidth(max(1, width-6))
	m.projects.input.SetWidth(max(1, width-4))
	if m.screen == projectPicker {
		m.projects.input.SetWidth(max(1, width-len("> Search: ")-1))
	}
	m.projects.name.SetWidth(max(1, width-4))
	for i := range m.inputs {
		m.inputs[i].SetWidth(max(1, width-4))
	}
	m.viewport.SetWidth(width)
	top, bottom := m.chrome()
	m.viewport.SetHeight(max(1, m.height-lipgloss.Height(top)-lipgloss.Height(bottom)-2))
}

func (m Model) visibleServers() []sshmanager.Server {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if query == "" {
		return m.servers
	}
	servers := make([]sshmanager.Server, 0, len(m.servers))
	for _, server := range m.servers {
		if strings.Contains(strings.ToLower(server.Alias+" "+server.Username+" "+server.Host+" "+server.Description), query) {
			servers = append(servers, server)
		}
	}
	return servers
}

func (m Model) visibleKeys() []sshmanager.Key {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if query == "" {
		return m.keys
	}
	keys := make([]sshmanager.Key, 0, len(m.keys))
	for _, key := range m.keys {
		haystack := key.Name + " " + key.Type + " " + key.Comment + " " + key.Fingerprint + " " + key.PrivatePath
		if strings.Contains(strings.ToLower(haystack), query) {
			keys = append(keys, key)
		}
	}
	return keys
}

func (m *Model) clearSearch() {
	m.search.SetValue("")
	m.search.Blur()
	m.searching = false
}
