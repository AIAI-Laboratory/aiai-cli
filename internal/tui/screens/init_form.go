package screens

import "fmt"

func InitForm(inputs []string, focus int, force, dryRun bool, t Theme) string {
	labels := []string{"Project name", "Destination", "Python package"}
	hints := []string{"Blank uses the destination name", "Blank uses the project name", "Blank derives from the project name"}
	s := t.Heading("Configure your project", "Set the essentials. We will preview every file first.")
	for i, input := range inputs {
		prefix := "  "
		if i == focus {
			prefix = "> "
		}
		label := t.Title(labels[i])
		if i == focus {
			label = t.Accent(labels[i])
		}
		if t.Width() >= 62 {
			s += prefix + label + t.Muted(" · "+hints[i]) + "\n  " + input + "\n"
		} else {
			s += prefix + label + "\n  " + input + "\n  " + t.Muted(hints[i]) + "\n"
		}
		if i < len(inputs)-1 {
			s += "\n"
		}
	}
	for i, item := range []string{fmt.Sprintf("[%s] Replace conflicting files (--force)", mark(force)), fmt.Sprintf("[%s] Preview only (--dry-run)", mark(dryRun)), "Preview project"} {
		prefix := "  "
		if focus == i+3 {
			prefix = "> "
		}
		if focus == i+3 {
			item = t.Accent(item)
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
