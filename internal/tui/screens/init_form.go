package screens

import "fmt"

func InitForm(inputs []string, focus int, force, dryRun bool) string {
	labels := []string{"Project name (blank: destination name)", "Destination (blank: project name)", "Python package (blank: derive from name)"}
	s := "INITIALIZE PYTHON PROJECT\n\n"
	for i, input := range inputs {
		prefix := "  "
		if i == focus {
			prefix = "> "
		}
		s += prefix + labels[i] + "\n  " + input + "\n\n"
	}
	for i, item := range []string{fmt.Sprintf("[%s] Replace conflicting files (--force)", mark(force)), fmt.Sprintf("[%s] Preview only (--dry-run)", mark(dryRun)), "Preview project"} {
		prefix := "  "
		if focus == i+3 {
			prefix = "> "
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
