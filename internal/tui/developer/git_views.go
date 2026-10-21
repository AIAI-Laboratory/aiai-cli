package developer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
)

func (m Model) tone(text, color string) string {
	return style.Paint(text, color, false, m.opts.NoColor)
}
func (m Model) muted(text string) string { return m.tone(text, style.Muted) }
func (m Model) title(text string) string { return style.Paint(text, "", true, m.opts.NoColor) }
func oneLine(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(clean(text), "\n", "↵"), "\t", " ")
}

func column(text string, width int) string {
	text = ansi.Truncate(oneLine(text), max(0, width), "…")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func pathLabel(path string) string {
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return strconv.QuoteToGraphic(path)
	}
	return path
}

func pathTail(path string, width int) string {
	path = pathLabel(path)
	if ansi.StringWidth(path) > width {
		path = ansi.TruncateLeft(path, ansi.StringWidth(path)-width+1, "…")
	}
	return ansi.Truncate(path, max(0, width), "")
}

func changeLabel(c gitlocal.Change) string {
	switch {
	case c.Conflict:
		return "Conflict"
	case c.Untracked:
		return "New"
	case strings.Contains(c.Index+c.Worktree, "R"):
		return "Renamed"
	case strings.Contains(c.Index+c.Worktree, "D"):
		return "Deleted"
	case strings.Contains(c.Index+c.Worktree, "A"):
		return "Added"
	case strings.Contains(c.Index+c.Worktree, "T"):
		return "Type"
	default:
		return "Modified"
	}
}

func (m Model) paneTitle(text string, active bool) string {
	if active {
		return m.accent("> " + text)
	}
	return m.muted("  " + text)
}

// Diff framing belongs to the view, not the source: only pre-hunk Git headers
// are hidden. Content resembling a header inside a hunk must remain visible.
func diffContent(diff string) ([]string, int, int) {
	lines := []string{}
	added, removed := 0, 0
	inHunk := false
	for _, line := range strings.Split(strings.TrimSuffix(clean(diff), "\n"), "\n") {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
		}
		if !inHunk && (strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")) {
			continue
		}
		if inHunk {
			if strings.HasPrefix(line, "+") {
				added++
			}
			if strings.HasPrefix(line, "-") {
				removed++
			}
		}
		lines = append(lines, strings.ReplaceAll(line, "\t", "    "))
	}
	return lines, added, removed
}

func (m Model) diffView(width, height int) string {
	indices := m.visible()
	if len(indices) == 0 {
		return "Working tree clean, or no files match your search."
	}
	cursor := min(m.cursor, len(indices)-1)
	selected := indices[cursor]
	if selected >= len(m.status.Changes) {
		return "No file selected."
	}
	split := width >= 90
	leftWidth, rightWidth := width, width
	if split {
		leftWidth = min(42, max(28, width/3))
		rightWidth = width - leftWidth - 3
	}
	capacity := max(1, height-3)
	scope := "Unstaged"
	if m.staged {
		scope = "Staged"
	}
	files := []string{m.paneTitle(fmt.Sprintf("FILES  %d / %d", cursor+1, len(indices)), !m.preview), m.muted(fmt.Sprintf("  %s · %d staged", scope, m.status.Staged)), m.muted(strings.Repeat("─", leftWidth))}
	start := max(0, cursor-capacity+1)
	for visible := start; visible < min(len(indices), start+capacity); visible++ {
		index := indices[visible]
		if index >= len(m.status.Changes) {
			continue
		}
		change := m.status.Changes[index]
		label := changeLabel(change)
		name := pathTail(change.Path, max(1, leftWidth-16))
		checkbox := "[ ] "
		if change.Index != "" && change.Index != "." && change.Index != "?" {
			checkbox = "[x] "
			if change.Worktree != "" && change.Worktree != "." {
				checkbox = "[-] "
			}
		}
		if change.Conflict {
			checkbox = "[!] "
		}
		marker := "  "
		if visible == cursor {
			marker = m.accent("> ")
			name = m.accent(name)
		} else {
			name = clean(name)
		}
		status := m.muted(column(label, 9))
		if label == "Conflict" || label == "Deleted" {
			status = m.tone(column(label, 9), style.Accent)
		}
		if label == "New" || label == "Added" {
			status = m.tone(column(label, 9), style.Good)
		}
		files = append(files, marker+m.muted(checkbox)+status+" "+name)
	}
	diff := m.diff
	if diff == "" {
		diff = "No " + strings.ToLower(scope) + " changes for this file. Press s to switch."
	}
	lines, added, removed := diffContent(diff)
	if len(lines) == 0 {
		lines = []string{"No textual changes in this view."}
	}
	offset := min(m.offset, max(0, len(lines)-capacity))
	end := min(len(lines), offset+capacity)
	title := fmt.Sprintf("DIFF  +%d −%d · %d–%d/%d", added, removed, offset+1, end, len(lines))
	preview := []string{ansi.Truncate(m.paneTitle(title, m.preview), rightWidth, "…"), m.muted(pathTail(m.status.Changes[selected].Path, rightWidth)), m.muted(strings.Repeat("─", rightWidth))}
	for _, line := range lines[offset:end] {
		line = ansi.Truncate(line, rightWidth, "…")
		switch {
		case strings.HasPrefix(line, "+"):
			line = m.tone(line, style.Good)
		case strings.HasPrefix(line, "-"):
			line = m.tone(line, style.Accent)
		case strings.HasPrefix(line, "@@"):
			line = m.tone(line, style.Info)
		}
		preview = append(preview, line)
	}
	if !split {
		if m.preview {
			return strings.Join(preview, "\n")
		}
		return strings.Join(files, "\n")
	}
	// Join row by row instead of relying on wrapping styles, so a long path or
	// diff line cannot shift the other pane or push its footer out of view.
	rows := []string{}
	for i := 0; i < min(height, max(len(files), len(preview))); i++ {
		left, right := "", ""
		if i < len(files) {
			left = files[i]
		}
		if i < len(preview) {
			right = preview[i]
		}
		left = ansi.Truncate(left, leftWidth, "…")
		left += strings.Repeat(" ", max(0, leftWidth-ansi.StringWidth(left)))
		rows = append(rows, left+m.muted(" │ ")+right)
	}
	return strings.Join(rows, "\n")
}

func (m Model) historyView(width, height int) string {
	if len(m.commits) == 0 {
		return m.scrollRows(m.rows, width, height)
	}
	indices := m.visible()
	if len(indices) == 0 {
		return "No commits match your search."
	}
	heading := m.muted(fmt.Sprintf("%d commits · all authors", len(indices)))
	rows := []string{}
	wide := width >= 90
	dateWidth, authorWidth := 11, 12
	if width >= 100 {
		dateWidth, authorWidth = 16, 16
	}
	messageWidth := width - dateWidth - authorWidth - 8 - 6
	header := ""
	if wide {
		header = m.muted(column("DATE / TIME", dateWidth) + "  " + column("COMMIT", 8) + "  " + column("MESSAGE", messageWidth) + "  " + column("AUTHOR", authorWidth))
	}
	previousDay := ""
	for _, index := range indices {
		if index >= len(m.commits) {
			continue
		}
		commit := m.commits[index]
		local := commit.Date.Local()
		day := local.Format("2006-01-02")
		if len(rows) > 0 && (day != previousDay || !wide) {
			rows = append(rows, "")
		}
		previousDay = day
		hash := commit.Hash[:min(8, len(commit.Hash))]
		if wide {
			date := local.Format("01-02 15:04")
			if dateWidth == 16 {
				date = local.Format("2006-01-02 15:04")
			}
			rows = append(rows, m.muted(column(date, dateWidth))+"  "+m.tone(column(hash, 8), style.Info)+"  "+column(commit.Subject, messageWidth)+"  "+m.muted(column(commit.Author, authorWidth)))
		} else {
			rows = append(rows, ansi.Truncate(oneLine(commit.Subject), width, "…"), m.muted(ansi.Truncate(hash+" · "+local.Format("01-02 15:04")+" · "+oneLine(commit.Author), width, "…")))
		}
	}
	fixed := []string{heading}
	if wide {
		fixed = append(fixed, header)
	}
	fixed = append(fixed, m.muted(strings.Repeat("─", width)))
	capacity := max(1, height-len(fixed))
	start := min(m.offset, max(0, len(rows)-capacity))
	fixed = append(fixed, rows[start:min(len(rows), start+capacity)]...)
	return strings.Join(fixed, "\n")
}
