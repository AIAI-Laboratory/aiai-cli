package developer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func focusDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dh %02dm", seconds/3600, seconds%3600/60)
}

func eventSource(source string) string {
	if source == "zen" {
		return "Focus"
	}
	return "Git"
}

func (m Model) joinColumns(columns []string, widths []int) string {
	cells := make([][]string, len(columns))
	height := 0
	for i, col := range columns {
		cells[i] = strings.Split(col, "\n")
		height = max(height, len(cells[i]))
	}
	rows := make([]string, height)
	for row := range height {
		pieces := make([]string, len(columns))
		for col := range columns {
			value := ""
			if row < len(cells[col]) {
				value = cells[col][row]
			}
			value = ansi.Truncate(value, widths[col], "…")
			pieces[col] = value + strings.Repeat(" ", max(0, widths[col]-ansi.StringWidth(value)))
		}
		rows[row] = strings.Join(pieces, m.muted(" │ "))
	}
	return strings.Join(rows, "\n")
}

func (m Model) timelineRows(width int) []string {
	o := m.selected()
	if o == nil {
		return []string{"Project unavailable"}
	}
	rows := []string{m.title(oneLine(o.Project.Name)), m.muted(pathTail(o.Project.Root, width)), "", m.title("Development timeline"), m.muted("Commits and focus sessions are separate facts"), ""}
	if m.timeline == nil {
		return rows
	}
	day := ""
	for _, event := range m.timeline.Events {
		date := event.Time.Local().Format("02 Jan 2006")
		if date != day {
			if day != "" {
				rows = append(rows, "")
			}
			rows = append(rows, m.title(date))
			day = date
		}
		text := oneLine(event.Text)
		text = strings.ReplaceAll(strings.ReplaceAll(text, "short_break", "Short break"), "long_break", "Long break")
		rows = append(rows, m.muted("  "+event.Time.Local().Format("15:04")+"  "+eventSource(event.Source)), "  "+ansi.Truncate(text, width-2, "…"))
	}
	if len(m.timeline.Events) == 0 {
		rows = append(rows, "No activity recorded.")
	}
	if m.timeline.Warning != "" {
		rows = append(rows, "", m.muted(oneLine(m.timeline.Warning)))
	}
	if m.timeline.Truncated {
		rows = append(rows, "", m.muted("Up to 100 reachable commits · CLI supports date filters"))
	}
	return rows
}
