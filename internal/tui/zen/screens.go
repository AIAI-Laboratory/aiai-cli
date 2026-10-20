package zen

import (
	"fmt"
	"strings"
	"time"

	zenmodel "github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

func statsView(sessions []zenmodel.Session, now time.Time, width int, noColor bool) string {
	stats := zenmodel.CalculateStatistics(sessions, now)
	var out strings.Builder
	out.WriteString(bold("FOCUS STATISTICS") + "\n\n" + bold("TODAY") + "\n")
	fmt.Fprintf(&out, "  Focus time       %s\n  Completed        %d\n  Interrupted      %d\n  Completion rate  %d%%\n\n", duration(stats.Today.FocusSeconds), stats.Today.Completed, stats.Today.Interrupted, stats.Today.CompletionRate)
	out.WriteString(bold("THIS WEEK") + "\n")
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
	out.WriteString("\n" + muted("enter menu · esc back", noColor))
	return out.String()
}

func historyView(sessions []zenmodel.Session, width int, noColor bool) string {
	var out strings.Builder
	out.WriteString(bold("SESSION HISTORY") + "\n\n")
	if len(sessions) == 0 {
		return out.String() + muted("No focus sessions yet. Start gently; the fish will wait.", noColor)
	}
	limit := min(14, len(sessions))
	if width >= 68 {
		out.WriteString(muted("DATE        START  TYPE          PLANNED  ACTUAL   STATUS", noColor) + "\n")
		for _, item := range sessions[:limit] {
			fmt.Fprintf(&out, "%-11s %-6s %-13s %-8s %-8s %s\n", item.StartedAt.Local().Format("2006-01-02"), item.StartedAt.Local().Format("15:04"), item.Type, duration(item.PlannedSeconds), duration(item.ActiveSeconds), item.Status)
		}
	} else {
		for _, item := range sessions[:limit] {
			fmt.Fprintf(&out, "%s %s  %s\n  %s / %s · %s\n\n", item.StartedAt.Local().Format("Jan 02"), item.StartedAt.Local().Format("15:04"), item.Type, duration(item.ActiveSeconds), duration(item.PlannedSeconds), item.Status)
		}
	}
	out.WriteString("\n" + muted("enter menu · esc back", noColor))
	return out.String()
}

func duration(seconds int64) string {
	value := time.Duration(seconds) * time.Second
	if value >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(value.Hours()), int(value.Minutes())%60)
	}
	return fmt.Sprintf("%dm %02ds", int(value.Minutes()), int(value.Seconds())%60)
}
