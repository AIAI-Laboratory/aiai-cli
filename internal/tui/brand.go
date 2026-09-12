package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The open A and red i² reproduce the supplied AIAI mark in terminal cells.
// Use the terminal foreground for the A so it remains visible on dark themes.
func (m Model) logo() string {
	a := []string{
		"      ▄██▄    ",
		"     ██████   ",
		"    ▐██▌▐██▌  ",
		"   ▄██▀  ▀██▄ ",
		"  ▄██▀    ▀██▄",
		" ▐██▌      ▐██",
		" ▀▀▀        ▀▀",
	}
	i := []string{" ▄▀▀▄ ", "  ▄▀  ", " ▀▀▀▀ ", " ████ ", " ████ ", " ████ ", " ▀▀▀▀ "}
	for row := range a {
		a[row] += m.theme().Brand(i[row])
	}
	return strings.Join(a, "\n")
}

func (m Model) header() string {
	t := m.theme()
	version := m.opts.Version
	if version == "" {
		version = "dev"
	}
	name := t.Title("AIAI CLI") + "  " + t.Muted(version)
	if m.height < 22 || t.Width < 55 || (m.screen != home && m.screen != about) {
		return name + "\n" + t.Rule()
	}
	detail := "\n" + name + "\n" + t.Muted("Your next project starts here.") + "\n\n" +
		t.Muted(m.workspace) + "\n" + t.Good("●") + t.Muted(" Offline · Embedded templates")
	detail = lipgloss.NewStyle().Width(max(1, t.Width-24)).MaxWidth(max(1, t.Width-24)).Render(detail)
	return "\n" + lipgloss.JoinHorizontal(lipgloss.Top, m.logo(), "    ", detail) + "\n\n" + t.Rule()
}

func (m Model) toolbar() string {
	t := m.theme()
	if m.screen == home {
		return t.Accent("❯ ") + m.command.View() + "\n" + t.Rule() + "\n"
	}
	step := -1
	switch m.screen {
	case selection:
		step = 0
	case form, planning:
		step = 1
	case preview, confirmation:
		step = 2
	case applying, result:
		step = 3
	}
	if step >= 0 {
		return t.Step(step) + "\n\n"
	}
	return t.Muted("AIAI / "+map[screen]string{help: "Help", about: "About"}[m.screen]) + "\n\n"
}

func (m Model) footer() string {
	t := m.theme()
	hint := "↑/↓ navigate · enter select · esc back"
	switch m.screen {
	case home:
		hint = "↑/↓ navigate · enter select · tab complete"
	case form:
		hint = "tab next · shift+tab previous · enter continue"
	case preview:
		hint = "↑/↓ scroll · enter continue · esc edit"
		if m.plan.HasConflicts() {
			hint = "↑/↓ scroll · esc edit options"
		}
	case confirmation:
		hint = "↑/↓ choose · enter confirm · esc preview"
	case result:
		hint = "↑/↓ scroll · enter finish"
	case help, about:
		hint = "↑/↓ scroll · esc back"
	case planning, applying:
		hint = "ctrl+c cancel safely"
	}
	if t.Width < 48 {
		hint = "↑/↓ · tab · enter · esc · ?"
	}
	status := "? help · ctrl+c cancel"
	if m.screen == home {
		status = "Type / to browse commands · esc quit"
	}
	if m.opts.DryRun {
		status = "Preview only · No files will be written"
	}
	if t.Width < 48 {
		status = "ctrl+c cancel"
		if m.screen == home {
			status = "/ commands · esc quit"
		}
	}
	return t.Keys(hint) + "\n" + t.Muted(status)
}
