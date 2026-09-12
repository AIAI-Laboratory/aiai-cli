package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func Preview(p scaffold.Plan, dryRun bool, t Theme) string {
	var s strings.Builder
	s.WriteString(t.Heading("Review your project", p.TemplateID+" v"+p.TemplateVersion))
	s.WriteString(t.Muted("Destination  ") + p.TargetDir + "\n\n")
	counts := make(map[scaffold.Action]int)
	for _, op := range p.Operations {
		counts[op.Action]++
	}
	s.WriteString(t.Good(fmt.Sprintf("%d create", counts[scaffold.Create])) + "  ·  " +
		t.Warn(fmt.Sprintf("%d replace", counts[scaffold.Overwrite])) + "  ·  " +
		t.Muted(fmt.Sprintf("%d unchanged", counts[scaffold.Skip])) + "  ·  " +
		t.Accent(fmt.Sprintf("%d conflicts", counts[scaffold.Conflict])) + "\n\n")
	for _, op := range p.Operations {
		label := fmt.Sprintf("%-11s", "+ create")
		style := t.Good
		switch op.Action {
		case scaffold.Overwrite:
			label, style = fmt.Sprintf("%-11s", "~ replace"), t.Warn
		case scaffold.Skip:
			label, style = fmt.Sprintf("%-11s", "= unchanged"), t.Muted
		case scaffold.Conflict:
			label, style = fmt.Sprintf("%-11s", "! conflict"), t.Accent
		}
		s.WriteString("  " + style(label) + " " + op.Path + "\n")
	}
	for _, warning := range p.Warnings {
		s.WriteString("\n" + t.Warn("! "+warning) + "\n")
	}
	if p.HasConflicts() {
		s.WriteString("\n" + t.Accent("Conflicts block generation.") + " Esc to edit options.\n")
	} else if dryRun {
		s.WriteString("\n" + t.Muted("Preview only. No files written. Enter to finish.") + "\n")
	} else {
		s.WriteString("\n" + t.Muted("Enter to review the final confirmation.") + "\n")
	}
	return s.String()
}
