package screens

import "fmt"

// InitFormState holds the rendered inputs and toggles displayed on the init configuration screen.
type InitFormState struct {
	AgentOnly        bool
	Inputs           []string
	Focus            int
	ShowIncludeAgent bool
	IncludeAgent     bool
	Force            bool
	DryRun           bool
	NextLabel        string
}

// InitForm renders the project or agent destination configuration form.
func InitForm(state InitFormState, t Theme) string {
	var labels, hints []string
	var s string
	if state.AgentOnly {
		labels = []string{"Destination"}
		hints = []string{"Blank uses the current directory (.)"}
		s = t.Heading("Configure Agent Setup", "Choose the target repository. Next you can select rules, skills, and docs.")
	} else {
		labels = []string{"Project name", "Destination", "Python package"}
		hints = []string{"Blank uses the destination name", "Blank uses the project name", "Blank derives from the project name"}
		s = t.Heading("Configure your project", "Set the essentials. We will preview every file first.")
	}

	for i, input := range state.Inputs {
		prefix := "  "
		if i == state.Focus {
			prefix = t.Accent("> ")
		}
		label := t.Title(labels[i])
		if i == state.Focus {
			label = t.Accent(labels[i])
		}
		if t.Width() >= 62 {
			s += prefix + label + t.Muted(" · "+hints[i]) + "\n  " + input + "\n"
		} else {
			s += prefix + label + "\n  " + input + "\n  " + t.Muted(hints[i]) + "\n"
		}
		if i < len(state.Inputs)-1 {
			s += "\n"
		}
	}

	s += "\n" + t.Section("Options") + "\n"
	nextLabel := state.NextLabel
	if nextLabel == "" {
		nextLabel = "Continue to preview"
	}
	options := make([]string, 0, 4)
	if state.ShowIncludeAgent {
		options = append(options, fmt.Sprintf("[%s] Include Agent Setup (AGENTS.md, rules, skills, docs)", mark(state.IncludeAgent)))
	}
	options = append(options,
		fmt.Sprintf("[%s] Replace conflicting files (--force)", mark(state.Force)),
		fmt.Sprintf("[%s] Preview only (--dry-run)", mark(state.DryRun)),
		nextLabel,
	)

	offset := len(state.Inputs)
	for i, item := range options {
		idx := offset + i
		prefix := "  "
		if state.Focus == idx {
			prefix = t.Accent("> ")
			item = t.Accent(t.Truncate(item, max(1, t.Width()-2)))
		} else {
			item = t.Truncate(item, max(1, t.Width()-2))
		}
		s += prefix + item + "\n"
	}
	return s
}

func mark(on bool) string {
	if on {
		return "x"
	}
	return " "
}
