package monitor

import (
	"context"
	"io"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"

	metrics "github.com/AIAI-Laboratory/aiai-cli/internal/monitor"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
)

type (
	refreshMsg   struct{ generation uint64 }
	collectedMsg struct {
		snapshot metrics.Snapshot
		err      error
	}
)

type Options struct {
	Interval      time.Duration
	NoColor       bool
	Width, Height int
}

type Model struct {
	ctx                              context.Context
	cancel                           context.CancelFunc
	collector                        metrics.Collector
	interval                         time.Duration
	noColor                          bool
	width, height                    int
	snapshot                         metrics.Snapshot
	err                              error
	paused, loading, help, searching bool
	search                           textinput.Model
	order                            string
	panel, cursor, diskOffset        int
	coreOffset                       int
	selectedPID                      int32
	iface                            string
	generation                       uint64
	cpuHistory                       []float64
	downloadHistory, uploadHistory   map[string][]float64
	sampleCancel                     context.CancelFunc
}

func NewModel(ctx context.Context, collector metrics.Collector, opts Options) Model {
	ctx, cancel := context.WithCancel(ctx)
	if opts.Interval == 0 {
		opts.Interval = time.Second
	}
	if opts.Width <= 0 {
		opts.Width = 80
	}
	if opts.Height <= 0 {
		opts.Height = 24
	}
	search := textinput.New()
	search.Prompt = ""
	search.Placeholder = "Process name or PID…"
	search.CharLimit = 80
	if opts.NoColor {
		search.SetStyles(textinput.Styles{})
	}
	style.ResizeInput(&search, min(128, opts.Width-4)-len(style.SearchPrefix)-1)
	return Model{ctx: ctx, cancel: cancel, collector: collector, interval: opts.Interval, noColor: opts.NoColor, width: opts.Width, height: opts.Height, order: "cpu", panel: 4, search: search, downloadHistory: map[string][]float64{}, uploadHistory: map[string][]float64{}}
}

func Run(ctx context.Context, in io.Reader, out io.Writer, collector metrics.Collector, opts Options) error {
	if err := metrics.ValidateInterval(opts.Interval); err != nil {
		return err
	}
	if f, ok := out.(interface{ Fd() uintptr }); ok {
		if width, height, err := term.GetSize(int(f.Fd())); err == nil {
			opts.Width, opts.Height = width, height
		}
	}
	m := NewModel(ctx, collector, opts)
	defer m.cancel()
	options := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx)}
	if opts.NoColor {
		options = append(options, tea.WithColorProfile(colorprofile.Ascii))
	}
	_, err := tea.NewProgram(m, options...).Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (m Model) Init() tea.Cmd { return func() tea.Msg { return refreshMsg{generation: m.generation} } }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		style.ResizeInput(&m.search, min(128, m.width-4)-len(style.SearchPrefix)-1)
	case refreshMsg:
		if msg.generation != m.generation || m.paused || m.loading || m.ctx.Err() != nil {
			return m, nil
		}
		return m.sample()
	case collectedMsg:
		m.loading = false
		if m.sampleCancel != nil {
			m.sampleCancel()
			m.sampleCancel = nil
		}
		if m.ctx.Err() != nil {
			return m, nil
		}
		if !m.paused {
			m.err = msg.err
			if msg.err == nil {
				m.snapshot = msg.snapshot
				m.cpuHistory = metrics.AppendHistory(m.cpuHistory, msg.snapshot.CPU.Percent)
				down, up := map[string][]float64{}, map[string][]float64{}
				for _, iface := range msg.snapshot.Networks {
					down[iface.Name] = metrics.AppendHistory(m.downloadHistory[iface.Name], iface.Download)
					up[iface.Name] = metrics.AppendHistory(m.uploadHistory[iface.Name], iface.Upload)
				}
				m.downloadHistory, m.uploadHistory = down, up
				if m.iface == "" {
					best := -1.0
					for _, iface := range msg.snapshot.Networks {
						if iface.Download != nil && iface.Upload != nil && *iface.Download+*iface.Upload > best {
							best = *iface.Download + *iface.Upload
							m.iface = iface.Name
						}
					}
				}
				m.retainSelection()
			}
			generation := m.generation
			return m, tea.Tick(m.interval, func(time.Time) tea.Msg { return refreshMsg{generation: generation} })
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m.quit()
		}
		if m.searching {
			switch key {
			case "esc":
				m.search.SetValue("")
				m.search.Blur()
				m.searching = false
			case "enter":
				m.search.Blur()
				m.searching = false
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.cursor = 0
				m.selectedPID = 0
				return m, cmd
			}
			m.cursor = 0
			m.selectedPID = 0
			return m, nil
		}
		if m.help {
			if key == "?" || key == "esc" || key == "q" {
				m.help = false
			}
			return m, nil
		}
		switch key {
		case "q", "esc":
			return m.quit()
		case "?":
			m.help = true
		case "/":
			m.searching = true
			m.panel = 4
			return m, m.search.Focus()
		case "tab":
			m.panel = (m.panel + 1) % 5
		case "c":
			m.order = "cpu"
			m.cursor = 0
			m.selectedPID = 0
		case "m":
			m.order = "memory"
			m.cursor = 0
			m.selectedPID = 0
		case "p":
			m.order = "pid"
			m.cursor = 0
			m.selectedPID = 0
		case "space", " ":
			m.paused = !m.paused
			m.generation++
			if m.paused {
				if m.sampleCancel != nil {
					m.sampleCancel()
				}
			} else if !m.loading {
				return m.sample()
			}
		case "r":
			if !m.paused && !m.loading {
				return m.sample()
			}
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "left", "h", "[":
			m.switchInterface(-1)
		case "right", "l", "]":
			m.switchInterface(1)
		}
	}
	return m, nil
}

func (m Model) sample() (tea.Model, tea.Cmd) {
	m.loading = true
	m.generation++
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	m.sampleCancel = cancel
	return m, func() tea.Msg { s, err := m.collector.Collect(ctx); return collectedMsg{snapshot: s, err: err} }
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	if m.sampleCancel != nil {
		m.sampleCancel()
	}
	return m, tea.Quit
}

func (m Model) processes() []metrics.Process {
	return metrics.Processes(m.snapshot.Processes, m.search.Value(), m.order)
}

func (m *Model) retainSelection() {
	items := m.processes()
	for i, item := range items {
		if item.PID == m.selectedPID {
			m.cursor = i
			return
		}
	}
	// No deliberate selection (or the process exited): show the highest-ranked row.
	m.cursor, m.selectedPID = 0, 0
}

func (m *Model) move(delta int) {
	if m.panel == 0 {
		m.coreOffset = max(0, min(max(0, len(m.snapshot.CPU.Cores)-6), m.coreOffset+delta*6))
		return
	}
	if m.panel == 3 {
		m.switchInterface(delta)
		return
	}
	if m.panel == 2 {
		m.diskOffset = max(0, min(len(m.snapshot.Disks)-1, m.diskOffset+delta))
		return
	}
	items := m.processes()
	if len(items) == 0 {
		return
	}
	m.cursor = (m.cursor + delta + len(items)) % len(items)
	m.selectedPID = items[m.cursor].PID
}

func (m *Model) switchInterface(delta int) {
	items := m.snapshot.Networks
	if len(items) == 0 {
		return
	}
	index := 0
	for i, item := range items {
		if item.Name == m.iface {
			index = i
		}
	}
	m.iface = items[(index+delta+len(items))%len(items)].Name
}
