package zen

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	zenmodel "github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

type memoryRepo struct {
	sessions []zenmodel.Session
	config   zenmodel.Config
}

func (r *memoryRepo) Load(context.Context) (zenmodel.Config, []zenmodel.Session, error) {
	return zenmodel.DefaultConfig(), r.sessions, nil
}

func (r *memoryRepo) SaveConfig(_ context.Context, config zenmodel.Config) error {
	r.config = config
	return nil
}

func (r *memoryRepo) AddSession(_ context.Context, session zenmodel.Session) error {
	r.sessions = append(r.sessions, session)
	return nil
}

func TestFocusTimerAndAquariumPauseIndependently(t *testing.T) {
	now := time.Now()
	repo := &memoryRepo{}
	config := zenmodel.DefaultConfig()
	config.WorkSeconds = 60
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: config, Repo: repo, Now: func() time.Time { return now }})
	m.scene.Paused = false
	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = next.(Model)
	if !m.scene.Paused || m.timer.State != zenmodel.TimerRunning {
		t.Fatal("aquarium pause changed timer")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = next.(Model)
	if m.timer.State != zenmodel.TimerPaused || !m.scene.Paused {
		t.Fatal("timer pause changed aquarium")
	}
}

func TestStatisticsReturnsToActiveFocus(t *testing.T) {
	now := time.Now()
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), Repo: &memoryRepo{}, Now: func() time.Time { return now }})
	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	if m.screen != statsScreen || m.timer.State != zenmodel.TimerRunning {
		t.Fatal("statistics did not preserve focus timer")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.screen != focusScreen || cmd != nil || m.timer.State != zenmodel.TimerRunning {
		t.Fatal("statistics did not return to active focus")
	}
}

func TestSmallTerminalFallback(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: StartAquarium, Config: zenmodel.DefaultConfig(), Repo: &memoryRepo{}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m = next.(Model)
	if got := m.View().Content; !strings.Contains(got, "too small") || lipgloss.Width(got) > 20 || lipgloss.Height(got) > 8 {
		t.Fatalf("invalid fallback: %q", got)
	}
}

func TestVeryShortTerminalUsesSingleLineFallback(t *testing.T) {
	m := NewModel(context.Background(), Options{Mode: StartAquarium, Config: zenmodel.DefaultConfig(), Repo: &memoryRepo{}, Width: 80, Height: 3})
	view := m.View().Content
	if !strings.Contains(view, "enlarge terminal") || lipgloss.Height(view) != 1 {
		t.Fatalf("fallback = %q", view)
	}
}

func TestAquariumFitsResponsiveTerminals(t *testing.T) {
	for _, size := range [][2]int{{28, 12}, {40, 16}, {80, 24}, {120, 40}} {
		m := NewModel(context.Background(), Options{Mode: StartAquarium, Config: zenmodel.DefaultConfig(), Repo: &memoryRepo{}, NoColor: true})
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := next.(Model).View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("%dx%d rendered %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestFocusTimerIsLargeAndCentered(t *testing.T) {
	now := time.Now()
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), Repo: &memoryRepo{}, NoColor: true, Width: 80, Height: 24, Now: func() time.Time { return now }})
	view := m.View().Content
	if !strings.Contains(view, "━━━━━┓") || strings.ContainsAny(view, "═║╔╗╚╝") || !strings.Contains(view, "●") || !strings.Contains(view, "SESSION 1 OF 4") || !strings.Contains(view, "░") || !strings.Contains(view, "%") {
		t.Fatalf("large timer is missing: %q", view)
	}
}

func TestFocusSetupSupportsDefaultsAndCustomValues(t *testing.T) {
	repo := &memoryRepo{}
	m := NewModel(context.Background(), Options{Mode: StartMenu, Config: zenmodel.DefaultConfig(), Repo: repo})
	m.cursor = 1
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.screen != focusPresetScreen || !strings.Contains(m.View().Content, "Default Pomodoro") {
		t.Fatal("focus preset chooser did not open")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.screen != focusCustomScreen {
		t.Fatal("custom setup did not open")
	}
	before := m.opts.Config.WorkSeconds
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if m.opts.Config.WorkSeconds != before+5*60 {
		t.Fatal("custom focus duration did not change")
	}
	m.cursor = 4
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.screen != focusScreen || cmd == nil {
		t.Fatal("custom setup did not start focus")
	}
	_ = cmd()
	if repo.config.WorkSeconds != before+5*60 {
		t.Fatal("custom configuration was not saved")
	}
}

func TestFocusLifecyclePersistsStableIdentity(t *testing.T) {
	now := time.Now().UTC()
	repo, _ := zenmodel.NewJSONRepository(filepath.Join(t.TempDir(), "zen.json"))
	config := zenmodel.DefaultConfig()
	config.WorkSeconds = 10
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: config, Repo: repo, Now: func() time.Time { return now }, Context: zenmodel.Session{ProjectID: "project", ProjectName: "app"}})
	start := m.session(now, zenmodel.StatusRunning)
	msg := m.persist(start)()
	next, _ := m.Update(msg)
	m = next.(Model)
	now = now.Add(3 * time.Second)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = next.(Model)
	msg = cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	paused := m.session(now, zenmodel.StatusPaused)
	if paused.ID != start.ID || paused.ActiveMS != 3000 || paused.ProjectID != "project" {
		t.Fatalf("pause %+v", paused)
	}
	now = now.Add(time.Hour)
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = next.(Model)
	_ = cmd()
	now = now.Add(7 * time.Second)
	next, cmd = m.Update(tickMsg(now))
	m = next.(Model)
	if !m.recorded || m.timer.State != zenmodel.TimerCompleted || cmd == nil {
		t.Fatal("completion did not continue ticking")
	}
	final := m.session(now, zenmodel.StatusCompleted)
	_ = m.persist(final)()
	_, sessions, err := repo.Load(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ActiveSeconds != 10 || sessions[0].Status != zenmodel.StatusCompleted {
		t.Fatalf("completion %+v %v", sessions, err)
	}
	m.startNextStage()
	if m.stage != zenmodel.SessionShortBreak || m.sessionID == start.ID {
		t.Fatal("short break not started with fresh identity")
	}
	m.stage = zenmodel.SessionFocus
	m.cycle = config.Cycles
	m.startNextStage()
	if m.stage != zenmodel.SessionLongBreak {
		t.Fatal("long break missing")
	}
	m.startNextStage()
	if m.stage != zenmodel.SessionFocus || m.cycle != 1 {
		t.Fatal("cycle did not reset")
	}
}

func TestFocusKeepsTimeInStatisticsAndPreservesRecordBeforeEnrichment(t *testing.T) {
	now := time.Now().UTC()
	repo := &memoryRepo{}
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), Repo: repo, Now: func() time.Time { return now }})
	m.screen = statsScreen
	for range 3 {
		now = now.Add(time.Minute)
		next, _ := m.Update(tickMsg(now))
		m = next.(Model)
	}
	if m.timer.State != zenmodel.TimerRunning || m.timer.Active(now) != 3*time.Minute {
		t.Fatal("background timer stopped")
	}
	m.opts.Enrich = func(_ context.Context, s zenmodel.Session) zenmodel.Session {
		if len(repo.sessions) == 0 {
			t.Fatal("Git queried before timer record persisted")
		}
		s.Observation = "Git unavailable"
		return s
	}
	msg := m.persist(m.session(now, zenmodel.StatusInterrupted))()
	if msg.(savedMsg).err != nil || len(repo.sessions) != 2 {
		t.Fatal("record lost on Git failure")
	}
}

func TestProjectFocusKeepsControlsVisible(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 15}} {
		m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), NoColor: true, Width: size[0], Height: size[1], Context: zenmodel.Session{ProjectName: "aiai-cli"}})
		m.opts.Context.BranchAtStart = "feature/workspace"
		for _, message := range []string{"", "2m active · 0 new reachable commits observed\nHEAD abcdef01 → abcdef01"} {
			m.message = message
			view := m.View().Content
			if !strings.Contains(view, "q end") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%dx%d controls clipped: %q", size[0], size[1], view)
			}
		}
	}
}

func TestCompletedSessionCannotBecomeRunningFromDelayedSave(t *testing.T) {
	now := time.Now().UTC()
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), Now: func() time.Time { return now }})
	m.timer.State = zenmodel.TimerCompleted
	m.recorded = true
	final := m.session(now, zenmodel.StatusCompleted)
	m.opts.Sessions = []zenmodel.Session{final}
	delayed := final
	delayed.Status = zenmodel.StatusRunning
	delayed.UpdatedAt = now.Add(time.Second)
	next, _ := m.Update(savedMsg{session: &delayed})
	m = next.(Model)
	if m.opts.Sessions[0].Status != zenmodel.StatusCompleted {
		t.Fatal("delayed transition regressed completed session")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil {
		t.Fatal("p attempted to resume a completed session")
	}
}

func TestEveryZenScreenKeepsControlsVisibleAndReadable(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 20}, {40, 15}} {
		for _, s := range []screen{menu, aquariumScreen, focusPresetScreen, focusCustomScreen, focusScreen, statsScreen, historyScreen, settingsScreen, helpScreen, exitConfirm} {
			m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), NoColor: true, Width: size[0], Height: size[1]})
			m.screen = s
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%v screen %d overflow", size, s)
			}
			if !strings.Contains(view, "back") && !strings.Contains(view, "quit") && !strings.Contains(view, "end") {
				t.Fatalf("%v screen %d controls clipped: %q", size, s, view)
			}
			if strings.Contains(view, "A little peace inside") {
				t.Fatal("decorative subtitle returned")
			}
		}
	}
}

func TestStatisticsScrollsWithoutAffectingTimer(t *testing.T) {
	now := time.Now()
	m := NewModel(context.Background(), Options{Mode: StartFocus, Config: zenmodel.DefaultConfig(), NoColor: true, Width: 40, Height: 15, Now: func() time.Time { return now }})
	next, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = next.(Model)
	if m.scrollOffset == 0 || m.timer.State != zenmodel.TimerRunning {
		t.Fatal("stats could not scroll independently")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(Model).screen != focusScreen {
		t.Fatal("stats did not return to focus")
	}
}

func TestSettingsSaveFailureVisibleAndRetryClearsError(t *testing.T) {
	for _, height := range []int{12, 24} {
		m := NewModel(context.Background(), Options{Mode: StartSettings, Config: zenmodel.DefaultConfig(), NoColor: true, Width: 40, Height: height})
		m.cursor = 2
		next, _ := m.Update(savedMsg{err: errors.New("disk full")})
		m = next.(Model)
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "disk full") || !strings.Contains(view, "> ●  Unicode aquarium") && !strings.Contains(view, "> ○  Unicode aquarium") {
			t.Fatalf("%d rows hid save error or selected setting: %q", height, view)
		}
		next, _ = m.Update(savedMsg{})
		m = next.(Model)
		if m.err != nil || strings.Contains(m.View().Content, "Save failed") {
			t.Fatal("successful retry left a stale save error")
		}
	}
}

func TestSmallZenMenusKeepEverySelectedOptionVisible(t *testing.T) {
	for _, size := range [][2]int{{28, 12}, {40, 12}, {80, 12}, {80, 24}} {
		for s, labels := range map[screen][]string{
			menu:              {"Just Chill", "Start Focus", "Focus Statistics", "Session History", "Settings"},
			focusPresetScreen: {"Default Pomodoro", "Custom"},
			focusCustomScreen: {"Focus", "Short break", "Long break", "Cycles", "Start focus"},
			settingsScreen:    {"Auto-start next stage", "Reduced motion", "Unicode aquarium"},
			exitConfirm:       {"Keep focusing", "End session"},
		} {
			for cursor, label := range labels {
				m := NewModel(context.Background(), Options{Config: zenmodel.DefaultConfig(), NoColor: true, Width: size[0], Height: size[1]})
				m.screen, m.cursor = s, cursor
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(strings.Join(strings.Fields(view), " "), label) || !strings.Contains(view, "> ") || lipgloss.Height(view) > size[1] || lipgloss.Width(view) > size[0] {
					t.Fatalf("%v screen %d hid selection %q: %q", size, s, label, view)
				}
			}
		}
	}
}

func TestHistoryCanScrollToSessionsBeyondFourteen(t *testing.T) {
	for _, width := range []int{40, 80} {
		sessions := []zenmodel.Session{}
		for day := 1; day <= 20; day++ {
			sessions = append(sessions, zenmodel.Session{StartedAt: time.Date(2026, 10, day, 12, 0, 0, 0, time.Local), Type: zenmodel.SessionFocus, Status: zenmodel.StatusCompleted})
		}
		m := NewModel(context.Background(), Options{Mode: StartHistory, Sessions: sessions, Config: zenmodel.DefaultConfig(), NoColor: true, Width: width, Height: 15})
		for range 100 {
			next, _ := m.key("pgdown")
			m = next.(Model)
		}
		if view := m.View().Content; !strings.Contains(view, "2026-10-20") && !strings.Contains(view, "20 Oct") {
			t.Fatalf("%d columns hid older sessions: %q", width, view)
		}
	}
}
