// Package style keeps contrast and keyboard hints consistent across the TUIs.
package style

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	Accent       = "#FF3B30"
	Muted        = "#767676"
	Good         = "#208457"
	Info         = "#3579b3"
	Warn         = "#986d13"
	SearchPrefix = "> Search: "
)

// ResizeInput keeps the cursor visible when the terminal or field width changes.
func ResizeInput(input *textinput.Model, width int) {
	width = max(1, width)
	if input.Width() == width {
		return
	}
	pos := input.Position()
	input.SetWidth(width)
	input.CursorEnd()
	input.SetCursor(pos)
}

func Search(input textinput.Model, width int, noColor bool) string {
	input.Prompt = ""
	ResizeInput(&input, width-len(SearchPrefix)-1)
	return Paint("> ", Accent, true, noColor) + Paint("Search: ", "", true, noColor) +
		ansi.Truncate(input.View(), max(1, width-len(SearchPrefix)), "…")
}

func Paint(text, color string, bold, noColor bool) string {
	if noColor {
		return text
	}
	s := lipgloss.NewStyle().Bold(bold)
	if color != "" {
		s = s.Foreground(lipgloss.Color(color))
	}
	return s.Render(text)
}

func Keys(hint string, noColor bool) string {
	parts := strings.Split(hint, " · ")
	for i, part := range parts {
		key, label, _ := strings.Cut(part, " ")
		parts[i] = Paint(key, "", true, noColor)
		if label != "" {
			parts[i] += Paint(" "+label, Muted, false, noColor)
		}
	}
	return strings.Join(parts, "  ")
}

// Fit fixes both dimensions in terminal cells, including ANSI and wide glyphs.
func Fit(text string, width, height int) string {
	width, height = max(1, width), max(1, height)
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
