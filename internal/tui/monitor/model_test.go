package monitor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	metrics "github.com/AIAI-Laboratory/aiai-cli/internal/monitor"
)

type fakeCollector struct {
	calls    int
	snapshot metrics.Snapshot
	context  context.Context
}

func (f *fakeCollector) Collect(ctx context.Context) (metrics.Snapshot, error) {
	f.calls++
	f.context = ctx
	return f.snapshot, nil
}

func press(m Model, key rune, text string) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: text})
	return next.(Model), cmd
}

func TestSamplingPauseRefreshAndCleanup(t *testing.T) {
	fake := &fakeCollector{}
	m := NewModel(context.Background(), fake, Options{Interval: 250 * time.Millisecond})
	defer m.cancel()
	next, cmd := m.Update(m.Init()())
	m = next.(Model)
	if !m.loading || cmd == nil || fake.calls != 0 {
		t.Fatal("collect must execute asynchronously")
	}
	if _, extra := m.Update(refreshMsg{generation: m.generation}); extra != nil {
		t.Fatal("overlapping sample started")
	}
	message := cmd()
	next, tick := m.Update(message)
	m = next.(Model)
	if m.loading || tick == nil || fake.calls != 1 {
		t.Fatal("sample did not complete/schedule")
	}
	start := time.Now()
	refresh := tick().(refreshMsg)
	if time.Since(start) < 200*time.Millisecond || refresh.generation != m.generation {
		t.Fatal("refresh interval or generation was not respected")
	}
	oldGeneration := m.generation
	m, _ = press(m, ' ', " ")
	if !m.paused {
		t.Fatal("pause failed")
	}
	if _, cmd := m.Update(refreshMsg{generation: oldGeneration}); cmd != nil {
		t.Fatal("stale refresh executed")
	}
	m, cmd = press(m, 'r', "r")
	if cmd != nil {
		t.Fatal("manual refresh bypassed pause")
	}
	m, cmd = press(m, ' ', " ")
	if m.paused || cmd == nil {
		t.Fatal("resume failed")
	}
	_ = cmd()
	m, quit := press(m, 'q', "q")
	if quit == nil || m.ctx.Err() == nil || fake.context.Err() == nil {
		t.Fatal("quit did not cancel sample context")
	}
}

func TestSearchShortcutsAndNavigation(t *testing.T) {
	m := NewModel(context.Background(), &fakeCollector{}, Options{})
	defer m.cancel()
	m.snapshot.Processes = []metrics.Process{{PID: 1, Name: "chrome"}, {PID: 2, Name: "music"}}
	m, _ = press(m, '/', "/")
	for _, r := range "cmq?" {
		m, _ = press(m, r, string(r))
	}
	if m.search.Value() != "cmq?" || m.order != "cpu" || m.help || m.ctx.Err() != nil {
		t.Fatal("search interpreted a shortcut")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "> Search: cmq?") {
		t.Fatal("monitor search lost its persistent label")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.searching || m.search.Value() != "" {
		t.Fatal("escape did not clear search")
	}
	m, _ = press(m, 'j', "j")
	if m.cursor != 1 {
		t.Fatal("process navigation failed")
	}
	m, _ = press(m, 'm', "m")
	if m.order != "memory" {
		t.Fatal("memory sort failed")
	}
	m, _ = press(m, '?', "?")
	if !m.help {
		t.Fatal("help failed")
	}
}

func TestResponsiveMissingMetrics(t *testing.T) {
	for _, size := range [][2]int{{16, 5}, {32, 12}, {40, 16}, {80, 24}, {120, 40}} {
		for panel := 0; panel < 5; panel++ {
			m := NewModel(context.Background(), &fakeCollector{}, Options{NoColor: true})
			defer m.cancel()
			m.panel = panel
			m.snapshot.Processes = []metrics.Process{{PID: 1, Name: strings.Repeat("服务器", 40)}}
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(Model)
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%v panel%d rendered %dx%d", size, panel, lipgloss.Width(view), lipgloss.Height(view))
			}
			if ansi.Strip(view) != view && strings.Contains(view, "38;") {
				t.Fatal("NO_COLOR emitted color")
			}
			if size[0] >= 32 && size[1] >= 12 && !strings.Contains(view, "unavailable") {
				t.Fatal("missing metrics should be explicit")
			}
			if size[0] >= 32 && size[1] >= 12 && !strings.Contains(view, "PROCESSES") {
				t.Fatal("compact layout hid processes")
			}
		}
	}
}

func TestHistorySelectionAndNetwork(t *testing.T) {
	m := NewModel(context.Background(), &fakeCollector{}, Options{})
	defer m.cancel()
	value := 12.0
	for i := 0; i < metrics.HistoryLimit+10; i++ {
		next, _ := m.Update(collectedMsg{snapshot: metrics.Snapshot{CPU: metrics.CPU{Percent: &value}, Processes: []metrics.Process{{PID: 1, Name: "one"}, {PID: 2, Name: "two"}}, Networks: []metrics.Network{{Name: "en0", Download: &value}, {Name: "en1"}}}})
		m = next.(Model)
	}
	if len(m.cpuHistory) != metrics.HistoryLimit || len(m.downloadHistory["en0"]) != metrics.HistoryLimit {
		t.Fatal("history unbounded")
	}
	m.switchInterface(1)
	if m.iface != "en1" {
		t.Fatal("interface selection failed")
	}
	m.selectedPID = 2
	m.retainSelection()
	if m.processes()[m.cursor].PID != 2 {
		t.Fatal("refresh lost selected PID")
	}
	if got := fmt.Sprint(m.snapshot.CPU.Percent); got == "<nil>" {
		t.Fatal("snapshot not retained")
	}
}

func TestDashboardKeepsSelectedProcessVisible(t *testing.T) {
	value := 24.0
	for _, size := range [][2]int{{80, 24}, {120, 30}, {200, 30}} {
		m := NewModel(context.Background(), &fakeCollector{}, Options{Width: size[0], Height: size[1], NoColor: true})
		m.snapshot.CPU.Percent = &value
		m.snapshot.Memory = &metrics.Memory{Total: 16 << 30, Used: 8 << 30, Percent: 50}
		for range 8 {
			m.snapshot.CPU.Cores = append(m.snapshot.CPU.Cores, &value)
		}
		m.snapshot.Networks = []metrics.Network{{Name: "en0", Download: &value, Upload: &value}}
		m.snapshot.Disks = []metrics.Disk{{Mount: "/", Usage: &metrics.Memory{Total: 100 << 30, Used: 50 << 30, Percent: 50}}}
		for i := range 100 {
			m.snapshot.Processes = append(m.snapshot.Processes, metrics.Process{PID: int32(i + 1), Name: fmt.Sprintf("process-%03d", i), CPU: &value})
		}
		m.cursor = 99
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "process-099") {
			t.Fatalf("%v selected process clipped", size)
		}
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("%v overflow", size)
		}
		if size[0] >= 120 {
			for _, label := range []string{"CPU", "MEMORY", "NETWORK", "DISKS", "PROCESSES"} {
				if !strings.Contains(view, label) {
					t.Fatalf("%v missing %s", size, label)
				}
			}
		}
		m.cancel()
	}
}
