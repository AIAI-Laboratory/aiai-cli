package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestWizardRequiresPreviewAndAppliesCleanPlanDirectly(t *testing.T) {
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
	// Enter from the form input immediately advances to planning without tabbing to a button.
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != planning || calls != 0 {
		t.Fatal("wrote before planning")
	}
	m, _ = runCmd(m, cmd)
	if m.screen != preview {
		t.Fatal(m.screen)
	}
	// Clean plans apply directly on Enter from preview without a redundant confirmation screen.
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != applying || cmd == nil {
		t.Fatalf("expected direct apply on clean preview, got screen=%d", m.screen)
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
	m.screen = agentSelect
	m.editingDest = true
	m.inputs[1].SetValue("")
	_ = m.inputs[1].Focus()
	next, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(Model)
	if m.screen != agentSelect || m.inputs[1].Value() != "?" {
		t.Fatal("agentSelect destination input interpreted ? as a shortcut")
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
	if toolbar := m.toolbar(); !strings.HasPrefix(toolbar, "/ ") || strings.Contains(toolbar, "Filter") {
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
	for _, size := range [][2]int{{18, 8}, {24, 10}, {32, 12}, {60, 22}, {80, 24}, {120, 40}} {
		for _, s := range []screen{home, selection, form, agentSelect, planning, preview, applying, result, help, about, authStarting, authWaiting, authResult, profile, logoutConfirmation, loggingOut, updatePrompt, updating, updateResult, serverLoading, serverList, serverDetail, logLoading, accessLogs, sshFailure, keyLoading, keyList, keyDetail, keyDeleteConfirmation} {
			t.Run(fmt.Sprintf("%dx%d/screen%d", size[0], size[1], s), func(t *testing.T) {
				calls := 0
				m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{&calls}, []templates.TemplateMetadata{{ID: "python", DisplayName: "Python", Description: "A minimal Python project"}}, nil, nil, Options{NoColor: true})
				defer m.cancel()
				m.screen = s
				m.plan = &scaffold.Plan{
					TargetDir: strings.Repeat("long-path/", 15),
					Operations: []scaffold.Operation{
						{Path: "docs/CONTRIBUTING.md", Action: scaffold.Conflict},
					},
				}
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
	if !strings.Contains(header, "AIAI CLI  0.4.1") || !strings.Contains(header, "Your next project starts here.") {
		t.Fatalf("welcome header is missing the product title or tagline: %q", header)
	}
	if !strings.Contains(header, "Offline templates") || !strings.Contains(header, "Signed in as @octocat") {
		t.Fatalf("welcome header is missing template or account status: %q", header)
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
	applyErr    error
	postponeErr error
	applied     bool
	postponed   bool
	skipped     bool
	restarted   bool
}

func (f *fakeTUIUpdater) CheckForUpdate(ctx context.Context, currentVersion string, force bool) (*updater.CheckResult, error) {
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

	// Press enter or esc to return to home
	m, _ = press(m, tea.KeyEscape)
	if m.screen != home {
		t.Fatalf("expected return to home, got %d", m.screen)
	}
}

type capturePlanner struct {
	got project.InitRequest
}

func (c *capturePlanner) Plan(_ context.Context, req project.InitRequest) (scaffold.Plan, error) {
	c.got = req
	return scaffold.Plan{
		TemplateID:      req.TemplateID,
		TemplateVersion: "1",
		TargetDir:       req.TargetDir,
		Operations: []scaffold.Operation{
			{Path: "AGENTS.md", Action: scaffold.Append},
			{Path: ".agents/rules/docstring.md", Action: scaffold.Protected},
		},
		Warnings: []string{`Protected existing rule "docstring"`},
	}, nil
}

func TestWizardAgentSetupFlow(t *testing.T) {
	reg, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	planner := &capturePlanner{}
	calls := 0
	m := NewModel(context.Background(), planner, fakeExecutor{&calls}, reg.List(), nil, nil, Options{StartInit: true, NoColor: true})
	defer m.cancel()

	// Default first item is agent-kit; Enter jumps directly to the resource checklist.
	if m.screen != selection || !strings.Contains(m.content(), "Agent Setup (Default)") {
		t.Fatalf("expected Agent Setup pack first on selection screen:\n%s", m.content())
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != agentSelect || !m.isAgentTemplate() || !m.allAgentComponentsChecked() {
		t.Fatalf("expected direct jump to agentSelect, got screen=%d", m.screen)
	}

	// Edit destination inline via Tab, confirm with Enter -> detects existing AGENTS.md.
	m, _ = press(m, tea.KeyTab)
	if !m.editingDest {
		t.Fatal("Tab did not open inline destination editor")
	}
	m.inputs[1].SetValue(dir)
	m, _ = press(m, tea.KeyEnter)
	if m.editingDest || !m.agentsMDExists || !m.showAgentsMDModeSelector() {
		t.Fatalf("expected AGENTS.md detection after confirming inline destination")
	}

	// Toggle All off via 'a' shortcut, verify Enter blocks when empty, then restore via Space.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if m.allAgentComponentsChecked() || len(m.selectedAgentComponents()) != 0 {
		t.Fatal("'a' shortcut did not clear components")
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != agentSelect || m.formError == "" {
		t.Fatal("empty agent selection should warn instead of proceeding")
	}
	m.agentFocus = 0
	m, _ = press(m, tea.KeySpace)
	if !m.allAgentComponentsChecked() {
		t.Fatal("Space did not restore default components")
	}

	// Uncheck one item (docs, last component) with Space, keep focus there, and verify Enter still submits.
	docsSlot := len(m.agentComponents())
	m.agentFocus = docsSlot
	m, _ = press(m, tea.KeySpace)
	if m.allAgentComponentsChecked() || m.agentChecked["docs"] {
		t.Fatal("unchecking docs did not update state")
	}

	// Default AGENTS.md mode must be Append; verify cycling to Replace and back to Append.
	if m.agentsMDMode != scaffold.ExistingAppend {
		t.Fatalf("expected default append mode, got %s", m.agentsMDMode)
	}
	modeSlot := len(m.agentComponents()) + 1
	m.agentFocus = modeSlot
	m, _ = press(m, tea.KeyRight)
	if m.agentsMDMode != scaffold.ExistingReplace {
		t.Fatalf("expected replace mode after Right, got %s", m.agentsMDMode)
	}
	m, _ = press(m, tea.KeyLeft)
	if m.agentsMDMode != scaffold.ExistingAppend {
		t.Fatalf("expected append mode after Left, got %s", m.agentsMDMode)
	}

	// Pressing Enter from modeSlot or Right from a checklist row goes to Step 3 Create (review) -> Right/Enter applies cleanly.
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != planning || cmd == nil {
		t.Fatalf("expected planning screen after Enter, got %d", m.screen)
	}
	m, _ = runCmd(m, cmd)
	if m.screen != preview {
		t.Fatalf("expected preview screen, got %d", m.screen)
	}
	if !strings.Contains(m.View().Content, "1 Select Template") || !strings.Contains(m.View().Content, "3 Create") {
		t.Fatalf("expected 3-step breadcrumb in view:\n%s", m.View().Content)
	}
	previewView := m.content()
	if !strings.Contains(previewView, "++ append") || !strings.Contains(previewView, "⊘ protected") || !strings.Contains(previewView, "Review & create") {
		t.Fatalf("preview did not render unified review with append and protected badges:\n%s", previewView)
	}
	if planner.got.TemplateID != templates.AgentKitID || planner.got.AgentsMDMode != scaffold.ExistingAppend || !planner.got.ComponentsSpecified {
		t.Fatalf("unexpected InitRequest: %+v", planner.got)
	}
	// Left and Right on Step 3 Create must not apply or leave; Esc steps back to Step 2 Configure,
	// and Left/Right on a checklist row must not leave or submit; Enter advances and applies.
	m, _ = press(m, tea.KeyLeft)
	if m.screen != preview {
		t.Fatalf("expected Left on preview to stay on preview, got %d", m.screen)
	}
	m, cmd = press(m, tea.KeyRight)
	if m.screen != preview || cmd != nil {
		t.Fatalf("expected Right on preview to stay on preview without applying, got screen=%d", m.screen)
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != agentSelect {
		t.Fatalf("expected Esc on Step 3 Create to return to agentSelect, got %d", m.screen)
	}
	m.agentFocus = 0
	m, _ = press(m, tea.KeyLeft)
	if m.screen != agentSelect {
		t.Fatalf("expected Left on checklist row to stay on agentSelect, got %d", m.screen)
	}
	m, cmd = press(m, tea.KeyRight)
	if m.screen != agentSelect || cmd != nil {
		t.Fatalf("expected Right on checklist row to stay on agentSelect, got screen=%d", m.screen)
	}
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != planning || cmd == nil {
		t.Fatalf("expected planning screen after Enter from checklist row, got %d", m.screen)
	}
	m, _ = runCmd(m, cmd)
	if m.screen != preview {
		t.Fatalf("expected preview screen after re-entering Create, got %d", m.screen)
	}
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != applying || cmd == nil {
		t.Fatalf("expected direct apply on clean Create review, got %d", m.screen)
	}
	m, _ = runCmd(m, cmd)
	if m.screen != result || calls != 1 {
		t.Fatalf("expected result screen after apply, got %d (calls=%d)", m.screen, calls)
	}
}

type captureExecutor struct {
	applied *scaffold.Plan
}

func (c *captureExecutor) Apply(_ context.Context, p scaffold.Plan) (scaffold.Result, error) {
	copied := p
	c.applied = &copied
	r := scaffold.NewResult(p.TargetDir)
	for _, op := range p.Operations {
		switch op.Action {
		case scaffold.Skip:
			r.Skipped = append(r.Skipped, op.Path)
		case scaffold.Protected:
			r.Protected = append(r.Protected, op.Path)
		default:
			r.Completed = append(r.Completed, op.Path)
		}
	}
	return r, nil
}

func TestWizardConflictAndOverwriteInlineResolution(t *testing.T) {
	exec := &captureExecutor{}
	m := NewModel(context.Background(), fakePlanner{}, exec, []templates.TemplateMetadata{{ID: templates.AgentKitID, Kind: templates.KindAgent, DisplayName: "Agent Setup", Components: []templates.Component{{ID: "agents-md", Name: "AGENTS.md", Default: true}}}}, nil, nil, Options{StartInit: true, NoColor: true})
	defer m.cancel()
	m.opts.Request.TemplateID = templates.AgentKitID
	m.screen = preview
	m.plan = &scaffold.Plan{
		TemplateID:      templates.AgentKitID,
		TemplateVersion: "1",
		TargetDir:       "/tmp/repo",
		Operations: []scaffold.Operation{
			{Path: "AGENTS.md", Action: scaffold.Create, Content: []byte("agents")},
			{Path: "docs/CONTRIBUTING.md", Action: scaffold.Conflict, Content: []byte("new"), ExpectedHash: "abc"},
			{Path: ".agents/rules/docstring.md", Action: scaffold.Protected},
		},
	}

	// Conflict options render inline directly on the Step 3 Create review screen (no separate confirmation screen).
	content := m.content()
	if !strings.Contains(content, "Resolve conflicts & create") || !strings.Contains(content, "! conflict") || !strings.Contains(content, "Keep existing") {
		t.Fatalf("missing inline conflict review and choices:\n%s", content)
	}

	// Selecting option 2 (Back to configure) with Down, Down, Enter returns to Step 2 without applying.
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	if m.confirmCursor != 2 {
		t.Fatalf("expected confirmCursor=2, got %d", m.confirmCursor)
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != agentSelect || exec.applied != nil {
		t.Fatalf("expected return to agentSelect without applying, got screen=%d", m.screen)
	}

	// Back on Step 3 Create with default option 0 (Keep existing -> skip conflict), Right does not apply; Enter applies.
	m.screen, m.confirmCursor = preview, 0
	m, cmd := press(m, tea.KeyRight)
	if m.screen != preview || cmd != nil {
		t.Fatalf("expected Right on conflict preview not to apply, got screen=%d", m.screen)
	}
	m, cmd = press(m, tea.KeyEnter)
	if m.screen != applying || cmd == nil {
		t.Fatalf("expected applying after resolving conflict via Enter, got screen=%d", m.screen)
	}
	m, _ = runCmd(m, cmd)
	if m.screen != result || exec.applied == nil {
		t.Fatalf("expected result after apply, got screen=%d", m.screen)
	}
	if exec.applied.Operations[1].Action != scaffold.Skip {
		t.Fatalf("expected conflict downgraded to Skip, got %s", exec.applied.Operations[1].Action)
	}
	if exec.applied.Operations[2].Action != scaffold.Protected {
		t.Fatalf("protected rule was altered: %s", exec.applied.Operations[2].Action)
	}

	// Plan with Overwrite shows inline overwrite confirmation directly on Step 3 Create.
	exec2 := &captureExecutor{}
	m2 := NewModel(context.Background(), fakePlanner{}, exec2, nil, nil, nil, Options{NoColor: true})
	defer m2.cancel()
	m2.screen = preview
	m2.plan = &scaffold.Plan{
		TemplateID:      templates.AgentKitID,
		TemplateVersion: "1",
		TargetDir:       "/tmp/repo",
		Operations: []scaffold.Operation{
			{Path: "docs/CONTRIBUTING.md", Action: scaffold.Overwrite, Content: []byte("new"), ExpectedHash: "abc"},
		},
	}
	if !strings.Contains(m2.content(), "Confirm overwrites & create") || !strings.Contains(m2.content(), "~ replace") {
		t.Fatalf("expected inline overwrite options on preview:\n%s", m2.content())
	}
	m2, cmd2 := press(m2, tea.KeyEnter)
	if m2.screen != applying || cmd2 == nil {
		t.Fatalf("expected applying on overwrite confirm, got screen=%d", m2.screen)
	}
	m2, _ = runCmd(m2, cmd2)
	if exec2.applied == nil || exec2.applied.Operations[0].Action != scaffold.Overwrite {
		t.Fatalf("expected Overwrite applied, got %+v", exec2.applied)
	}
}

func TestStrictSemanticKeyNavigationAndSafeConfirmations(t *testing.T) {
	reg, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	planner := &capturePlanner{}
	calls := 0
	m := NewModel(context.Background(), planner, fakeExecutor{&calls}, reg.List(), nil, nil, Options{StartInit: true, NoColor: true})
	defer m.cancel()

	// Left/Right on selection must not change screen; Enter opens agentSelect; q returns to selection.
	m, _ = press(m, tea.KeyRight)
	if m.screen != selection {
		t.Fatalf("expected Right on selection to stay on selection, got %d", m.screen)
	}
	m, _ = press(m, tea.KeyLeft)
	if m.screen != selection {
		t.Fatalf("expected Left on selection to stay on selection, got %d", m.screen)
	}
	m, _ = press(m, tea.KeyEnter)
	if m.screen != agentSelect {
		t.Fatalf("expected agentSelect on Enter from selection, got %d", m.screen)
	}
	m, _ = press(m, 'q')
	if m.screen != selection {
		t.Fatalf("expected q on agentSelect to return to selection, got %d", m.screen)
	}

	// Switch to python-minimal; Enter opens form; Left/Right inside text input or on checkbox slot must stay on form.
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != form {
		t.Fatalf("expected form on Enter for python, got %d", m.screen)
	}
	m.inputs[0].SetValue("demo")
	m, _ = press(m, tea.KeyLeft)
	if m.screen != form {
		t.Fatalf("Left inside text input must stay on form, got %d", m.screen)
	}
	m.focus = 3 // checkbox slot
	m, _ = press(m, tea.KeyLeft)
	if m.screen != form {
		t.Fatalf("Left on form checkbox must stay on form, got %d", m.screen)
	}
	m, _ = press(m, tea.KeyRight)
	if m.screen != form {
		t.Fatalf("Right on form checkbox must stay on form, got %d", m.screen)
	}
	m, _ = press(m, 'q')
	if m.screen != selection {
		t.Fatalf("q on non-input form slot should return to selection, got %d", m.screen)
	}

	// Confirmation screens must ignore Left/Right so pressing Left never flips No -> Yes.
	m.screen, m.logoutYes = logoutConfirmation, false
	for _, k := range []rune{tea.KeyLeft, tea.KeyRight, 'h', 'l'} {
		m, _ = press(m, k)
		if m.logoutYes {
			t.Fatalf("key %v flipped logoutConfirmation from No to Yes", k)
		}
	}
	m, _ = press(m, 'q')
	if m.screen != home || m.logoutYes {
		t.Fatalf("q on logoutConfirmation did not cancel safely: screen=%d yes=%v", m.screen, m.logoutYes)
	}

	m.screen, m.keyDeleteYes = keyDeleteConfirmation, false
	for _, k := range []rune{tea.KeyLeft, tea.KeyRight, 'h', 'l'} {
		m, _ = press(m, k)
		if m.keyDeleteYes {
			t.Fatalf("key %v flipped keyDeleteConfirmation from No to Yes", k)
		}
	}
	m, _ = press(m, tea.KeyEscape)
	if m.screen != keyDetail || m.keyDeleteYes {
		t.Fatalf("Esc on keyDeleteConfirmation did not cancel safely: screen=%d yes=%v", m.screen, m.keyDeleteYes)
	}

	m.screen, m.updateResultOK, m.updateResultCursor = updateResult, true, 0
	for _, k := range []rune{tea.KeyLeft, tea.KeyRight, 'h', 'l'} {
		m, _ = press(m, k)
		if m.updateResultCursor != 0 {
			t.Fatalf("key %v changed updateResultCursor", k)
		}
	}
}

func TestWizardPythonWithAgentToggle(t *testing.T) {
	reg, err := templates.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	planner := &capturePlanner{}
	calls := 0
	m := NewModel(context.Background(), planner, fakeExecutor{&calls}, reg.List(), nil, nil, Options{StartInit: true, NoColor: true})
	defer m.cancel()

	// Select second item: python-minimal.
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != form || m.isAgentTemplate() || !m.includeAgent {
		t.Fatalf("expected python form with Agent Setup enabled by default")
	}
	m.inputs[0].SetValue("demo")
	// Toggle Include Agent Setup off (slot 3) with Space -> Enter from slot 3 skips agentSelect straight to planning.
	m.focus = 3
	m, _ = press(m, tea.KeySpace)
	if m.includeAgent {
		t.Fatal("failed to toggle Include Agent Setup off")
	}
	m, cmd := press(m, tea.KeyEnter)
	if m.screen != planning || cmd == nil {
		t.Fatalf("expected planning when agent setup disabled, got %d", m.screen)
	}
	m, _ = runCmd(m, cmd)
	if planner.got.IncludeAgent {
		t.Fatal("expected IncludeAgent=false when toggled off")
	}

	// Esc back to form, re-enable Agent Setup -> Enter opens agentSelect.
	m, _ = press(m, tea.KeyEscape)
	if m.screen != form {
		t.Fatalf("expected Esc to return to form, got %d", m.screen)
	}
	m.focus = 3
	m, _ = press(m, tea.KeySpace)
	m, _ = press(m, tea.KeyEnter)
	if m.screen != agentSelect {
		t.Fatalf("expected agentSelect when Include Agent Setup enabled, got %d", m.screen)
	}
}
