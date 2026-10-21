package zen

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
	zenmodel "github.com/AIAI-Laboratory/aiai-cli/internal/zen"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen/aquarium"
)

type StartMode int

const (
	StartMenu StartMode = iota
	StartAquarium
	StartFocus
	StartStats
	StartHistory
	StartSettings
)

type Options struct {
	Mode     StartMode
	Config   zenmodel.Config
	Sessions []zenmodel.Session
	Repo     zenmodel.Repository
	NoColor  bool
	Now      func() time.Time
	Width    int
	Height   int
	Context  zenmodel.Session
	Enrich   func(context.Context, zenmodel.Session) zenmodel.Session
}

type screen int

const (
	menu screen = iota
	aquariumScreen
	focusPresetScreen
	focusCustomScreen
	focusScreen
	statsScreen
	historyScreen
	settingsScreen
	helpScreen
	exitConfirm
)

type (
	tickMsg  time.Time
	savedMsg struct {
		err     error
		session *zenmodel.Session
	}
)

type Model struct {
	ctx              context.Context
	opts             Options
	screen           screen
	previous         screen
	width            int
	height           int
	cursor           int
	scene            *aquarium.Scene
	timer            zenmodel.Timer
	stage            zenmodel.SessionType
	cycle            int
	startedAt        time.Time
	recorded         bool
	message          string
	err              error
	sessionID        string
	detailsFromFocus bool
	scrollOffset     int
}

func NewModel(ctx context.Context, opts Options) Model {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	width, height := opts.Width, opts.Height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	m := Model{ctx: ctx, opts: opts, width: width, height: height, cycle: 1, stage: zenmodel.SessionFocus}
	m.scene = aquarium.New(max(12, width-5), max(6, m.aquariumHeight()), opts.Now().UnixNano())
	switch opts.Mode {
	case StartAquarium:
		m.screen = aquariumScreen
	case StartFocus:
		m.screen = focusScreen
		m.startStage(zenmodel.SessionFocus)
	case StartStats:
		m.screen = statsScreen
	case StartHistory:
		m.screen = historyScreen
	case StartSettings:
		m.screen = settingsScreen
	default:
		m.screen = menu
	}
	return m
}

func Run(ctx context.Context, in io.Reader, out io.Writer, opts Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if stream, ok := out.(interface{ Fd() uintptr }); ok {
		if width, height, err := term.GetSize(int(stream.Fd())); err == nil {
			opts.Width, opts.Height = width, height
		}
	}
	programOptions := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx)}
	if opts.NoColor {
		programOptions = append(programOptions, tea.WithColorProfile(colorprofile.Ascii))
	}
	final, err := tea.NewProgram(NewModel(ctx, opts), programOptions...).Run()
	cancel()
	if err != nil {
		if m, ok := final.(Model); ok && m.opts.Repo != nil && (m.timer.State == zenmodel.TimerRunning || m.timer.State == zenmodel.TimerPaused) {
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer done()
			session := m.session(m.opts.Now(), zenmodel.StatusInterrupted)
			if saveErr := m.opts.Repo.AddSession(cleanup, session); saveErr != nil {
				return fmt.Errorf("%w; saving interrupted session: %v", err, saveErr)
			}
		}
		return err
	}
	return final.(Model).err
}

func (m Model) Init() tea.Cmd {
	if m.timer.State == zenmodel.TimerRunning {
		return tea.Batch(scheduleTick(m.frameDuration()), m.persist(m.session(m.opts.Now(), zenmodel.StatusRunning)))
	}
	return scheduleTick(m.frameDuration())
}

func (m Model) frameDuration() time.Duration {
	if m.opts.Config.ReducedMotion {
		return 250 * time.Millisecond
	}
	return 90 * time.Millisecond
}

func scheduleTick(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(now time.Time) tea.Msg { return tickMsg(now) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scene.Resize(max(12, m.width-5), max(6, m.aquariumHeight()))
	case tickMsg:
		m.scene.Step()
		if m.timer.State == zenmodel.TimerRunning {
			remaining := m.timer.Remaining(time.Time(msg))
			if m.timer.Uncertain() {
				m.message = "Timer paused after a long system sleep; resume when ready."
				return m, tea.Batch(scheduleTick(m.frameDuration()), m.persist(m.session(time.Time(msg), zenmodel.StatusPaused)))
			}
			if remaining == 0 {
				if m.timer.Uncertain() {
					m.message = "Timer paused after a long system sleep; resume when ready."
				} else if !m.recorded {
					return m.completeStage(time.Time(msg))
				}
			}
		}
		return m, scheduleTick(m.frameDuration())
	case savedMsg:
		if msg.err != nil {
			m.err, m.message = msg.err, "Could not save Zen data: "+msg.err.Error()
		}
		if msg.session != nil {
			session := *msg.session
			if session.ID == m.sessionID {
				session = session.WithStartFrom(m.opts.Context)
			}
			found := false
			for i, old := range m.opts.Sessions {
				if old.ID == session.ID {
					terminal := old.Status != zenmodel.StatusRunning && old.Status != zenmodel.StatusPaused
					if terminal && (session.Status == zenmodel.StatusRunning || session.Status == zenmodel.StatusPaused) || old.UpdatedAt.After(session.UpdatedAt) {
						session = old
					} else {
						m.opts.Sessions[i] = session
					}
					found = true
					break
				}
			}
			if !found {
				m.opts.Sessions = append([]zenmodel.Session{session}, m.opts.Sessions...)
			}
			if session.ID == m.sessionID {
				m.opts.Context = session
				if session.Status == zenmodel.StatusCompleted && session.Observation != "" {
					m.message = fmt.Sprintf("%s active · %s\nHEAD %.8s → %.8s", duration(session.ActiveSeconds), session.Observation, session.StartHead, session.EndHead)
				}
			}
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m Model) key(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" {
		if m.screen == exitConfirm {
			return m, nil
		}
		if m.timer.State == zenmodel.TimerRunning || m.timer.State == zenmodel.TimerPaused {
			m.previous, m.screen, m.cursor = m.screen, exitConfirm, 0
			return m, nil
		}
		return m.requestExit()
	}
	if m.screen == helpScreen {
		if m.scrollReadOnly(key) {
			return m, nil
		}
		if key == "?" || key == "esc" || key == "q" {
			m.screen = m.previous
		}
		return m, nil
	}
	if m.screen == exitConfirm {
		switch key {
		case "left", "right", "tab", "h", "l":
			m.cursor = 1 - m.cursor
		case "esc", "n":
			m.screen, m.cursor = m.previous, 0
		case "enter", "y":
			if m.cursor == 1 || key == "y" {
				return m.interruptAndQuit()
			}
			m.screen, m.cursor = m.previous, 0
		}
		return m, nil
	}
	if key == "?" {
		m.previous, m.screen = m.screen, helpScreen
		return m, nil
	}
	if key == "q" || key == "esc" {
		if m.screen == menu {
			return m, tea.Quit
		}
		return m.requestExit()
	}
	switch m.screen {
	case menu:
		items := 5
		switch key {
		case "up", "k":
			m.cursor = (m.cursor + items - 1) % items
		case "down", "j":
			m.cursor = (m.cursor + 1) % items
		case "enter":
			m.screen = []screen{aquariumScreen, focusPresetScreen, statsScreen, historyScreen, settingsScreen}[m.cursor]
			m.scrollOffset = 0
			m.detailsFromFocus = false
			m.cursor = 0
		}
	case aquariumScreen:
		m.aquariumKey(key)
	case focusPresetScreen:
		switch key {
		case "up", "down", "j", "k", "tab":
			m.cursor = 1 - m.cursor
		case "enter":
			if m.cursor == 0 {
				defaults := zenmodel.DefaultConfig()
				m.opts.Config.WorkSeconds = defaults.WorkSeconds
				m.opts.Config.BreakSeconds = defaults.BreakSeconds
				m.opts.Config.LongBreakSeconds = defaults.LongBreakSeconds
				m.opts.Config.Cycles = defaults.Cycles
				return m.beginConfiguredFocus()
			}
			m.screen, m.cursor = focusCustomScreen, 0
		}
	case focusCustomScreen:
		switch key {
		case "up", "k", "shift+tab":
			m.cursor = (m.cursor + 4) % 5
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % 5
		case "left", "h", "-":
			m.adjustFocusConfig(-1)
		case "right", "l", "+", "=":
			m.adjustFocusConfig(1)
		case "enter":
			if m.cursor < 4 {
				m.cursor++
			} else {
				return m.beginConfiguredFocus()
			}
		}
	case focusScreen:
		switch key {
		case "p":
			if m.timer.State != zenmodel.TimerRunning && m.timer.State != zenmodel.TimerPaused {
				return m, nil
			}
			switch m.timer.State {
			case zenmodel.TimerPaused:
				m.timer.Resume(m.opts.Now())
			case zenmodel.TimerRunning:
				m.timer.Pause(m.opts.Now())
			}
			status := zenmodel.StatusRunning
			if m.timer.State == zenmodel.TimerPaused {
				status = zenmodel.StatusPaused
			}
			return m, m.persist(m.session(m.opts.Now(), status))
		case "enter":
			if m.timer.State == zenmodel.TimerCompleted {
				m.startNextStage()
				return m, m.persist(m.session(m.opts.Now(), zenmodel.StatusRunning))
			}
		case "s":
			m.previous, m.screen = focusScreen, statsScreen
			m.scrollOffset = 0
			m.detailsFromFocus = true
		default:
			m.aquariumKey(key)
		}
	case statsScreen, historyScreen:
		if m.scrollReadOnly(key) {
			return m, nil
		}
		if key == "enter" {
			if m.detailsFromFocus {
				m.screen = focusScreen
			} else {
				m.screen, m.cursor = menu, 0
			}
		}
	case settingsScreen:
		switch key {
		case "up", "k":
			m.cursor = (m.cursor + 2) % 3
		case "down", "j":
			m.cursor = (m.cursor + 1) % 3
		case "enter", " ", "space":
			switch m.cursor {
			case 0:
				m.opts.Config.AutoStart = !m.opts.Config.AutoStart
			case 1:
				m.opts.Config.ReducedMotion = !m.opts.Config.ReducedMotion
			case 2:
				m.opts.Config.Unicode = !m.opts.Config.Unicode
			}
			config := m.opts.Config
			return m, func() tea.Msg {
				if m.opts.Repo == nil {
					return savedMsg{}
				}
				return savedMsg{err: m.opts.Repo.SaveConfig(m.ctx, config)}
			}
		}
	}
	return m, nil
}

func (m *Model) aquariumKey(key string) {
	switch key {
	case "f":
		m.scene.DropFood()
	case "a":
		m.scene.AddFish()
	case "d":
		m.scene.RemoveFish()
	case " ", "space":
		m.scene.Paused = !m.scene.Paused
	case "+", "=":
		m.scene.Speed = min(2.0, m.scene.Speed+0.2)
	case "-":
		m.scene.Speed = max(0.4, m.scene.Speed-0.2)
	}
}

func (m *Model) startStage(stage zenmodel.SessionType) {
	m.stage, m.recorded, m.message = stage, false, ""
	duration := time.Duration(m.opts.Config.WorkSeconds) * time.Second
	switch stage {
	case zenmodel.SessionShortBreak:
		duration = time.Duration(m.opts.Config.BreakSeconds) * time.Second
	case zenmodel.SessionLongBreak:
		duration = time.Duration(m.opts.Config.LongBreakSeconds) * time.Second
	}
	m.timer = zenmodel.NewTimer(duration)
	m.startedAt = m.opts.Now()
	m.sessionID = rand.Text()
	m.opts.Context.StartHead = ""
	m.opts.Context.EndHead = ""
	m.opts.Context.BranchAtStart = ""
	m.opts.Context.BranchAtEnd = ""
	m.opts.Context.Observation = ""
	m.opts.Context.GitSnapshotAt = time.Time{}
	m.timer.Start(m.startedAt)
}

func (m Model) beginConfiguredFocus() (tea.Model, tea.Cmd) {
	m.screen, m.cursor, m.cycle = focusScreen, 0, 1
	m.startStage(zenmodel.SessionFocus)
	config := m.opts.Config
	session := m.session(m.opts.Now(), zenmodel.StatusRunning)
	return m, func() tea.Msg {
		if m.opts.Repo != nil {
			if err := m.opts.Repo.SaveConfig(m.ctx, config); err != nil {
				return savedMsg{err: err}
			}
		}
		return m.persist(session)()
	}
}

func (m *Model) adjustFocusConfig(direction int64) {
	switch m.cursor {
	case 0:
		m.opts.Config.WorkSeconds = clampInt64(m.opts.Config.WorkSeconds+direction*5*60, 60, 4*60*60)
	case 1:
		m.opts.Config.BreakSeconds = clampInt64(m.opts.Config.BreakSeconds+direction*60, 60, 60*60)
	case 2:
		m.opts.Config.LongBreakSeconds = clampInt64(m.opts.Config.LongBreakSeconds+direction*5*60, 60, 2*60*60)
	case 3:
		m.opts.Config.Cycles = int(clampInt64(int64(m.opts.Config.Cycles)+direction, 1, 12))
	}
}

func clampInt64(value, low, high int64) int64 { return max(low, min(high, value)) }

func (m Model) completeStage(now time.Time) (tea.Model, tea.Cmd) {
	if m.screen == exitConfirm {
		m.screen = focusScreen
		m.cursor = 0
	}
	m.recorded = true
	session := m.session(now, zenmodel.StatusCompleted)
	m.timer.State = zenmodel.TimerCompleted
	if m.stage == zenmodel.SessionFocus {
		m.message = "Focus complete! Your fish are proud of you. Press Enter for your break."
	} else {
		m.message = "Break complete. Press Enter when you are ready to focus."
	}
	if m.opts.Config.AutoStart {
		m.startNextStage()
		return m, tea.Batch(scheduleTick(m.frameDuration()), tea.Sequence(m.persist(session), m.persist(m.session(now, zenmodel.StatusRunning))))
	}
	return m, tea.Batch(scheduleTick(m.frameDuration()), m.persist(session))
}

func (m *Model) startNextStage() {
	if m.stage == zenmodel.SessionFocus {
		if m.cycle%m.opts.Config.Cycles == 0 {
			m.startStage(zenmodel.SessionLongBreak)
		} else {
			m.startStage(zenmodel.SessionShortBreak)
		}
		return
	}
	if m.stage == zenmodel.SessionLongBreak {
		m.cycle = 1
	} else {
		m.cycle++
	}
	m.startStage(zenmodel.SessionFocus)
}

func (m Model) requestExit() (tea.Model, tea.Cmd) {
	if m.screen == focusCustomScreen {
		m.screen, m.cursor = focusPresetScreen, 1
		return m, nil
	}
	if m.screen == focusPresetScreen {
		m.screen, m.cursor = menu, 1
		return m, nil
	}
	if (m.screen == statsScreen || m.screen == historyScreen) && m.detailsFromFocus {
		m.screen = focusScreen
		return m, nil
	}
	if m.screen == focusScreen && (m.timer.State == zenmodel.TimerRunning || m.timer.State == zenmodel.TimerPaused) {
		m.previous, m.screen, m.cursor = m.screen, exitConfirm, 0
		return m, nil
	}
	if m.opts.Mode == StartMenu {
		m.screen, m.cursor = menu, 0
		return m, nil
	}
	return m, tea.Quit
}

func (m Model) interruptAndQuit() (tea.Model, tea.Cmd) {
	now := m.opts.Now()
	session := m.session(now, zenmodel.StatusInterrupted)
	return m, tea.Sequence(m.persist(session), tea.Quit)
}

func (m Model) session(now time.Time, status zenmodel.SessionStatus) zenmodel.Session {
	active := m.timer.Active(now)
	s := m.opts.Context
	s.ID = m.sessionID
	s.Type = m.stage
	s.StartedAt = m.startedAt.UTC()
	s.FinishedAt = time.Time{}
	if status != zenmodel.StatusRunning && status != zenmodel.StatusPaused {
		s.FinishedAt = now.UTC()
	}
	s.PlannedSeconds = int64(m.timer.Duration.Seconds())
	s.ActiveSeconds = max(0, int64(active.Seconds()))
	s.PlannedMS = m.timer.Duration.Milliseconds()
	s.ActiveMS = max(0, active.Milliseconds())
	s.Status = status
	s.CreatedAt = m.startedAt.UTC()
	s.UpdatedAt = now.UTC()
	s.OwnerPID = os.Getpid()
	return s
}

func (m Model) persist(session zenmodel.Session) tea.Cmd {
	return func() tea.Msg {
		if m.opts.Repo == nil {
			return savedMsg{session: &session}
		}
		// Save before optional Git collection: Git failures never lose timer facts.
		if err := m.opts.Repo.AddSession(m.ctx, session); err != nil {
			return savedMsg{err: err}
		}
		if m.opts.Enrich != nil {
			if session.GitSnapshotAt.IsZero() {
				_, stored, err := m.opts.Repo.Load(m.ctx)
				if err == nil {
					for _, old := range stored {
						if old.ID == session.ID {
							session = session.WithStartFrom(old)
							break
						}
					}
				}
			}
			session = m.opts.Enrich(m.ctx, session)
			if err := m.opts.Repo.AddSession(m.ctx, session); err != nil {
				return savedMsg{err: err, session: &session}
			}
		}
		return savedMsg{session: &session}
	}
}

func (m Model) aquariumHeight() int {
	reserved := 8
	if m.screen == focusScreen {
		reserved = 18
	}
	return max(6, m.height-reserved)
}

func (m Model) View() tea.View {
	if m.width < 28 || m.height < 12 {
		message := "AIAI ZEN\nTerminal too small — resize to at least 28×12\nq quit"
		if m.height < 5 {
			message = "AIAI ZEN · enlarge terminal to 28×12"
		}
		view := tea.NewView(lipgloss.NewStyle().Width(max(1, m.width)).MaxHeight(max(1, m.height)).Render(message))
		view.AltScreen = true
		return view
	}
	width, height := max(1, m.width-4), max(1, m.height-7)
	body := m.render()
	if m.screen == statsScreen || m.screen == historyScreen || m.screen == helpScreen {
		lines := strings.Split(body, "\n")
		start := min(m.scrollOffset, max(0, len(lines)-height))
		body = strings.Join(lines[start:], "\n")
	}
	content := bold("AIAI / Zen") + "\n\n" + style.Fit(body, width, height) + "\n\n" + muted(strings.Repeat("─", width), m.opts.NoColor) + "\n" + ansi.Truncate(style.Keys(m.footer(), m.opts.NoColor), width, "…")
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	view.AltScreen = true
	return view
}

func (m Model) render() string {
	var body string
	switch m.screen {
	case menu:
		body = m.menuView()
	case aquariumScreen:
		body = m.aquariumView()
	case focusPresetScreen:
		body = m.focusPresetView()
	case focusCustomScreen:
		body = m.focusCustomView()
	case focusScreen:
		body = m.focusView()
	case statsScreen:
		body = statsView(m.opts.Sessions, m.opts.Now(), m.width-5, m.opts.NoColor)
	case historyScreen:
		body = historyView(m.opts.Sessions, m.width-5, m.opts.NoColor)
	case settingsScreen:
		body = m.settingsView()
	case helpScreen:
		body = helpView(m.previous)
	case exitConfirm:
		body = bold("End this focus session?") + "\n\nActual focused time will be saved as interrupted.\n\n" + choice("Keep focusing", m.cursor == 0, m.opts.NoColor) + "  " + choice("End session", m.cursor == 1, m.opts.NoColor)
	}
	return body
}

func (m Model) menuView() string {
	items := [][2]string{{"Just Chill", "Open the aquarium"}, {"Start Focus", "Begin a Pomodoro"}, {"Focus Statistics", "Today and this week"}, {"Session History", "Your saved sessions"}, {"Settings", "Motion and behavior"}}
	var out strings.Builder
	out.WriteString(bold("Choose your pace") + "\n\n")
	for i, item := range items {
		description := ""
		if m.width >= 70 {
			description = muted("  "+item[1], m.opts.NoColor)
		}
		out.WriteString(choice(item[0], i == m.cursor, m.opts.NoColor) + description + "\n")
	}
	return out.String()
}

func (m Model) aquariumView() string {
	state := fmt.Sprintf("%d fish · %.1fx", len(m.scene.Fish), m.scene.Speed)
	if m.scene.Paused {
		state += " · paused"
	}
	return bold("Just chill") + muted("  "+state, m.opts.NoColor) + "\n\n" + m.renderSceneLimit(max(1, m.height-10))
}

func (m Model) focusPresetView() string {
	return bold("Start focus") + "\n\n" + choice("Default Pomodoro", m.cursor == 0, m.opts.NoColor) + "\n  " + muted("25m focus · 5m break · 4 cycles", m.opts.NoColor) + "\n\n" + choice("Custom", m.cursor == 1, m.opts.NoColor) + "\n  " + muted("Set durations and cycles", m.opts.NoColor)
}

func (m Model) focusCustomView() string {
	items := []struct {
		label string
		value string
	}{
		{"Focus", compactDuration(m.opts.Config.WorkSeconds)},
		{"Short break", compactDuration(m.opts.Config.BreakSeconds)},
		{"Long break", compactDuration(m.opts.Config.LongBreakSeconds)},
		{"Cycles", fmt.Sprintf("%d", m.opts.Config.Cycles)},
		{"Start focus", ""},
	}
	var out strings.Builder
	out.WriteString(bold("Custom Pomodoro") + "\n\n")
	for index, item := range items {
		label := item.label
		if item.value != "" {
			label = fmt.Sprintf("%-14s %s", item.label, item.value)
		}
		out.WriteString(choice(label, m.cursor == index, m.opts.NoColor) + "\n")
	}
	return out.String()
}

func compactDuration(seconds int64) string {
	duration := time.Duration(seconds) * time.Second
	if duration%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	if duration >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(duration.Hours()), int(duration.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(duration.Minutes()))
}

func (m Model) focusView() string {
	now := m.opts.Now()
	remaining := m.timer.Remaining(now)
	label := map[zenmodel.SessionType]string{zenmodel.SessionFocus: "FOCUS", zenmodel.SessionShortBreak: "SHORT BREAK", zenmodel.SessionLongBreak: "LONG BREAK"}[m.stage]
	seconds := int(math.Ceil(remaining.Seconds()))
	timer := fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
	progress := 1 - float64(remaining)/float64(m.timer.Duration)
	barWidth := max(10, min(30, m.width-20))
	filled := int(math.Ceil(progress * float64(barWidth)))
	filled = max(0, min(barWidth, filled))
	bar := accent(strings.Repeat("█", filled), m.opts.NoColor) + muted(strings.Repeat("░", barWidth-filled), m.opts.NoColor)
	percentage := max(0, min(100, int(math.Round(progress*100))))
	status := accent(label, m.opts.NoColor) + muted(fmt.Sprintf("  •  SESSION %d OF %d", m.cycle, m.opts.Config.Cycles), m.opts.NoColor)
	if m.opts.Context.ProjectName != "" {
		status += "\n" + ansi.Strip(m.opts.Context.ProjectName)
	}
	if m.opts.Context.BranchAtStart != "" {
		status += " · " + m.opts.Context.BranchAtStart
	}
	if m.timer.State == zenmodel.TimerPaused {
		status += accent("  •  PAUSED", m.opts.NoColor)
	}
	innerWidth := max(1, m.width-5)
	timerDisplay := lipgloss.NewStyle().Bold(true).Render(timer)
	if innerWidth >= 36 && m.height >= 24 {
		timerDisplay = accent(largeTimer(timer), m.opts.NoColor)
	}
	center := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center)
	pctLabel := fmt.Sprintf("  %3d%%", percentage)
	progressDisplay := strings.Repeat(" ", lipgloss.Width(pctLabel)) + bar + pctLabel
	body := center.Render(status) + "\n\n" + center.Render(timerDisplay) + "\n" + center.Render(progressDisplay) + "\n"
	if m.message != "" {
		body += center.Render(accent(m.message, m.opts.NoColor)) + "\n"
	}
	// Keep project context, summary and controls visible before spending the
	// remaining height on the aquarium; the timer glyphs keep their usual size.
	available := m.height - 7 - lipgloss.Height(body) + 1
	if available >= 2 {
		body += m.renderSceneLimit(available)
	}
	return body
}

func largeTimer(value string) string {
	digits := map[rune][]string{
		'0': {"┏━━━━┓", "┃    ┃", "┃    ┃", "┃    ┃", "┃    ┃", "┃    ┃", "┗━━━━┛"},
		'1': {"  ┏┓  ", "  ┛┃  ", "   ┃  ", "   ┃  ", "   ┃  ", "   ┃  ", " ━━┻━━"},
		'2': {"━━━━━┓", "     ┃", "     ┃", "┏━━━━┛", "┃     ", "┃     ", "┗━━━━━"},
		'3': {"━━━━━┓", "     ┃", "     ┃", " ━━━━┫", "     ┃", "     ┃", "━━━━━┛"},
		'4': {"┃    ┃", "┃    ┃", "┃    ┃", "┗━━━━┫", "     ┃", "     ┃", "     ┃"},
		'5': {"┏━━━━━", "┃     ", "┃     ", "┗━━━━┓", "     ┃", "     ┃", "━━━━━┛"},
		'6': {"┏━━━━━", "┃     ", "┃     ", "┣━━━━┓", "┃    ┃", "┃    ┃", "┗━━━━┛"},
		'7': {"━━━━━┓", "     ┃", "     ┃", "     ┃", "     ┃", "     ┃", "     ┃"},
		'8': {"┏━━━━┓", "┃    ┃", "┃    ┃", "┣━━━━┫", "┃    ┃", "┃    ┃", "┗━━━━┛"},
		'9': {"┏━━━━┓", "┃    ┃", "┃    ┃", "┗━━━━┫", "     ┃", "     ┃", "━━━━━┛"},
		':': {"  ", "  ", " ●", "  ", " ●", "  ", "  "},
	}
	rows := make([]strings.Builder, 7)
	for _, character := range value {
		glyph, ok := digits[character]
		if !ok {
			continue
		}
		for row := range rows {
			if rows[row].Len() > 0 {
				rows[row].WriteString(" ")
			}
			rows[row].WriteString(glyph[row])
		}
	}
	lines := make([]string, len(rows))
	for row := range rows {
		lines[row] = rows[row].String()
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSceneLimit(limit int) string {
	grid := m.scene.Render(m.opts.Config.Unicode)
	if len(grid) > limit {
		grid = grid[len(grid)-limit:]
	}
	colors := []string{"#42C5F5", "#68D391", "#F6AD55", "#B794F4", "#63B3ED", "#ECC94B", "#718096"}
	var out strings.Builder
	for y, row := range grid {
		for _, cell := range row {
			text := string(cell.Rune)
			if !m.opts.NoColor && cell.Rune != ' ' {
				text = lipgloss.NewStyle().Foreground(lipgloss.Color(colors[cell.Color%len(colors)])).Render(text)
			}
			out.WriteString(text)
		}
		if y < len(grid)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func (m Model) settingsView() string {
	items := []struct {
		label string
		on    bool
	}{{"Auto-start next stage", m.opts.Config.AutoStart}, {"Reduced motion", m.opts.Config.ReducedMotion}, {"Unicode aquarium", m.opts.Config.Unicode}}
	var out strings.Builder
	out.WriteString(bold("Settings") + "\n\n")
	for i, item := range items {
		mark := "○"
		if item.on {
			mark = "●"
		}
		out.WriteString(choice(mark+"  "+item.label, i == m.cursor, m.opts.NoColor) + "\n")
	}
	return out.String()
}

func helpView(from screen) string {
	common := "? close help · q / esc back"
	if from == focusScreen {
		return bold("FOCUS HELP") + "\n\np  Pause or resume the timer\nspace  Pause only the aquarium\nf  Feed the fish\ns  View saved statistics\n\n" + common
	}
	return bold("AQUARIUM HELP") + "\n\nf  Drop food\na / d  Add or remove fish\nspace  Pause the aquarium\n+ / -  Animation speed\n\n" + common
}

func choice(text string, selected, noColor bool) string {
	if selected {
		return accent("> "+text, noColor)
	}
	return "  " + text
}

func accent(text string, noColor bool) string {
	return style.Paint(text, style.Accent, true, noColor)
}

func muted(text string, noColor bool) string {
	return style.Paint(text, style.Muted, false, noColor)
}
func bold(text string) string { return lipgloss.NewStyle().Bold(true).Render(text) }

func (m Model) footer() string {
	if m.width < 40 {
		switch m.screen {
		case focusScreen:
			return "p pause · q end"
		case aquariumScreen:
			return "f feed · q back"
		case helpScreen:
			return "? close · q back"
		default:
			return "enter select · esc back"
		}
	}
	if m.width < 70 {
		switch m.screen {
		case menu:
			return "↑↓ choose · enter open · q quit"
		case focusScreen:
			return "p pause · f feed · q end"
		case aquariumScreen:
			return "f feed · space pause · q back"
		case focusCustomScreen:
			return "↑↓ field · ←→ time · enter · esc back"
		case statsScreen, historyScreen:
			return "↑↓ scroll · esc back"
		case helpScreen:
			return "↑↓ scroll · ? close · q back"
		default:
			return "↑↓ choose · enter · esc back"
		}
	}
	switch m.screen {
	case focusScreen:
		return "p timer · space aquarium · f feed · s stats · ? help · q end"
	case aquariumScreen:
		return "f feed · a/d fish · space pause · +/- speed · ? help · q back"
	case focusCustomScreen:
		return "↑↓ field · ←→ adjust · enter start · esc back"
	case focusPresetScreen:
		return "↑↓ choose · enter select · esc back"
	case statsScreen, historyScreen:
		return "↑↓ scroll · enter return · esc back"
	case settingsScreen:
		return "↑↓ choose · enter toggle · esc back"
	case helpScreen:
		return "↑↓ scroll · ? close · q back"
	case exitConfirm:
		return "←→ choose · enter confirm · esc back"
	default:
		return "↑↓ choose · enter open · ? help · q quit"
	}
}

func (m *Model) scrollReadOnly(key string) bool {
	delta := 0
	switch key {
	case "up", "k":
		delta = -1
	case "down", "j":
		delta = 1
	case "pgup":
		delta = -max(1, m.height-7)
	case "pgdown":
		delta = max(1, m.height-7)
	default:
		return false
	}
	limit := max(0, lipgloss.Height(m.render())-max(1, m.height-7))
	m.scrollOffset = max(0, min(limit, m.scrollOffset+delta))
	return true
}
