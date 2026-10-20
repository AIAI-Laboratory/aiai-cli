package developer

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
)

func (m Model) gitTabs(width int) string {
	if width < 65 {
		return m.accent("[ "+pages[m.opts.Page]+" ]") + m.muted(fmt.Sprintf("  %d / %d", m.opts.Page+1, len(pages)))
	}
	labels := make([]string, len(pages))
	for i, label := range pages {
		if i == m.opts.Page {
			labels[i] = m.accent("[ " + label + " ]")
		} else {
			labels[i] = m.muted(label)
		}
	}
	return strings.Join(labels, "  ")
}

func (m Model) field(label, value string, width int) string {
	return "  " + m.muted(column(label, 14)) + " " + ansi.Truncate(oneLine(value), max(1, width-17), "…")
}

func (m Model) pageRows(rows []string, width, height int) string {
	result := []string{}
	query := strings.ToLower(m.search.Value())
	for _, row := range rows {
		if query == "" || strings.Contains(strings.ToLower(ansi.Strip(row)), query) {
			result = append(result, row)
		}
	}
	return m.viewPage(result, width, height)
}

func (m Model) viewPage(rows []string, width, height int) string {
	if len(rows) == 0 {
		return m.muted("Nothing matches this search.")
	}
	start := min(m.offset, max(0, len(rows)-height))
	visible := append([]string{}, rows[start:min(len(rows), start+height)]...)
	for i := range visible {
		visible[i] = ansi.Truncate(visible[i], width, "…")
	}
	return strings.Join(visible, "\n")
}

func (m Model) matches(text string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(m.search.Value()))
}
func shortHead(head string) string { return head[:min(8, len(head))] }

func (m Model) statusView(width, height int) string {
	s := m.status
	branch := s.Branch
	if s.Detached {
		branch = "Detached HEAD · " + shortHead(s.Head)
	}
	if s.Unborn {
		branch += " · no commits"
	}
	rows := []string{m.muted(pathTail(s.Root, width)), "", m.title("Working tree"), m.field("Branch", branch, width)}
	if len(s.Changes) == 0 {
		rows = append(rows, m.field("Changes", "Clean", width))
	} else {
		rows = append(rows, m.field("Changes", fmt.Sprintf("%d staged   %d unstaged   %d new", s.Staged, s.Unstaged, s.Untracked), width))
	}
	if s.Conflicts > 0 {
		rows = append(rows, m.tone(m.field("Conflicts", fmt.Sprintf("%d files need attention", s.Conflicts), width), style.Accent))
	}
	rows = append(rows, "", m.title("Tracking · local only"))
	if s.Upstream == "" {
		rows = append(rows, m.field("Upstream", "Not configured", width))
	} else {
		rows = append(rows, m.field("Upstream", s.Upstream, width))
		summary := "Up to date"
		if s.Ahead != 0 || s.Behind != 0 {
			summary = fmt.Sprintf("%d ahead · %d behind", s.Ahead, s.Behind)
		}
		rows = append(rows, m.field("Ahead / behind", summary, width))
	}
	rows = append(rows, "", m.title("Latest commit"))
	if s.LastCommit == nil {
		rows = append(rows, "  No commits yet.")
	} else {
		c := s.LastCommit
		rows = append(rows, "  "+ansi.Truncate(oneLine(c.Subject), width-2, "…"), m.muted("  "+shortHead(c.Hash)+" · "+oneLine(c.Author)+" · "+c.Date.Local().Format("02 Jan 15:04")))
	}
	return m.pageRows(rows, width, height)
}

func (m Model) branchesView(width, height int) string {
	if len(m.branches) == 0 {
		return m.muted("No local branches yet.")
	}
	rows := []string{m.title("Local branches"), ""}
	for _, b := range m.branches {
		if !m.matches(b.Name + " " + b.Upstream + " " + b.Head) {
			continue
		}
		prefix := "  "
		name := oneLine(b.Name)
		if b.Current {
			prefix = "> "
			name = m.accent(name)
		}
		rows = append(rows, prefix+name+m.muted("  "+shortHead(b.Head)))
		tracking := "No upstream"
		if b.Upstream != "" {
			tracking = oneLine(b.Upstream)
			if b.Tracking != "" {
				tracking += " · " + strings.Trim(oneLine(b.Tracking), "[]")
			}
		}
		rows = append(rows, m.muted("  "+tracking), "")
	}
	if len(rows) == 2 {
		return m.muted("No branches match this search.")
	}
	return m.viewPage(rows, width, height)
}

func (m Model) worktreesView(width, height int) string {
	if len(m.trees) == 0 {
		return m.muted("No worktrees available.")
	}
	rows := []string{m.title("Worktrees"), ""}
	for _, w := range m.trees {
		if !m.matches(w.Path + " " + w.Branch + " " + w.Head) {
			continue
		}
		branch := w.Branch
		if w.Detached {
			branch = "Detached HEAD"
		}
		prefix := "  "
		if w.Path == m.status.Root {
			prefix = "> "
			branch = m.accent(oneLine(branch))
		} else {
			branch = m.title(oneLine(branch))
		}
		rows = append(rows, prefix+branch, m.muted("  "+pathTail(w.Path, width-2)))
		flags := []string{shortHead(w.Head)}
		if w.Locked != "" {
			flags = append(flags, "Locked: "+oneLine(w.Locked))
		}
		if w.Prunable != "" {
			flags = append(flags, "Prunable: "+oneLine(w.Prunable))
		}
		rows = append(rows, m.muted("  "+strings.Join(flags, " · ")), "")
	}
	if len(rows) == 2 {
		return m.muted("No worktrees match this search.")
	}
	return m.viewPage(rows, width, height)
}

func (m Model) activityView(width, height int) string {
	if m.activity == nil {
		return m.muted("No commit activity yet.")
	}
	a := m.activity
	rows := []string{m.title("Last 7 days"), m.muted("All authors · reachable commits"), ""}
	peak := 1
	now := time.Now()
	for i := 6; i >= 0; i-- {
		peak = max(peak, a.Days[now.AddDate(0, 0, -i).Format("2006-01-02")])
	}
	barWidth := max(4, min(24, width-22))
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		count := a.Days[day.Format("2006-01-02")]
		bar := m.tone(column(strings.Repeat("━", count*barWidth/peak), barWidth), style.Info)
		rows = append(rows, "  "+m.muted(day.Format("Mon 02 Jan"))+"  "+bar+fmt.Sprintf("  %d", count))
	}
	rows = append(rows, "", m.title("Recent commits"))
	if a.Truncated {
		rows = append(rows, m.muted("Showing a bounded history snapshot"))
	}
	for _, c := range a.Commits {
		rows = append(rows, "  "+ansi.Truncate(oneLine(c.Subject), width-2, "…"), m.muted("  "+shortHead(c.Hash)+" · "+c.Date.Local().Format("02 Jan 15:04")+" · "+oneLine(c.Author)), "")
	}
	return m.pageRows(rows, width, height)
}
