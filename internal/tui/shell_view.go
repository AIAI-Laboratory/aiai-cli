package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
)

type shellFile struct {
	name string
	dir  bool
	link bool
}

// Only format an unambiguous, plain directory listing. Other commands keep
// their original output, including ls flags, pipelines and redirected data.
func shellListing(line, dir, output string) []shellFile {
	switch strings.TrimSpace(line) {
	case "ls", "ls -a", "ls -A":
	default:
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil || output == "" {
		return nil
	}
	known := map[string]shellFile{".": {name: ".", dir: true}, "..": {name: "..", dir: true}}
	for _, entry := range entries {
		known[entry.Name()] = shellFile{name: entry.Name(), dir: entry.IsDir(), link: entry.Type()&os.ModeSymlink != 0}
	}
	var files []shellFile
	for _, name := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		file, ok := known[name]
		if !ok {
			return nil
		}
		files = append(files, file)
	}
	return files
}

func shellPath(dir string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if dir == home {
			return "~"
		}
		if strings.HasPrefix(dir, home+string(filepath.Separator)) {
			return "~" + strings.TrimPrefix(dir, home)
		}
	}
	return dir
}

func shellRow(left, right string, t Theme) string {
	right = t.Truncate(right, max(1, t.Width()/2))
	left = t.Truncate(left, max(1, t.Width()-lipgloss.Width(right)-2))
	return left + strings.Repeat(" ", max(1, t.Width()-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

func (m Model) shellHeader() string {
	t := m.theme()
	count := len(m.shellResults)
	project := m.projectContext.Project
	if project == "" {
		project = filepath.Base(m.opts.Terminal.Dir)
	}
	dir := projectLabel(shellPath(m.opts.Terminal.Dir))
	dirWidth := max(1, t.Width()-len("Dir: "))
	if ansi.StringWidth(dir) > dirWidth {
		dir = ansi.TruncateLeft(dir, ansi.StringWidth(dir)-dirWidth+1, "…")
	}
	projectLine := t.Truncate(t.Title("Current: "+projectLabel(project)), t.Width())
	if !m.shellHistory {
		historyKey := "F2 history"
		if t.Width() < 48 {
			historyKey = "F2"
		}
		projectLine = shellRow(projectLine, t.Muted(historyKey), t)
	}
	current := projectLine + "\n" + t.Muted("Dir: "+dir)
	if m.shellHistory {
		return current + "\n" + shellRow(t.Title("Command history"), t.Muted("F2 / Esc back"), t) + "\n" + t.Rule()
	}
	r := m.shellResults[m.shellSelected]
	running := m.shellJob != nil && m.shellSelected == count-1
	command := strings.Join(strings.Fields(r.line), " ")
	return current + "\n" +
		t.Rule() + "\n" +
		shellRow(t.Accent("❯ ")+t.Title(command), r.status(t, running), t) + "\n" +
		shellRow(t.Muted("Ran in: "+shellPath(r.dir)), t.Muted(fmt.Sprintf("Run %d / %d", m.shellSelected+1, count)), t) + "\n" + t.Rule()
}

func (m Model) shellContent() string {
	t := m.theme()
	if m.shellHistory {
		var rows []string
		for i, r := range m.shellResults {
			prefix := "  "
			command := strings.Join(strings.Fields(r.line), " ")
			if i == m.shellSelected {
				prefix, command = "> ", t.Accent(command)
			}
			rows = append(rows, shellRow(fmt.Sprintf("%s%02d  %s", prefix, i+1, command), r.status(t, m.shellJob != nil && i == len(m.shellResults)-1), t),
				t.Muted(t.Truncate("  Ran in: "+shellPath(r.dir), t.Width())), "")
		}
		return strings.Join(rows, "\n")
	}
	r := m.shellResults[m.shellSelected]
	output := strings.TrimRight(r.output, "\n")
	if len(r.files) > 0 {
		output = shellFileGrid(r.files, t.Width()-2, t)
	} else if output == "" {
		output = t.Muted("No output")
		if m.shellJob != nil && m.shellSelected == len(m.shellResults)-1 {
			output = t.Muted("Waiting for output…")
		}
	}
	lines := strings.Split(ansi.Hardwrap(output, max(1, t.Width()-2), true), "\n")
	for i := range lines {
		lines[i] = t.Muted("│") + " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func shellFileGrid(files []shellFile, width int, t Theme) string {
	width = max(1, width)
	cell := min(width, 20)
	labels := make([]string, len(files))
	for i, file := range files {
		label := file.name
		if file.dir {
			label += "/"
		} else if file.link {
			label += " ↗"
		}
		if size := lipgloss.Width(label) + 3; size <= min(36, width) {
			cell = max(cell, size)
		}
		if file.dir {
			label = style.Paint(label, style.Info, true, t.NoColor())
		} else if file.link {
			label = t.Accent(label)
		} else if strings.HasPrefix(file.name, ".") {
			label = t.Muted(label)
		}
		labels[i] = label
	}
	columns := max(1, width/cell)
	cell = width / columns
	var rows []string
	var row string
	used := 0
	flush := func() {
		if used > 0 {
			rows = append(rows, strings.TrimRight(row, " "))
			row, used = "", 0
		}
	}
	for _, label := range labels {
		if lipgloss.Width(label)+2 > cell {
			flush()
			rows = append(rows, ansi.Hardwrap(label, width, true))
			continue
		}
		row += label + strings.Repeat(" ", max(0, cell-lipgloss.Width(label)))
		used++
		if used == columns {
			flush()
		}
	}
	flush()
	return strings.Join(rows, "\n")
}
