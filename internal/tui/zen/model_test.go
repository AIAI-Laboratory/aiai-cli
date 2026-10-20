package zen

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
