package tui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

func TestTerminalTabCompletionCyclesCommandsAndPaths(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Documents", "Downloads", "My Projects", "Documents/child"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"report $(touch marker).txt", "文件.txt", ".hidden", "$cash.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	executable := "aiai-completion-test"
	if runtime.GOOS == "windows" {
		executable += ".cmd"
	}
	if err := os.WriteFile(filepath.Join(bin, executable), nil, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{Terminal: &platform.ShellSession{Dir: root}, NoColor: true})
	defer m.cancel()
	set := func(line string) {
		m.completion = nil
		m.command.SetValue(line)
		m.command.CursorEnd()
	}
	set("aiai-comp")
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != executable+" " {
		t.Fatal("Tab did not complete an executable from PATH:", m.command.Value())
	}
	set("cd Do")
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cd Do" || len(m.completion.matches) != 2 || !strings.Contains(m.completionView(5), "Downloads/") {
		t.Fatal("ambiguous completion did not offer both directories")
	}
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cd Documents/" {
		t.Fatal("Tab did not select the first match")
	}
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cd Downloads/" {
		t.Fatal("Tab did not cycle matches")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)
	if m.command.Value() != "cd Documents/" {
		t.Fatal("Shift+Tab did not cycle backward")
	}
	m, _ = press(m, tea.KeyEscape)
	if m.completion != nil || m.command.Value() != "cd Documents/" {
		t.Fatal("Escape changed input instead of closing completion")
	}
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cd Documents/child/" {
		t.Fatal("Tab did not continue completing inside a directory")
	}
	for _, line := range []string{"cd My", `cd "My Pr`} {
		set(line)
		m, _ = press(m, tea.KeyTab)
		word := shellWord(m.command.Value(), m.command.Position())
		if word.prefix != "My Projects/" {
			t.Fatalf("spaced or quoted directory was not completed safely: %s", m.command.Value())
		}
	}
	set("cat re --flag")
	m.command.SetCursor(len([]rune("cat re")))
	m, _ = press(m, tea.KeyTab)
	want := "cat " + quoteCompletion("report $(touch marker).txt", 0) + " --flag"
	if m.command.Value() != want || !strings.HasPrefix(m.command.Value()[len("cat "):], "'") {
		t.Fatal("completion lost following arguments or left shell syntax unquoted:", m.command.Value())
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatal("completion evaluated shell code")
	}
	set("cat 文")
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cat 文件.txt " || m.command.Position() != len([]rune(m.command.Value())) {
		t.Fatal("Unicode completion/cursor was incorrect")
	}
	escapedDollar := `cat \$ca`
	if runtime.GOOS == "windows" {
		escapedDollar = "cat `$ca"
	}
	set(escapedDollar)
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cat "+quoteCompletion("$cash.txt", 0)+" " {
		t.Fatal("completion expanded an escaped, literal dollar sign")
	}
	set("cd rep")
	m, _ = press(m, tea.KeyTab)
	if len(m.completion.matches) != 0 || m.command.Value() != "cd rep" {
		t.Fatal("cd completed a file instead of a directory")
	}
	set("echo hello | aiai-comp")
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "echo hello | "+executable+" " {
		t.Fatal("completion did not recognize a command after a pipe")
	}
	set("/abo")
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "/about" {
		t.Fatal("terminal completion broke AIAI slash commands")
	}
	for _, size := range [][2]int{{32, 12}, {80, 24}, {160, 40}} {
		for _, withResult := range []bool{false, true} {
			m.shellResults = nil
			if withResult {
				m.shellResults = []shellResult{{line: "pwd", dir: root, output: root}}
			}
			set("cd Do")
			m.width, m.height = size[0], size[1]
			m, _ = press(m, tea.KeyTab)
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("completion popup overflowed at %dx%d", size[0], size[1])
			}
		}
	}
}

func TestCompletionQuotingAndHomePaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	if err := os.Mkdir(filepath.Join(root, "My Projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := NewModel(context.Background(), fakePlanner{}, fakeExecutor{new(int)}, nil, nil, nil, Options{Terminal: &platform.ShellSession{Dir: root}})
	defer m.cancel()
	m.command.SetValue("cd ~/My")
	m.command.CursorEnd()
	m, _ = press(m, tea.KeyTab)
	if m.command.Value() != "cd ~/"+quoteCompletion("My Projects/", 0) {
		t.Fatal("tilde expansion was quoted away:", m.command.Value())
	}
	for _, name := range []string{"single'quote", "back\\slash", "$(echo hello)", "~other", "with spaces"} {
		for _, quote := range []rune{0, '\'', '"'} {
			encoded := "cat " + quoteCompletion(name, quote)
			if got := shellWord(encoded, len([]rune(encoded))).prefix; got != name {
				t.Fatalf("completion altered filename %q to %q via %q", name, got, encoded)
			}
		}
	}
}
