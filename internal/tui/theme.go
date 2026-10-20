package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AIAI-Laboratory/aiai-cli/internal/tui/style"
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
	return style.Paint(text, color, bold, t.noColor)
}

func (t Theme) Accent(text string) string { return t.paint(text, style.Accent, true) }
func (t Theme) Brand(text string) string  { return t.paint(text, style.Accent, true) }
func (t Theme) Muted(text string) string  { return t.paint(text, style.Muted, false) }
func (t Theme) Good(text string) string   { return t.paint(text, style.Good, false) }
func (t Theme) Warn(text string) string   { return t.paint(text, style.Warn, false) }
func (t Theme) Title(text string) string  { return t.paint(text, "", true) }
func (t Theme) Rule() string              { return t.Muted(strings.Repeat("─", max(1, t.width))) }

func (t Theme) Section(title string) string {
	return t.Title(title)
}

func (t Theme) Truncate(text string, width int) string { return truncate(text, width) }

func (t Theme) Heading(title, subtitle string) string {
	if subtitle == "" {
		return t.Title(title) + "\n\n"
	}
	return t.Title(title) + "\n" + t.Muted(subtitle) + "\n\n"
}

func (t Theme) Keys(hint string) string {
	return ansi.Truncate(style.Keys(hint, t.noColor), t.width, "…")
}

func (t Theme) Choice(label, description string, selected bool) string {
	labelWidth := max(1, t.width-2)
	descriptionWidth := max(1, t.width-4)
	if t.width >= 62 {
		labelWidth = 16
		descriptionWidth = max(1, t.width-20)
	}
	label = truncate(label, labelWidth)
	description = truncate(description, descriptionWidth)
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

func truncate(text string, width int) string {
	return ansi.Truncate(text, max(1, width), "…")
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
