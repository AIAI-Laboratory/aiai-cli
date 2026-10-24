package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/screens"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
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

type fakeAuth struct {
	polls int
	user  auth.User
}

type fakeSSHManager struct {
	servers []sshmanager.Server
	logs    []sshmanager.AccessLog
	removed string
}

func (f *fakeSSHManager) Add(context.Context, sshmanager.AddRequest) (sshmanager.Server, error) {
	return sshmanager.Server{}, nil
}

func (f *fakeSSHManager) List(context.Context) ([]sshmanager.Server, error) { return f.servers, nil }

func (f *fakeSSHManager) Show(context.Context, string) (sshmanager.Server, error) {
	return sshmanager.Server{}, nil
}

func (f *fakeSSHManager) Update(context.Context, string, sshmanager.UpdateRequest) (sshmanager.Server, error) {
	return sshmanager.Server{}, nil
}
func (f *fakeSSHManager) Remove(context.Context, string) error { return nil }
func (f *fakeSSHManager) SetKey(context.Context, string, string) (sshmanager.Server, error) {
	return sshmanager.Server{}, nil
}
func (f *fakeSSHManager) ShowKey(context.Context, string) (string, error) { return "", nil }
func (f *fakeSSHManager) ClearKey(context.Context, string) (sshmanager.Server, error) {
	return sshmanager.Server{}, nil
}
func (f *fakeSSHManager) Connect(context.Context, string, sshmanager.Streams) error { return nil }
func (f *fakeSSHManager) Logs(context.Context, sshmanager.LogFilter) ([]sshmanager.AccessLog, error) {
	return f.logs, nil
}

func (f *fakeSSHManager) GenerateKey(context.Context, sshmanager.KeyGenerateRequest, sshmanager.Streams) (sshmanager.Key, error) {
	return sshmanager.Key{}, nil
}

func (f *fakeSSHManager) FindKeys(context.Context, string) ([]sshmanager.Key, error) {
	return []sshmanager.Key{{Name: "laptop", Type: "ssh-ed25519", Fingerprint: "SHA256:test", PrivatePath: "/keys/laptop", PublicPath: "/keys/laptop.pub", Managed: true}}, nil
}

func (f *fakeSSHManager) FindKey(context.Context, string) (sshmanager.Key, error) {
	return sshmanager.Key{Name: "laptop"}, nil
}

func (f *fakeSSHManager) PublicKey(context.Context, string) (string, error) {
	return "ssh-ed25519 AQID laptop", nil
}

func (f *fakeSSHManager) RemoveKey(_ context.Context, value string) error {
	f.removed = value
	return nil
}

func (f *fakeAuth) StartLogin(context.Context, auth.LoginOptions) (auth.LoginStart, error) {
	return auth.LoginStart{Device: auth.DeviceAuthorization{AuthID: "flow", UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device", ExpiresAt: time.Now().Add(time.Minute), IntervalSeconds: 1}}, nil
}

func (f *fakeAuth) PollLogin(context.Context, auth.DeviceAuthorization) (auth.PollResult, error) {
	f.polls++
	if f.polls == 1 {
		return auth.PollResult{Pending: true, RetryAfterSeconds: 1}, nil
	}
	return auth.PollResult{User: f.user}, nil
}

func (f *fakeAuth) Login(context.Context, auth.LoginOptions, func(auth.DeviceAuthorization)) (auth.Result, error) {
	return auth.Result{Authenticated: true, User: f.user, Storage: "keyring"}, nil
}

func (f *fakeAuth) WhoAmI(context.Context) (auth.Result, error) {
	return auth.Result{Authenticated: true, User: f.user, Storage: "keyring"}, nil
}

func (f *fakeAuth) RequireSession(context.Context) (auth.Credential, auth.User, error) {
	return auth.Credential{}, f.user, nil
}

func (f *fakeAuth) Cached() (auth.Result, error) {
	return auth.Result{Authenticated: f.user.ID != "", User: f.user, Storage: "keyring"}, nil
}

func (f *fakeAuth) Logout(context.Context) (auth.LogoutResult, error) {
	return auth.LogoutResult{}, nil
}

func press(m Model, code rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return next.(Model), cmd
}

func runCmd(m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		next, follow := m.Update(msg)
		return next.(Model), follow
	}
	var follow tea.Cmd
	for _, nested := range batch {
		msg = nested()
		if _, tick := msg.(spinner.TickMsg); tick {
			continue
		}
		next, nextCmd := m.Update(msg)
		m = next.(Model)
		if nextCmd != nil {
			follow = nextCmd
		}
	}
	return m, follow
}

func TestWizardRequiresPreviewAndConfirmation(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python-minimal", DisplayName: "Python"}}, nil, nil, Options{})
	defer m.cancel()
	m, _ = press(m, tea.KeyEnter)
	if m.screen != home || m.homeCategory != "Project" {
		t.Fatal("Project category did not open")
	}
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
	m, _ = runCmd(m, cmd)
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
	m, _ = runCmd(m, cmd)
	if m.screen != result || calls != 1 || len(m.result.Completed) != 1 {
		t.Fatal("generation did not complete")
	}
}

func TestWizardHelpResizeBackAndCancel(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{})
	defer m.cancel()
	m = typeCommand(m, "/help")
	m, _ = press(m, tea.KeyEnter)
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

func TestTextInputsDoNotCaptureShortcuts(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python-minimal", DisplayName: "Python"}}, nil, nil, Options{})
	defer m.cancel()

	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(Model)
	if m.screen != home || m.command.Value() != "?" {
		t.Fatal("home command input interpreted ? as a shortcut")
	}
	m.command.SetValue("")
	m.screen = form
	_ = m.setFocus(0)
	m.inputs[0].SetValue("")
	next, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(Model)
	if m.screen != form || m.inputs[0].Value() != "?" {
		t.Fatal("form input interpreted ? as a shortcut")
	}
}

func TestServerSearchAndBackNavigation(t *testing.T) {
	calls := 0
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	m.screen = serverList
	m.servers = []sshmanager.Server{
		{Alias: "production", Host: "prod.example.com", Username: "deploy"},
		{Alias: "staging", Host: "staging.example.com", Username: "ubuntu"},
	}
	if toolbar := m.toolbar(); toolbar != "" {
		t.Fatalf("idle list should hide search: %q", toolbar)
	}

	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != serverDetail || m.cursor != 1 {
		t.Fatal("server selection did not open details")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != serverList || m.cursor != 1 {
		t.Fatal("returning from details lost the selected server")
	}

	m, _ = press(m, '/')
	if !m.searching {
		t.Fatal("/ did not focus server search")
	}
	if toolbar := m.toolbar(); !strings.HasPrefix(toolbar, "> Search: ") || strings.Contains(toolbar, "Filter") {
		t.Fatalf("search field is missing: %q", toolbar)
	}
	m = typeCommand(m, "production")
	if got := m.visibleServers(); len(got) != 1 || got[0].Alias != "production" {
		t.Fatalf("server filter returned %+v", got)
	}
	m, _ = press(m, tea.KeyEscape)
	if m.searching || m.search.Value() != "" || m.screen != serverList {
		t.Fatal("Esc should clear search before leaving the list")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != home {
		t.Fatal("second Esc should leave the list")
	}

	m.screen = keyList
	m.keys = []sshmanager.Key{{Name: "laptop", Type: "ssh-ed25519", Fingerprint: "SHA256:test", PrivatePath: "/keys/laptop", Managed: true}}
	m, _ = press(m, '/')
	m = typeCommand(m, "missing")
	if len(m.visibleKeys()) != 0 || !strings.Contains(m.content(), "No keys match") {
		t.Fatal("key filter did not show a useful empty state")
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
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	homeView := m.content()
	if !strings.Contains(homeView, "Project") || !strings.Contains(homeView, "SSH") || !strings.Contains(homeView, "/help") || !strings.Contains(homeView, "/quit") || strings.Contains(homeView, "/servers") {
		t.Fatal("home commands are not grouped by category")
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.homeCategory != "SSH" || !strings.Contains(m.content(), "/servers") || strings.Contains(m.content(), "/login") {
		t.Fatal("SSH category did not reveal only SSH commands")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.homeCategory != "" || !strings.Contains(m.content(), "Account") {
		t.Fatal("Esc did not return to categories")
	}
	m.cursor = len(screens.Categories())
	m, _ = press(m, tea.KeyEnter)
	if m.screen != help {
		t.Fatal("Help is not directly selectable from Home")
	}
	m, _ = press(m, tea.KeyEscape)
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
	for _, size := range [][2]int{{18, 8}, {24, 10}, {32, 12}, {60, 12}, {80, 12}, {120, 12}, {60, 22}, {80, 24}, {120, 40}} {
		for _, s := range []screen{home, selection, form, planning, preview, confirmation, applying, result, help, about, authStarting, authWaiting, authResult, profile, logoutConfirmation, loggingOut, updatePrompt, updating, updateResult, serverLoading, serverList, serverDetail, logLoading, accessLogs, sshFailure, keyLoading, keyList, keyDetail, keyDeleteConfirmation} {
			t.Run(fmt.Sprintf("%dx%d/screen%d", size[0], size[1], s), func(t *testing.T) {
				calls := 0
				m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python", DisplayName: "Python", Description: "A minimal Python project"}}, nil, nil, Options{NoColor: true})
				defer m.cancel()
				m.screen = s
				m.plan = &scaffold.Plan{TargetDir: strings.Repeat("long-path/", 15)}
				m.result = &scaffold.Result{TargetDir: "demo", Completed: []string{"pyproject.toml"}}
				user := auth.User{ID: "user-1", GitHubID: "42", Login: "octocat"}
				m.authUser = &user
				m.authDevice = &auth.DeviceAuthorization{UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device"}
				m.authText = "Signed in."
				m.updateRelease = &updater.Release{TagName: "v0.2.0", Body: "Highlights"}
				m.servers = []sshmanager.Server{{Alias: strings.Repeat("production-", 10), Host: strings.Repeat("host.", 30) + "example.com", Port: 22, Username: strings.Repeat("deploy", 15), CreatedAt: time.Now(), UpdatedAt: time.Now()}}
				m.logs = []sshmanager.AccessLog{{ID: "log-1", ServerAlias: "production", Host: "example.com", Username: "deploy", StartedAt: time.Now(), Status: sshmanager.StatusSuccess}}
				m.sshErr = fmt.Errorf("example SSH error")
				m.keys = []sshmanager.Key{{Name: strings.Repeat("laptop-", 12), Type: "ssh-ed25519", Fingerprint: "SHA256:" + strings.Repeat("x", 80), PrivatePath: "/keys/" + strings.Repeat("long-path/", 20), PublicPath: "/keys/laptop.pub", Managed: true}}
				m.focus = 5
				m.projectContext = ProjectContext{Project: "aiai-cli", Branch: "feat/tui-ux-polish", Changes: 23}
				next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = next.(Model)
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("view is %dx%d, terminal is %dx%d", lipgloss.Width(view), lipgloss.Height(view), size[0], size[1])
				}
				if strings.Contains(view, "[38;") || strings.Contains(view, "[48;") {
					t.Fatal("color emitted with NoColor")
				}
				if s == form && size[0] >= 32 && size[1] >= 12 && !strings.Contains(view, "Continue to preview") {
					t.Fatal("focused action is hidden")
				}
			})
		}
	}
}

func TestWelcomeHeaderRestoresBrandAndSessionContext(t *testing.T) {
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{Version: "v0.4.1", NoColor: true})
	defer m.cancel()
	user := auth.User{Login: "octocat"}
	m.authUser = &user

	header := m.header()
	if !strings.Contains(header, "AIAI CLI  0.4.1") {
		t.Fatalf("welcome header is missing the product title: %q", header)
	}
	if !strings.Contains(header, "Offline templates") || !strings.Contains(header, "Signed in as @octocat") {
		t.Fatalf("welcome header is missing account status: %q", header)
	}
	if !strings.Contains(header, "Your next project starts here.") {
		t.Fatalf("welcome header lost the original tagline: %q", header)
	}
	if !strings.Contains(header, "▄██▄") || !strings.Contains(header, "▐██▌▐██▌") || !strings.Contains(header, "▄▀▀▄") {
		t.Fatalf("welcome header is missing the original ASCII logo: %q", header)
	}

	m.homeCategory = "SSH"
	categoryHeader := m.header()
	if strings.Contains(categoryHeader, "Your next project starts here.") || !strings.Contains(categoryHeader, "AIAI CLI") {
		t.Fatalf("category view should use the compact header: %q", categoryHeader)
	}
}

func TestProjectContextIsDistinctAndFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{24, 12}, {40, 12}, {40, 24}, {80, 24}, {120, 30}} {
		for _, changes := range []int{0, 1, 23} {
			m := NewModel(context.Background(), nil, nil, nil, nil, nil, Options{NoColor: true})
			defer m.cancel()
			m.width, m.height = size[0], size[1]
			m.projectContext = ProjectContext{Project: "aiai-cli", Branch: "feat/tui-ux-polish", Changes: changes}
			view := m.contextView()
			if lipgloss.Width(view) > m.theme().Width() || strings.Contains(view, "\x1b") {
				t.Fatalf("%v invalid context width or NO_COLOR: %q", size, view)
			}
			if !strings.Contains(view, "Project") || !strings.Contains(view, "├─ ") || !strings.Contains(view, "[") {
				t.Fatalf("%v context lost its labels: %q", size, view)
			}
			rows := strings.Split(view, "\n")
			if !strings.HasPrefix(rows[0], "Project ") || strings.Contains(rows[0], "├─") || !strings.HasPrefix(rows[1], "├─ ") {
				t.Fatalf("%v project and Git branch are not separated: %q", size, view)
			}
			if size[0] >= 80 && (!strings.Contains(view, "aiai-cli") || !strings.Contains(view, "feat/tui-ux-polish")) {
				t.Fatalf("%v context truncated short values: %q", size, view)
			}
			if changes == 0 && !strings.Contains(view, "[Clean]") {
				t.Fatal("clean repository is not identified")
			}
			if changes == 1 && size[0] >= 40 && !strings.Contains(view, "[1 change]") {
				t.Fatal("singular change is not identified")
			}
			if strings.ContainsAny(view, "╭╮╯╰") {
				t.Fatal("compact context has an unnecessary frame")
			}
			expected := 2
			if m.theme().Width() < 32 {
				expected = 3
			}
			if lipgloss.Height(view) != expected {
				t.Fatalf("%v status line wrapped unexpectedly: %q", size, view)
			}
			m.projectContext.Project = strings.Repeat("项目", 40)
			m.projectContext.Branch = strings.Repeat("feature/", 20)
			full := m.View().Content
			if lipgloss.Width(full) > m.width || lipgloss.Height(full) > m.height {
				t.Fatalf("%v context overflowed the screen", size)
			}
		}
	}
}

func TestResponsiveSSHPresentation(t *testing.T) {
	now := time.Now()
	server := sshmanager.Server{Alias: "production", Host: "prod.example.com", Port: 2222, Username: "deploy", IdentityFile: "/keys/deploy", Description: "Primary deployment host", UpdatedAt: now}

	wide := NewTheme(96, true)
	wideView := screens.ServerList([]sshmanager.Server{server}, 0, false, "", wide)
	if strings.Contains(wideView, "SELECTED") || !strings.Contains(wideView, "production") || !strings.Contains(wideView, "deploy@prod.example.com:2222") {
		t.Fatal("wide server browser did not use the classic single-column list")
	}

	narrow := NewTheme(36, true)
	narrowView := screens.ServerList([]sshmanager.Server{server}, 0, false, "", narrow)
	if strings.Contains(narrowView, "SELECTED") || !strings.Contains(narrowView, "production") {
		t.Fatal("narrow server browser did not collapse to a readable list")
	}

	duration := int64(1250)
	logs := screens.AccessLogs([]sshmanager.AccessLog{{ServerAlias: "production", Host: "prod.example.com", Username: "deploy", StartedAt: now, DurationMS: &duration, Status: sshmanager.StatusSuccess}}, wide)
	if !strings.Contains(logs, "STATUS") || !strings.Contains(logs, "DESTINATION") || !strings.Contains(logs, "✓ success") {
		t.Fatal("wide access logs are not rendered as a readable status table")
	}
}

func TestUnicodeTruncationUsesTerminalCells(t *testing.T) {
	got := truncate("服务器-production", 9)
	if lipgloss.Width(got) > 9 || !strings.HasSuffix(got, "…") {
		t.Fatalf("unicode truncation produced %q at width %d", got, lipgloss.Width(got))
	}
}

func TestSSHCommandPaletteFlows(t *testing.T) {
	calls := 0
	exitCode := 0
	manager := &fakeSSHManager{
		servers: []sshmanager.Server{{Alias: "production", Host: "example.com", Port: 22, Username: "deploy", CreatedAt: time.Now(), UpdatedAt: time.Now()}},
		logs:    []sshmanager.AccessLog{{ID: "log-1", ServerAlias: "production", Host: "example.com", Username: "deploy", StartedAt: time.Now(), Status: sshmanager.StatusSuccess, ExitCode: &exitCode}},
	}
	copied := ""

	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{SSH: manager, Clipboard: func(value string) error {
		copied = value
		return nil
	}})
	defer m.cancel()
	m = typeCommand(m, "/servers")
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != serverLoading || cmd == nil {
		t.Fatal("/servers did not start loading")
	}
	m, _ = runCmd(m, cmd)
	if m.screen != serverList || !strings.Contains(m.content(), "production") {
		t.Fatal("/servers did not show profiles")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != serverDetail || !strings.Contains(m.content(), "deploy") {
		t.Fatal("server details did not open")
	}
	m, _ = press(m, tea.KeyEscape)
	m, _ = press(m, tea.KeyEscape)

	m = typeCommand(m, "/ssh")
	m, cmd = press(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatalf("/ssh did not start loading, screen=%d command=%q", m.screen, m.command.Value())
	}
	m, _ = runCmd(m, cmd)
	if m.screen != serverList || !m.serverConnect {
		t.Fatalf("/ssh did not open server chooser: screen=%d connect=%v", m.screen, m.serverConnect)
	}
	m, cmd = press(m, tea.KeyEnter)
	if m.sshAlias != "production" || cmd == nil {
		t.Fatal("/ssh did not select a server and quit")
	}

	m.sshAlias, m.screen = "", home
	m = typeCommand(m, "/logs")
	m, cmd = press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	if m.screen != accessLogs || !strings.Contains(m.content(), "production") || !strings.Contains(m.content(), "success") {
		t.Fatal("/logs did not show access history")
	}
	m.screen = home
	m = typeCommand(m, "/keys laptop")
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != keyLoading || cmd == nil {
		t.Fatal("/keys query did not start key search")
	}
	m, _ = runCmd(m, cmd)
	if m.screen != keyList || !strings.Contains(m.content(), "laptop") {
		t.Fatal("/keys did not show key results")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != keyDetail || !strings.Contains(m.content(), "Copy public key") || !strings.Contains(m.content(), "Delete managed key") {
		t.Fatal("key details did not show copy and delete actions")
	}
	m, cmd = press(m, 'c')
	if cmd == nil {
		t.Fatal("copy action did not start")
	}
	m, _ = runCmd(m, cmd)
	if copied != "ssh-ed25519 AQID laptop" || !strings.Contains(m.content(), "Public key copied") {
		t.Fatalf("public key was not copied: %q", copied)
	}
	m, _ = press(m, 'd')
	if m.screen != keyDeleteConfirmation {
		t.Fatal("delete action did not request confirmation")
	}
	m, _ = press(m, tea.KeyDown)
	m, cmd = press(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("confirmed delete did not start")
	}
	m, _ = runCmd(m, cmd)
	if manager.removed != "/keys/laptop" || m.screen != keyList || !strings.Contains(m.content(), "Deleted managed key") {
		t.Fatalf("managed key was not deleted: removed=%q screen=%d", manager.removed, m.screen)
	}
	m, _ = press(m, tea.KeyEscape)
	m = typeCommand(m, "/keygen")
	m, cmd = press(m, tea.KeyEnter)
	if cmd != nil || m.screen != home || m.command.Value() != "/keygen " {
		t.Fatalf("/keygen palette selection should prompt for a name, screen=%d command=%q", m.screen, m.command.Value())
	}
	m.command.SetValue("")
	m = typeCommand(m, "/keygen deploy-key")
	m, cmd = press(m, tea.KeyEnter)
	if cmd == nil || m.action.GenerateKey == nil || m.action.GenerateKey.Name != "deploy-key" {
		t.Fatal("/keygen did not request interactive key generation")
	}
}

func TestAuthCommandPaletteFlow(t *testing.T) {
	calls := 0
	authenticator := &fakeAuth{user: auth.User{ID: "user-1", GitHubID: "42", Login: "octocat"}}
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, authenticator, nil, Options{})
	defer m.cancel()
	m = typeCommand(m, "/login")
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != authStarting || cmd == nil {
		t.Fatal("login did not start")
	}
	m, cmd = runCmd(m, cmd)
	if m.screen != authWaiting || m.authDevice == nil || m.authDevice.UserCode != "ABCD-EFGH" || cmd == nil {
		t.Fatal("device code was not shown")
	}
	next, cmd := m.Update(authPollTickMsg{})
	m = next.(Model)
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.screen != authWaiting || cmd == nil {
		t.Fatal("pending login did not schedule another poll")
	}
	next, cmd = m.Update(authPollTickMsg{})
	m = next.(Model)
	m, _ = runCmd(m, cmd)
	if m.screen != authResult || m.authUser == nil || m.authUser.Login != "octocat" {
		t.Fatal("authorized login did not update the profile")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != home || m.authUser == nil || m.authUser.Login != "octocat" {
		t.Fatal("signed-in state was not retained")
	}
	m = typeCommand(m, "/whoami")
	m, cmd = press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	if m.screen != profile || !strings.Contains(m.content(), "GitHub ID: 42") {
		t.Fatal("whoami did not show the profile")
	}
	m, _ = press(m, tea.KeyEscape)
	m = typeCommand(m, "/logout")
	m, _ = press(m, tea.KeyEnter)
	if m.screen != logoutConfirmation || m.logoutYes {
		t.Fatal("logout must default to no")
	}
	m, _ = press(m, tea.KeyDown)
	m, cmd = press(m, tea.KeyEnter)
	m, _ = runCmd(m, cmd)
	if m.screen != authResult || m.authUser != nil || !m.authOK {
		t.Fatal("logout did not clear the signed-in state")
	}
}

func TestLogoutCancellationWaitsForLocalCleanup(t *testing.T) {
	calls := 0
	authenticator := &fakeAuth{user: auth.User{ID: "user-1", GitHubID: "42", Login: "octocat"}}
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, authenticator, nil, Options{})
	defer m.cancel()
	user := authenticator.user
	m.authUser, m.screen, m.logoutYes = &user, logoutConfirmation, true
	m, logoutCmd := press(m, tea.KeyEnter)
	if m.screen != loggingOut || logoutCmd == nil {
		t.Fatal("logout did not start")
	}
	next, quitCmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if quitCmd != nil || !m.canceling {
		t.Fatal("cancellation quit before logout cleanup")
	}
	m, quitCmd = runCmd(m, logoutCmd)
	if quitCmd == nil || m.err != context.Canceled {
		t.Fatal("logout cleanup did not finish cancellation")
	}
}

func TestDryRunAndConflictNeverApply(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		calls := 0
		m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{DryRun: true})
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
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, nil, Options{})
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

type fakeTUIUpdater struct {
	checkResult *updater.CheckResult
	checkErr    error
	checkForced bool
	applyErr    error
	postponeErr error
	applied     bool
	postponed   bool
	skipped     bool
	restarted   bool
}

func (f *fakeTUIUpdater) CheckForUpdate(ctx context.Context, currentVersion string, force bool) (*updater.CheckResult, error) {
	f.checkForced = force
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	if f.checkResult != nil {
		return f.checkResult, nil
	}
	return &updater.CheckResult{CurrentVersion: currentVersion, HasUpdate: false}, nil
}

func (f *fakeTUIUpdater) ApplyUpdate(ctx context.Context, release updater.Release) error {
	f.applied = true
	return f.applyErr
}

func (f *fakeTUIUpdater) Postpone(release updater.Release, skip bool) error {
	f.postponed = true
	f.skipped = skip
	return f.postponeErr
}

func (f *fakeTUIUpdater) Restart() error {
	f.restarted = true
	return nil
}

func TestAutoUpdateFlowPromptAndPostpone(t *testing.T) {
	calls := 0
	u := &fakeTUIUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      true,
			CurrentVersion: "v0.1.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
				Version: "0.2.0",
				Body:    "Great new features",
			},
		},
	}

	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, u, Options{Version: "v0.1.0"})
	defer m.cancel()

	// Initially on home
	if m.screen != home {
		t.Fatalf("expected home screen, got %d", m.screen)
	}

	// Receive background updateCheckedMsg
	next, _ := m.Update(updateCheckedMsg{result: u.checkResult, manual: false})
	m = next.(Model)

	if m.screen != updatePrompt {
		t.Fatalf("expected updatePrompt screen, got %d", m.screen)
	}
	if m.updateRelease == nil || m.updateRelease.TagName != "v0.2.0" {
		t.Fatalf("updateRelease not set")
	}

	// Navigate to "Remind me later" (cursor = 1)
	m, _ = press(m, tea.KeyDown)
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}

	// Press Enter to postpone
	m, _ = press(m, tea.KeyEnter)
	if m.screen != home {
		t.Fatalf("expected return to home after postpone, got %d", m.screen)
	}
	if !u.postponed || u.skipped {
		t.Fatalf("expected postponed=true, skipped=false")
	}
}

func TestAutoUpdateFlowPromptAndApply(t *testing.T) {
	calls := 0
	u := &fakeTUIUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      true,
			CurrentVersion: "v0.1.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
				Version: "0.2.0",
			},
		},
	}

	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, u, Options{Version: "v0.1.0"})
	defer m.cancel()

	// Receive background updateCheckedMsg
	next, _ := m.Update(updateCheckedMsg{result: u.checkResult, manual: false})
	m = next.(Model)

	// Cursor is 0: "Update now"
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != updating || cmd == nil {
		t.Fatalf("expected updating screen and cmd, got screen=%d", m.screen)
	}

	// Execute apply cmd
	m, _ = runCmd(m, cmd)

	if !u.applied {
		t.Fatalf("expected ApplyUpdate to be executed")
	}
	if m.screen != updateResult || !m.updateResultOK {
		t.Fatalf("expected updateResult screen, got %d", m.screen)
	}

	// On updateResult: option 0 is "Restart now"
	m, quitCmd := press(m, tea.KeyEnter)
	if quitCmd == nil || !u.restarted {
		t.Fatalf("expected Restart to be called and tea.Quit")
	}
}

func TestManualUpdateCommandFromHome(t *testing.T) {
	calls := 0
	u := &fakeTUIUpdater{
		checkResult: &updater.CheckResult{
			HasUpdate:      false,
			CurrentVersion: "v0.2.0",
			LatestRelease: &updater.Release{
				TagName: "v0.2.0",
			},
		},
	}

	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, nil, nil, u, Options{Version: "v0.2.0"})
	defer m.cancel()

	m = typeCommand(m, "/update")
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != updating || cmd == nil {
		t.Fatalf("expected updating screen when running /update, got %d", m.screen)
	}

	m, _ = runCmd(m, cmd)

	if m.screen != updateResult || !m.updateResultOK {
		t.Fatalf("expected updateResult (up to date), got screen=%d", m.screen)
	}

	if u.checkForced || strings.Contains(m.content(), "Restart now") {
		t.Fatal("manual check forced an update or offered an unnecessary restart")
	}
	// Enter returns home without restarting when there is nothing to install.
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != home {
		t.Fatalf("expected return to home, got %d", m.screen)
	}
	if cmd != nil || u.restarted {
		t.Fatal("up-to-date result exited or restarted AIAI")
	}
}

func TestHomeRestoresCommandResultsAndShowsToolFailure(t *testing.T) {
	opts := Options{NoColor: true, Session: &Session{}}
	m := NewModel(context.Background(), nil, nil, nil, nil, nil, opts)
	defer m.cancel()
	m.shellResults = []shellResult{{line: "echo marker", output: strings.Repeat("marker\n", 100)}, {line: "pwd", output: "/example"}}
	m.shellSelected, m.shellFollow = 1, true
	m.saveSession()
	opts = m.opts
	opts.Notice = "Could not open Zen: damaged data"
	reopened := NewModel(context.Background(), nil, nil, nil, nil, nil, opts)
	defer reopened.cancel()
	if len(reopened.shellResults) != 2 || reopened.shellSelected != 1 || reopened.shellResults[0].output != m.shellResults[0].output {
		t.Fatal("returning Home lost command results or selection")
	}
	if !strings.Contains(reopened.View().Content, "damaged data") {
		t.Fatal("tool failure was hidden behind previous command output")
	}
	reopened, _ = press(reopened, tea.KeyF2)
	if !strings.Contains(reopened.View().Content, "echo marker") {
		t.Fatal("restored command history is inaccessible")
	}
}

func TestConfirmationLabelsUseAvailableWidth(t *testing.T) {
	for _, label := range []string{"Yes, generate files", "Yes, delete both files", "No, keep this key"} {
		if !strings.Contains(NewTheme(80, true).Choice(label, "", true), label) {
			t.Fatalf("confirmation label truncated: %s", label)
		}
	}
}

func TestPostponedUpdateShowsReasonAndReturnsHome(t *testing.T) {
	m := NewModel(context.Background(), nil, nil, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	next, _ := m.Update(updateCheckedMsg{manual: true, result: &updater.CheckResult{Notice: "Version v1.0.0 was skipped."}})
	m = next.(Model)
	view := m.View().Content
	if !strings.Contains(view, "was skipped") || strings.Contains(view, "Up to date") {
		t.Fatal("postponed check misrepresented as up to date")
	}
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != home || cmd != nil {
		t.Fatal("postponed check did not return Home")
	}
}
