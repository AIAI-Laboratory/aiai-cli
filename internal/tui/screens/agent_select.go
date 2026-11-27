package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

// AgentSelectState holds the resource checklist, optional inline destination editor, and AGENTS.md mode state.
type AgentSelectState struct {
	AgentOnly      bool
	TargetDir      string
	EditingDest    bool
	DestInput      string
	Components     []templates.Component
	Checked        map[string]bool
	AllChecked     bool
	AgentsMDExists bool
	ShowAgentsMode bool
	AgentsMDMode   scaffold.ExistingMode
	Focus          int
	Error          string
}

// AgentSelect renders the compact Agent Setup resource checklist screen.
func AgentSelect(state AgentSelectState, t Theme) string {
	var s strings.Builder
	if state.Error != "" {
		s.WriteString(t.Accent("! "+t.Truncate(state.Error, max(1, t.Width()-2))) + "\n\n")
	}
	s.WriteString(t.Heading("Select Agent Setup resources", "Space toggles items, a toggles all, Enter continues. Existing skills and rules are protected."))

	if state.AgentOnly {
		if state.EditingDest {
			if t.Width() >= 62 {
				s.WriteString(t.Accent("> Destination") + t.Muted(" · Enter or Tab to confirm") + "\n  " + state.DestInput + "\n\n")
			} else {
				s.WriteString(t.Accent("> Destination") + "\n  " + state.DestInput + "\n  " + t.Muted("Enter/Tab to confirm") + "\n\n")
			}
		} else {
			dest := state.TargetDir
			if dest == "" {
				dest = "."
			}
			prefix := "Destination  "
			hint := "  (Tab or d to change)"
			if t.Width() < 58 {
				prefix = "Dest: "
				hint = "  (Tab)"
			}
			if t.Width() < len(prefix)+len(hint)+4 {
				hint = ""
			}
			avail := max(1, t.Width()-len(prefix)-len(hint))
			line := t.Muted(prefix) + t.Truncate(dest, avail)
			if hint != "" {
				line += t.Muted(hint)
			}
			s.WriteString(line + "\n\n")
		}
	} else if state.TargetDir != "" {
		s.WriteString(t.Muted("Destination  ") + t.Truncate(state.TargetDir, max(1, t.Width()-14)) + "\n\n")
	}

	var focusedDesc string
	renderRow := func(idx int, label, desc string) {
		prefix := "  "
		focused := !state.EditingDest && state.Focus == idx
		if focused {
			prefix = t.Accent("> ")
			focusedDesc = desc
		}
		line := label
		if t.Width() >= 68 && desc != "" {
			line += " · " + desc
		}
		if t.Width() >= 68 && desc != "" {
			// Style the name and muted description separately while respecting total terminal width.
			labelBudget := max(1, t.Width()-2)
			namePart := t.Truncate(label, labelBudget)
			if focused {
				s.WriteString(prefix + t.Accent(namePart))
			} else {
				s.WriteString(prefix + namePart)
			}
			rem := labelBudget - len([]rune(namePart))
			if rem > 4 {
				s.WriteString(t.Muted(t.Truncate(" · "+desc, rem)))
			}
			s.WriteString("\n")
			return
		}
		text := t.Truncate(line, max(1, t.Width()-2))
		if focused {
			text = t.Accent(text)
		}
		s.WriteString(prefix + text + "\n")
	}

	renderRow(0, fmt.Sprintf("[%s] All (default)", mark(state.AllChecked)), "All rules, skills, AGENTS.md, and docs")
	for i, c := range state.Components {
		renderRow(i+1, fmt.Sprintf("[%s] %s", mark(state.Checked[c.ID]), c.Name), c.Description)
	}

	if t.Width() < 68 && focusedDesc != "" {
		s.WriteString("  " + t.Muted(t.Truncate(focusedDesc, max(1, t.Width()-2))) + "\n")
	}

	if state.ShowAgentsMode {
		s.WriteString("\n" + t.Warn("! Existing AGENTS.md found in destination") + "\n")
		modeIdx := len(state.Components) + 1
		prefix := "  "
		focused := !state.EditingDest && state.Focus == modeIdx
		if focused {
			prefix = t.Accent("> ")
		}
		options := formatAgentsMDMode(state.AgentsMDMode, t.Width() < 58)
		line := t.Truncate(options, max(1, t.Width()-2))
		if focused {
			line = t.Accent(line)
		}
		s.WriteString(prefix + line + "\n")
	}

	if !state.EditingDest {
		s.WriteString("\n" + t.Muted(t.Truncate("Enter to create · Space toggle · a all · Esc back", max(1, t.Width()))) + "\n")
	}
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
