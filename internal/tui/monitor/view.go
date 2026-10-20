package monitor

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	width, height := max(1, m.width), max(1, m.height)
	if width < 32 || height < 12 {
		view := tea.NewView(lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render("AIAI MONITOR\nEnlarge terminal to 32×12\nq quit"))
		view.AltScreen = true
		return view
	}
	w := min(128, width-4)
	status := "LIVE"
	if m.paused {
		status = "PAUSED"
	} else if m.loading && m.snapshot.At.IsZero() {
		status = "SAMPLING"
	}
	header := m.accent("AIAI MONITOR") + m.muted(fmt.Sprintf("  %s · %s · %s", status, m.interval, clean(m.snapshot.System.Hostname)))
	if m.snapshot.System.UptimeSeconds != nil {
		header += m.muted(" · up " + (time.Duration(*m.snapshot.System.UptimeSeconds) * time.Second).Round(time.Minute).String())
	}
	header += "\n" + m.muted(strings.Repeat("─", w))
	separator := "\n\n"
	if height < 18 {
		header = m.accent("AIAI MONITOR") + m.muted("  "+status)
		separator = "\n"
	}
	var body string
	if m.help {
		body = "KEYBOARD\n\n↑/↓ j/k  Navigate processes or selected panel\nc / m / p  Sort CPU / RAM / PID\n/  Search process names or PID\nTab  Switch panel\n←/→ or [/]  Select network interface\nSpace  Pause / resume sampling\nr  Refresh now\n?  Close help\nEsc / q  Return\n\nRead-only · CPU process usage: 100% per core\nUnavailable metrics display as —"
	} else {
		if w >= 112 && height >= 26 {
			column := (w - 6) / 3
			fit := lipgloss.NewStyle().Width(column).MaxWidth(column)
			body = lipgloss.JoinHorizontal(lipgloss.Top,
				fit.Render(m.cpuView(column)), "   ",
				fit.Render(m.memoryView(column)), "   ",
				lipgloss.NewStyle().Width(w-2*column-6).MaxWidth(w-2*column-6).Render(m.networkView(w-2*column-6)))
			body += "\n\n" + m.diskView(w)
		} else if w >= 100 && height >= 26 {
			leftWidth := (w - 3) / 2
			rightWidth := w - leftWidth - 3
			left := m.cpuView(leftWidth) + "\n\n" + m.memoryView(leftWidth)
			right := m.networkView(rightWidth) + "\n\n" + m.diskView(rightWidth)
			body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Render(left), "   ", lipgloss.NewStyle().Width(rightWidth).MaxWidth(rightWidth).Render(right))
		} else if height >= 28 {
			body = m.cpuView(w) + "\n" + m.memoryView(w) + "\n" + m.networkView(w) + "\n" + m.diskView(w)
		} else {
			switch m.panel {
			case 0:
				body = m.cpuView(w) + "\n" + m.memoryCompact(w)
			case 1:
				body = m.memoryView(w)
			case 2:
				body = m.diskView(w) + "\n" + m.memoryCompact(w)
			case 3:
				body = m.networkView(w) + "\n" + m.memoryCompact(w)
			default:
				body = m.title(0, "CPU") + " " + m.bar(m.snapshot.CPU.Percent, min(20, max(4, w-22))) + "\n" + m.spark(m.cpuHistory, w, 100) + "\n" + m.memoryCompact(w)
			}
		}
		if height < 18 {
			body = m.title(0, "CPU") + " " + m.bar(m.snapshot.CPU.Percent, max(4, min(12, w-15))) + "\n" + m.memoryCompact(w)
		}
		if m.err != nil {
			body += "\n" + m.accent("Sample unavailable; press r to retry")
		}
		if len(m.snapshot.Warnings) > 0 {
			body += "\n" + m.muted(strings.Join(m.snapshot.Warnings, " · "))
		}
		if m.searching || m.search.Value() != "" {
			body += "\n" + m.search.View()
		}
		available := height - lipgloss.Height(header) - lipgloss.Height(body) - 6 - 2*(strings.Count(separator, "\n")-1)
		body += separator + m.processView(w, max(1, available))
	}
	footer := "↑/↓ select · c/m/p sort · / search · tab panel · space pause · r refresh · ? help · q back"
	if w < 80 {
		footer = "↑/↓ · c/m/p sort · / search · tab · ? help · q back"
	}
	if m.searching {
		footer = "Type to search · enter keep filter · esc clear"
	}
	content := header + separator + body
	// Reserve footer space, and truncate every line by terminal cells (including ANSI).
	lines := strings.Split(content, "\n")
	limit := height - 4
	if len(lines) > limit {
		lines = lines[:limit]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, w, "…")
	}
	for len(lines) < limit {
		lines = append(lines, "")
	}
	content = strings.Join(lines, "\n") + "\n" + ansi.Truncate(m.muted(footer), w, "…")
	view := tea.NewView(lipgloss.NewStyle().PaddingTop(1).PaddingBottom(1).PaddingLeft(max(2, (width-w)/2)).Render(content))
	view.AltScreen = true
	return view
}

func (m Model) cpuView(w int) string {
	s := m.section(0, fmt.Sprintf("CPU · %d cores", m.snapshot.System.LogicalCores), w) + "\n" + m.bar(m.snapshot.CPU.Percent, min(24, max(5, w-9)))
	s += "\n" + m.muted("History ") + m.spark(m.cpuHistory, max(1, w-8), 100)
	coreLabels := []string{}
	for i, percent := range m.snapshot.CPU.Cores {
		coreLabels = append(coreLabels, fmt.Sprintf("%2d %5s%%", i+1, percentText(percent)))
	}
	offset := min(m.coreOffset, max(0, len(coreLabels)-1))
	columns := max(1, min(6, w/12))
	for start := offset; start < len(coreLabels) && start < offset+columns*3; start += columns {
		s += "\n" + m.muted(strings.Join(coreLabels[start:min(start+columns, len(coreLabels))], "  "))
	}
	if len(coreLabels) > columns*3 {
		s += m.muted("  ↑/↓ more cores")
	}
	return s
}

func (m Model) memoryCompact(w int) string {
	if m.snapshot.Memory == nil {
		return m.title(1, "RAM") + " — unavailable"
	}
	ram := m.snapshot.Memory
	return m.title(1, "RAM") + " " + m.bar(&ram.Percent, min(20, max(4, w-22))) + m.muted("  "+bytes(ram.Used)+" / "+bytes(ram.Total))
}

func (m Model) memoryView(w int) string {
	s := m.section(1, "MEMORY", w)
	if m.snapshot.Memory != nil {
		ram := m.snapshot.Memory
		s += "\n" + m.bar(&ram.Percent, min(24, max(5, w-9)))
		s += "\n" + bytes(ram.Used) + m.muted(" used / "+bytes(ram.Total))
		s += "\n" + m.muted("Available  ") + bytes(ram.Available)
	} else {
		s += "\n" + m.muted("RAM unavailable")
	}
	if m.snapshot.Swap == nil {
		s += "\n" + m.muted("Swap unavailable")
	} else {
		s += "\n" + m.muted("Swap "+bytes(m.snapshot.Swap.Used)+" / "+bytes(m.snapshot.Swap.Total))
	}
	return s
}

func (m Model) networkView(w int) string {
	s := m.section(3, "NETWORK", w)
	for _, iface := range m.snapshot.Networks {
		if iface.Name == m.iface || m.iface == "" {
			s += "\n" + clean(iface.Name) + m.muted("  ←/→ interface")
			s += "\n" + m.muted("↓ ") + rate(iface.Download) + "  " + m.spark(m.downloadHistory[iface.Name], max(1, w-19), 0)
			s += "\n" + m.muted("↑ ") + rate(iface.Upload) + "  " + m.spark(m.uploadHistory[iface.Name], max(1, w-19), 0)
			return s
		}
	}
	return s + "\n" + m.muted("Network unavailable")
}

func (m Model) diskView(w int) string {
	s := m.section(2, "DISKS", w) + "\n" + m.muted("Read "+rate(m.snapshot.DiskIO.Read)+" · Write "+rate(m.snapshot.DiskIO.Write))
	if len(m.snapshot.Disks) == 0 {
		return s + "\n" + m.muted("Disk usage unavailable")
	}
	offset := min(m.diskOffset, len(m.snapshot.Disks)-1)
	if w >= 90 {
		item := m.snapshot.Disks[offset]
		line := ansi.Truncate(clean(item.Mount), 24, "…") + m.muted(fmt.Sprintf("  [%d/%d]  ", offset+1, len(m.snapshot.Disks)))
		if item.Usage == nil {
			line += m.muted("unavailable")
		} else {
			line += m.bar(&item.Usage.Percent, 12) + "  " + bytes(item.Usage.Used) + m.muted(" / "+bytes(item.Usage.Total)+" · Free "+bytes(item.Usage.Available))
		}
		return s + "\n" + line + m.muted("  · Tab → Disks, ↑/↓ mounts")
	}
	for _, item := range m.snapshot.Disks[offset:min(offset+3, len(m.snapshot.Disks))] {
		mount := ansi.Truncate(clean(item.Mount), max(6, w/3), "…")
		if item.Usage == nil {
			s += "\n" + mount + m.muted(" unavailable")
			continue
		}
		line := mount + fmt.Sprintf(" %.0f%%", item.Usage.Percent) + m.muted("  "+bytes(item.Usage.Used)+" / "+bytes(item.Usage.Total))
		s += "\n" + ansi.Truncate(line, w, "…")
		s += "\n" + m.muted("  Free "+bytes(item.Usage.Available))
	}
	if len(m.snapshot.Disks) > 3 {
		s += "\n" + m.muted("Tab to Disks · ↑/↓ scroll mounts")
	}
	return s
}

func (m Model) processView(w, rows int) string {
	items := m.processes()
	s := m.section(4, fmt.Sprintf("PROCESSES · %d · %s ↓", len(items), strings.ToUpper(m.order)), w) + "\n"
	nameWidth := max(2, w-26)
	s += m.muted(fmt.Sprintf("  %6s %6s %8s  %-*s", "PID", "CPU%", "RAM", nameWidth, "NAME"))
	if len(items) == 0 {
		return s + "\n" + m.muted("No processes available or matching your search")
	}
	cursor := max(0, min(m.cursor, len(items)-1))
	offset := max(0, cursor-rows+1)
	for i := offset; i < min(offset+rows, len(items)); i++ {
		item := items[i]
		ram := "—"
		if item.Memory != nil {
			ram = bytes(*item.Memory)
		}
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		name := lipgloss.NewStyle().Width(nameWidth).Render(ansi.Truncate(clean(item.Name), nameWidth, "…"))
		line := fmt.Sprintf("%s%6d %6s %8s  %s", prefix, item.PID, percentText(item.CPU), ram, name)
		if i == cursor {
			line = m.accent(line)
		}
		s += "\n" + line
	}
	return s
}

func (m Model) section(panel int, label string, w int) string {
	title := m.title(panel, label)
	return title + " " + m.muted(strings.Repeat("─", max(0, w-lipgloss.Width(title)-1)))
}

func (m Model) title(panel int, text string) string {
	if m.panel == panel {
		return m.accent("> " + text)
	}
	return lipgloss.NewStyle().Bold(true).Render(text)
}

func (m Model) accent(s string) string {
	style := lipgloss.NewStyle().Bold(true)
	if !m.noColor {
		style = style.Foreground(lipgloss.Color("#FF5F56"))
	}
	return style.Render(s)
}

func (m Model) muted(s string) string {
	style := lipgloss.NewStyle()
	if !m.noColor {
		style = style.Foreground(lipgloss.Color("#808080"))
	}
	return style.Render(s)
}

func percentText(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f", *value)
}

func bytes(value uint64) string {
	v := float64(value)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

func rate(value *float64) string {
	if value == nil {
		return "unavailable"
	}
	return bytes(uint64(max(0, *value))) + "/s"
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func (m Model) bar(value *float64, w int) string {
	if value == nil {
		return m.muted("unavailable")
	}
	filled := int(math.Round(max(0, min(100, *value)) * float64(w) / 100))
	return m.accent(strings.Repeat("█", filled)) + m.muted(strings.Repeat("░", w-filled)) + fmt.Sprintf(" %5.1f%%", *value)
}

func (m Model) spark(history []float64, w int, scale float64) string {
	if len(history) == 0 {
		return m.muted("Waiting for samples…")
	}
	if len(history) > w {
		history = history[len(history)-w:]
	}
	if scale == 0 {
		for _, point := range history {
			if !math.IsNaN(point) {
				scale = max(scale, point)
			}
		}
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, point := range history {
		if math.IsNaN(point) {
			b.WriteRune('·')
			continue
		}
		index := 0
		if scale > 0 {
			index = int(math.Round(max(0, min(1, point/scale)) * 7))
		}
		b.WriteRune(glyphs[index])
	}
	return m.accent(b.String())
}
