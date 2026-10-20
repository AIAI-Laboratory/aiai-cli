package zen

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"

	zenmodel "github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

func statsView(sessions []zenmodel.Session, now time.Time, width int, noColor bool) string {
	stats := zenmodel.CalculateStatistics(sessions, now)
	var out strings.Builder
	out.WriteString(bold("Today") + "\n")
	for _, item := range [][2]string{{"Focus time", duration(stats.Today.FocusSeconds)}, {"Completed", fmt.Sprint(stats.Today.Completed)}, {"Interrupted", fmt.Sprint(stats.Today.Interrupted)}, {"Completion", fmt.Sprintf("%d%%", stats.Today.CompletionRate)}} {
		fmt.Fprintf(&out, "  %s %s\n", muted(fmt.Sprintf("%-12s", item[0]), noColor), bold(item[1]))
	}
	out.WriteString("\n" + bold("This week") + "\n")
	maxSeconds := int64(1)
	for _, day := range stats.Week {
		maxSeconds = max(maxSeconds, day.FocusSeconds)
	}
	barWidth := max(4, min(28, width-22))
	for _, day := range stats.Week {
		date, _ := time.Parse("2006-01-02", day.Date)
		filled := int(day.FocusSeconds * int64(barWidth) / maxSeconds)
		fmt.Fprintf(&out, "  %s  %s%s  %s\n", date.Format("Mon"), accent(strings.Repeat("█", filled), noColor), muted(strings.Repeat("░", barWidth-filled), noColor), duration(day.FocusSeconds))
	}
	return out.String()
}

func historyView(sessions []zenmodel.Session, width int, noColor bool) string {
	var out strings.Builder
	out.WriteString(bold("Session history") + "\n\n")
	if len(sessions) == 0 {
		return out.String() + muted("No focus sessions yet. Start gently; the fish will wait.", noColor)
	}
	limit := min(14, len(sessions))
	if width >= 68 {
		out.WriteString(muted("DATE        START  TYPE          PLANNED  ACTUAL   STATUS", noColor) + "\n" + muted(strings.Repeat("─", width), noColor) + "\n")
		for _, item := range sessions[:limit] {
			fmt.Fprintf(&out, "%-11s %-6s %-13s %-8s %-8s %s\n", item.StartedAt.Local().Format("2006-01-02"), item.StartedAt.Local().Format("15:04"), sessionLabel(item.Type), duration(item.PlannedSeconds), duration(item.ActiveSeconds), sessionStatus(item.Status, noColor))
		}
	} else {
		for _, item := range sessions[:limit] {
			fmt.Fprintf(&out, "%s  %s\n  %s active · %s\n\n", bold(sessionLabel(item.Type)), muted(item.StartedAt.Local().Format("02 Jan 15:04"), noColor), duration(item.ActiveSeconds), sessionStatus(item.Status, noColor))
		}
	}
	lines := strings.Split(out.String(), "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}

func sessionLabel(kind zenmodel.SessionType) string {
	switch kind {
	case zenmodel.SessionShortBreak:
		return "Short break"
	case zenmodel.SessionLongBreak:
		return "Long break"
	default:
		return "Focus"
	}
}

func sessionStatus(status zenmodel.SessionStatus, noColor bool) string {
	color := style.Muted
	switch status {
	case zenmodel.StatusCompleted:
		color = style.Good
	case zenmodel.StatusInterrupted, zenmodel.StatusCanceled:
		color = style.Warn
	}
	return style.Paint(string(status), color, false, noColor)
}

func duration(seconds int64) string {
	value := time.Duration(seconds) * time.Second
	if value >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(value.Hours()), int(value.Minutes())%60)
	}
	return fmt.Sprintf("%dm %02ds", int(value.Minutes()), int(value.Seconds())%60)
}
