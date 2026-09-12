package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme leaves the terminal background and normal foreground untouched so the
// interface works with the user's own terminal palette.
type Theme struct {
	width   int
	noColor bool
}

func NewTheme(width int, noColor bool) Theme {
	return Theme{width: width, noColor: noColor}
}

func (t Theme) Width() int    { return t.width }
func (t Theme) NoColor() bool { return t.noColor }

func (t Theme) paint(text, color string, bold bool) string {
	s := lipgloss.NewStyle().Bold(bold)
	if !t.noColor {
		s = s.Foreground(lipgloss.Color(color))
	}
	return s.Render(text)
}

func (t Theme) Accent(text string) string { return t.paint(text, "#FF5F56", true) }
func (t Theme) Brand(text string) string  { return t.paint(text, "#FF3B30", true) }
func (t Theme) Muted(text string) string  { return t.paint(text, "#808080", false) }
func (t Theme) Good(text string) string   { return t.paint(text, "#36A879", false) }
func (t Theme) Warn(text string) string   { return t.paint(text, "#C99739", false) }
func (t Theme) Title(text string) string  { return lipgloss.NewStyle().Bold(true).Render(text) }
func (t Theme) Rule() string              { return t.Muted(strings.Repeat("─", max(1, t.width))) }

func (t Theme) Heading(title, subtitle string) string {
	return t.Title(title) + "\n" + t.Muted(subtitle) + "\n\n"
}

func (t Theme) Keys(hint string) string {
	parts := strings.Split(hint, " · ")
	for i, part := range parts {
		key, label, _ := strings.Cut(part, " ")
		parts[i] = t.Accent(key)
		if label != "" {
			parts[i] += t.Muted(" " + label)
		}
	}
	return strings.Join(parts, t.Muted(" · "))
}

func (t Theme) Choice(label, description string, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
		label = t.Accent(label)
	}
	if description == "" {
		return prefix + label + "\n"
	}
	if t.width < 62 {
		return prefix + label + "\n  " + t.Muted(description) + "\n"
	}
	return prefix + lipgloss.NewStyle().Width(16).Render(label) + "  " + t.Muted(description) + "\n"
}

func (t Theme) Step(current int) string {
	steps := []string{"Template", "Configure", "Preview", "Create"}
	for i, step := range steps {
		label := fmt.Sprintf("%d %s", i+1, step)
		if t.width < 52 && i != current {
			label = fmt.Sprint(i + 1)
		}
		if i == current {
			steps[i] = t.Accent(label)
		} else if i < current {
			steps[i] = t.Good(label)
		} else {
			steps[i] = t.Muted(label)
		}
	}
	return strings.Join(steps, t.Muted("  /  "))
}
