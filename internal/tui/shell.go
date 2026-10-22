package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

type shellFinishedMsg struct {
	err     error
	job     *shellJob
	session platform.ShellSession
	files   []shellFile
}

type shellTickMsg struct{ job *shellJob }

// Keep the most recent output bounded even for long-running build/watch commands.
const shellOutputLimit = 256 * 1024

type shellJob struct {
	mu     sync.Mutex
	output string
	cancel context.CancelFunc
}

type shellResult struct {
	line, dir, output string
	started           time.Time
	elapsed           time.Duration
	err               error
	files             []shellFile
}

func (j *shellJob) Write(data []byte) (int, error) {
	j.mu.Lock()
	j.output = tailOutput(j.output + string(data))
	j.mu.Unlock()
	return len(data), nil
}

func (j *shellJob) snapshot() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	text := ansi.Strip(strings.ToValidUTF8(j.output, "�"))
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return -1
		}
		return r
	}, text)
}

func tailOutput(text string) string {
	if len(text) > shellOutputLimit {
		return text[len(text)-shellOutputLimit:]
	}
	return text
}

func pollShell(job *shellJob) tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return shellTickMsg{job: job} })
}

func (m Model) showingShellOutput() bool {
	return m.screen == home && m.homeCategory == "" && len(m.shellResults) > 0 &&
		(m.shellHistory || m.command.Value() == "" || shellInput(m.command.Value()))
}

func (m Model) scrollShell(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.resize()
	m.viewport.SetContent(m.content())
	if m.shellFollow {
		m.viewport.GotoBottom()
	}
	var cmd tea.Cmd
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+home" {
		m.viewport.GotoTop()
	} else if ok && key.String() == "ctrl+end" {
		m.viewport.GotoBottom()
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	m.shellFollow = m.viewport.AtBottom()
	return m, cmd
}

func shellInput(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if !strings.HasPrefix(line, "/") {
		return true
	}
	return strings.Contains(strings.Fields(line)[0][1:], "/")
}

func (m Model) runShell(native bool) (tea.Model, tea.Cmd) {
	line := m.command.Value()
	if strings.ContainsRune(line, 0) {
		m.shellNotice = "Command cannot contain NUL."
		return m, nil
	}
	history := m.opts.Terminal.History
	if len(history) == 0 || history[len(history)-1] != line {
		history = append(history, line)
		if len(history) > 100 {
			history = history[len(history)-100:]
		}
		m.opts.Terminal.History = history
	}
	m.historyCursor, m.historyDraft = len(history), ""
	m.command.SetValue("")
	m.shellNotice = ""
	m.homeCategory = ""
	if native {
		process := &platform.ShellCommand{Context: m.ctx, Session: m.opts.Terminal, Line: line}
		return m, tea.Exec(process, func(err error) tea.Msg { return shellFinishedMsg{err: err} })
	}
	ctx, cancel := context.WithCancel(m.ctx)
	job := &shellJob{cancel: cancel}
	m.shellJob, m.shellFollow = job, true
	m.shellResults = append(m.shellResults, shellResult{line: line, dir: m.opts.Terminal.Dir, started: time.Now()})
	m.trimShellResults()
	m.shellSelected, m.shellHistory = len(m.shellResults)-1, false
	session := *m.opts.Terminal
	return m, tea.Batch(pollShell(job), func() tea.Msg {
		defer cancel()
		process := &platform.ShellCommand{Context: ctx, Session: &session, Line: line, Capture: true, Out: job, Err: job}
		err := process.Run()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		var files []shellFile
		if err == nil {
			files = shellListing(line, session.Dir, job.snapshot())
		}
		return shellFinishedMsg{err: err, job: job, session: session, files: files}
	})
}

func (m Model) recallCommand(delta int) Model {
	history := m.opts.Terminal.History
	if len(history) == 0 {
		return m
	}
	if m.historyCursor == len(history) {
		m.historyDraft = m.command.Value()
	}
	m.historyCursor = max(0, min(len(history), m.historyCursor+delta))
	value := m.historyDraft
	if m.historyCursor < len(history) {
		value = history[m.historyCursor]
	}
	m.command.SetValue(value)
	m.command.CursorEnd()
	return m
}

func (m Model) shellFinished(msg shellFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.job != nil {
		if m.shellJob != msg.job {
			return m, nil
		}
		result := &m.shellResults[len(m.shellResults)-1]
		result.output, result.files = msg.job.snapshot(), msg.files
		result.elapsed, result.err = time.Since(result.started), msg.err
		m.shellJob = nil
		m.trimShellResults()
		m.opts.Terminal.Dir, m.opts.Terminal.PreviousDir = msg.session.Dir, msg.session.PreviousDir
	}
	if err := os.Chdir(m.opts.Terminal.Dir); err != nil {
		m.shellNotice = fmt.Sprintf("Cannot use directory: %v", err)
	} else if msg.err != nil && msg.job == nil {
		m.shellNotice = fmt.Sprintf("Command failed: %v", msg.err)
	} else {
		m.shellNotice = ""
	}
	if dir, err := os.Getwd(); err == nil {
		m.opts.Terminal.Dir = dir
	}
	m.projectContext = ""
	if m.opts.Context != nil {
		return m, func() tea.Msg { return contextMsg(m.opts.Context(m.ctx)) }
	}
	return m, nil
}

func (m *Model) trimShellResults() {
	bytes := 0
	for i := len(m.shellResults) - 1; i >= 0; i-- {
		bytes += len(m.shellResults[i].output) + len(m.shellResults[i].line)
		if i < len(m.shellResults)-1 && (bytes > shellOutputLimit || len(m.shellResults)-i > 20) {
			m.shellResults = append([]shellResult(nil), m.shellResults[i+1:]...)
			m.shellSelected = max(0, m.shellSelected-i-1)
			return
		}
	}
}

func (m Model) shellKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.homeCategory = ""
	if strings.HasPrefix(m.command.Value(), "/") && !shellInput(m.command.Value()) {
		m.command.SetValue("")
	}
	switch msg.String() {
	case "f2":
		m.shellHistory = !m.shellHistory
	case "alt+left":
		m.shellSelected = max(0, m.shellSelected-1)
	case "alt+right":
		m.shellSelected = min(len(m.shellResults)-1, m.shellSelected+1)
	case "up":
		m.shellSelected = max(0, m.shellSelected-1)
	case "down":
		m.shellSelected = min(len(m.shellResults)-1, m.shellSelected+1)
	case "enter", "esc":
		m.shellHistory = false
	default:
		return m, nil
	}
	m.shellFollow = false
	m.viewport.GotoTop()
	return m, nil
}

func (r shellResult) status(t Theme, running bool) string {
	if running {
		return t.Accent("● Running")
	}
	status := t.Good("✓ Done")
	if errors.Is(r.err, context.Canceled) {
		status = t.Warn("■ Stopped")
	} else if r.err != nil {
		status = t.Accent("✕ " + r.err.Error())
	}
	return status + t.Muted(fmt.Sprintf(" · %.2fs", r.elapsed.Seconds()))
}
