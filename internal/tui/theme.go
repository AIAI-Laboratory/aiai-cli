package tui

import "charm.land/lipgloss/v2"

func title(text string, noColor bool) string {
	style := lipgloss.NewStyle().Bold(true)
	if !noColor {
		style = style.Foreground(lipgloss.Color("#8B9DFF"))
	}
	return style.Render(text)
}
