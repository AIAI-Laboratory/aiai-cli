package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

func TestHomeExecutesTerminalCommandsAndKeepsSlashMenus(t *testing.T) {
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	for _, line := range []string{"ls -la", "cd 'folder with spaces'", "npm run build", "git status", "/usr/bin/printf hello", "./script.sh", "aiai version --json"} {
		m.command.SetValue(line)
		next, cmd := press(m, tea.KeyEnter)
		if cmd == nil || next.screen != home || next.action.Git || next.action.GenerateKey != nil || next.command.Value() != "" {
			t.Fatalf("terminal command was treated as a menu: %q", line)
		}
		m = next
		m.shellJob.cancel()
		m.shellJob = nil
	}
	if len(m.opts.Terminal.History) != 7 {
		t.Fatal("terminal commands not recorded in history")
	}
	m.command.SetValue("/git")
	m, cmd := press(m, tea.KeyEnter)
	if cmd == nil || !m.action.Git {
		t.Fatal("slash command no longer opens the Git interface")
	}
}

func TestShellHistoryRestoresDraftAndSurvivesHomeReopen(t *testing.T) {
	session := &platform.ShellSession{History: []string{"ls", "git status"}}
	opts := Options{Terminal: session}
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, opts)
	defer m.cancel()
	m.command.SetValue("npm run ")
	m, _ = press(m, tea.KeyUp)
	if m.command.Value() != "git status" {
		t.Fatal("Up did not recall latest command")
	}
	m, _ = press(m, tea.KeyUp)
	if m.command.Value() != "ls" {
		t.Fatal("Up did not recall previous command")
	}
	m, _ = press(m, tea.KeyDown)
	m, _ = press(m, tea.KeyDown)
	if m.command.Value() != "npm run " {
		t.Fatal("Down lost the unfinished command")
	}
	m.command.SetValue("npm run build")
	m, _ = press(m, tea.KeyEnter)
	defer m.shellJob.cancel()
	reopened := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, opts)
	defer reopened.cancel()
	reopened, _ = press(reopened, tea.KeyUp)
	if reopened.command.Value() != "npm run build" {
		t.Fatal("returning from another AIAI screen lost history")
	}
	reopened.command.SetValue("/git")
	reopened, _ = press(reopened, tea.KeyUp)
	if reopened.command.Value() != "/git" {
		t.Fatal("menu navigation unexpectedly recalled shell history")
	}
	reopened.command.SetValue("/")
	reopened.cursor = 0
	reopened, _ = press(reopened, tea.KeyDown)
	reopened, _ = press(reopened, tea.KeyEnter)
	if reopened.homeCategory != "SSH" || reopened.command.Value() != "" {
		t.Fatal("slash menus became inaccessible after shell history was recorded")
	}
}

func TestInlineShellOutputStreamsScrollsAndCancelsWithoutExiting(t *testing.T) {
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	m.command.SetValue("npm run build")
	m, cmd := press(m, tea.KeyEnter)
	if cmd == nil || m.shellJob == nil || !m.showingShellOutput() {
		t.Fatal("Enter did not start inline output")
	}
	job := m.shellJob
	canceled := false
	cancel := job.cancel
	job.cancel = func() { canceled = true; cancel() }
	_, _ = job.Write([]byte(strings.Repeat("build log\n", 80) + "\x1b[31mready\x1b[0m\n"))
	next, _ := m.Update(shellTickMsg{job: job})
	m = next.(Model)
	if !strings.Contains(m.content(), "ready") || strings.Contains(m.content(), "\x1b") {
		t.Fatal("live stdout/stderr was not rendered as safe text")
	}
	m, _ = press(m, tea.KeyPgUp)
	if m.shellFollow || m.viewport.AtBottom() {
		t.Fatal("output did not scroll away from the live tail")
	}
	m, _ = press(m, tea.KeyPgDown)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if !canceled || m.ctx.Err() != nil {
		t.Fatal("Ctrl+C exited AIAI instead of stopping the command")
	}
	next, _ = m.Update(shellFinishedMsg{job: job, session: *m.opts.Terminal, err: context.Canceled})
	m = next.(Model)
	if m.shellJob != nil || !strings.Contains(m.content(), "ready") || !strings.Contains(m.shellHeader(), "Stopped") {
		t.Fatal("completion lost the command result")
	}
	m.command.SetValue("ls")
	if !strings.Contains(m.content(), "ready") {
		t.Fatal("typing the next command hid the previous result")
	}
	m.command.SetValue("/")
	if m.showingShellOutput() || !strings.Contains(m.content(), "Project") {
		t.Fatal("output blocked AIAI menus")
	}
}

func TestAltEnterKeepsInteractiveTerminalAndOutputBufferIsBounded(t *testing.T) {
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{})
	defer m.cancel()
	m.command.SetValue("ssh server")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	m = next.(Model)
	if cmd == nil || m.shellJob != nil || m.command.Value() != "" {
		t.Fatal("Alt+Enter did not preserve native terminal execution")
	}
	job := &shellJob{}
	_, _ = job.Write([]byte(strings.Repeat("x", shellOutputLimit) + "\nlatest output\n"))
	if len(job.output) != shellOutputLimit || !strings.HasSuffix(job.snapshot(), "latest output\n") {
		t.Fatal("output buffer did not retain a bounded tail")
	}
}

func TestShellCompletionRefreshesAIAIWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	destination := t.TempDir()
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{NoColor: true})
	defer m.cancel()
	m.opts.Terminal.Dir = destination
	next, _ := m.Update(shellFinishedMsg{})
	m = next.(Model)
	wd, err := os.Getwd()
	canonical, _ := filepath.EvalSymlinks(destination)
	if err != nil || wd != canonical || m.opts.Terminal.Dir != canonical {
		t.Fatal("AIAI views did not follow shell working directory")
	}
	for _, line := range []string{"j", "k"} {
		m.command.SetValue("")
		m = typeCommand(m, line)
		if m.command.Value() != line {
			t.Fatal("navigation shortcut swallowed terminal input")
		}
	}
}
