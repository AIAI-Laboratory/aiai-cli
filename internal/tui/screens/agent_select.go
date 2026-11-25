package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

// AgentSelectState holds the resource checklist and existing AGENTS.md mode state.
type AgentSelectState struct {
	TargetDir      string
	Components     []templates.Component
	Checked        map[string]bool
	AllChecked     bool
	AgentsMDExists bool
	ShowAgentsMode bool
	AgentsMDMode   scaffold.ExistingMode
	Focus          int
	Error          string
}

// AgentSelect renders the selectable Agent Setup resource checklist screen.
func AgentSelect(state AgentSelectState, t Theme) string {
	var s strings.Builder
	if state.Error != "" {
		s.WriteString(t.Accent("! "+t.Truncate(state.Error, max(1, t.Width()-2))) + "\n\n")
	}
	s.WriteString(t.Heading("Select Agent Setup resources", "Check what to include. Existing skills and rules with matching names are never replaced."))
	if state.TargetDir != "" {
		s.WriteString(t.Muted("Destination  ") + t.Truncate(state.TargetDir, max(1, t.Width()-14)) + "\n\n")
	}

	renderRow := func(idx int, label, desc string) {
		prefix := "  "
		focused := state.Focus == idx
		if focused {
			prefix = t.Accent("> ")
		}
		text := t.Truncate(label, max(1, t.Width()-2))
		if focused {
			text = t.Accent(text)
		}
		s.WriteString(prefix + text + "\n")
		if desc != "" {
			s.WriteString("      " + t.Muted(t.Truncate(desc, max(1, t.Width()-6))) + "\n")
		}
	}

	renderRow(0, fmt.Sprintf("[%s] All (default)", mark(state.AllChecked)), "Add all default rules, skills, AGENTS.md, and docs")
	for i, c := range state.Components {
		renderRow(i+1, fmt.Sprintf("[%s] %s", mark(state.Checked[c.ID]), c.Name), c.Description)
	}

	nextIdx := len(state.Components) + 1
	if state.ShowAgentsMode {
		s.WriteString("\n" + t.Warn("! Existing AGENTS.md found in destination") + "\n")
		modeIdx := nextIdx
		nextIdx++
		prefix := "  "
		if state.Focus == modeIdx {
			prefix = t.Accent("> ")
		}
		options := formatAgentsMDMode(state.AgentsMDMode, t.Width() < 58)
		line := t.Truncate(options, max(1, t.Width()-2))
		if state.Focus == modeIdx {
			line = t.Accent(line)
		}
		s.WriteString(prefix + line + "\n")
		s.WriteString("  " + t.Muted(t.Truncate("Use ←/→ or Space to switch (default: Append)", max(1, t.Width()-2))) + "\n")
	}

	s.WriteString("\n")
	prefix := "  "
	action := "Continue to preview"
	if state.Focus == nextIdx {
		prefix = t.Accent("> ")
		action = t.Accent(action)
	}
	s.WriteString(prefix + action + "\n")
	return s.String()
}

func formatAgentsMDMode(mode scaffold.ExistingMode, compact bool) string {
	radio := func(target scaffold.ExistingMode, label string) string {
		if mode == target {
			return "(•) " + label
		}
		return "( ) " + label
	}
	if compact {
		return radio(scaffold.ExistingAppend, "Append") + "  " + radio(scaffold.ExistingReplace, "Replace") + "  " + radio(scaffold.ExistingSkip, "Skip")
	}
	return "AGENTS.md: " + radio(scaffold.ExistingAppend, "Append (default)") + "  " + radio(scaffold.ExistingReplace, "Replace") + "  " + radio(scaffold.ExistingSkip, "Skip")
}
