package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func Preview(p scaffold.Plan, dryRun bool, t Theme) string {
	var s strings.Builder
	s.WriteString(t.Heading("Review your project", p.TemplateID+" v"+p.TemplateVersion))
	s.WriteString(t.Muted("Destination  ") + t.Truncate(p.TargetDir, max(1, t.Width()-14)) + "\n\n")
	counts := make(map[scaffold.Action]int)
	for _, op := range p.Operations {
		counts[op.Action]++
	}
	create := t.Good(fmt.Sprintf("%d create", counts[scaffold.Create]))
	replace := t.Warn(fmt.Sprintf("%d replace", counts[scaffold.Overwrite]))
	unchanged := t.Muted(fmt.Sprintf("%d unchanged", counts[scaffold.Skip]))
	conflicts := t.Accent(fmt.Sprintf("%d conflicts", counts[scaffold.Conflict]))
	if t.Width() < 58 {
		s.WriteString(create + " · " + replace + "\n" + unchanged + " · " + conflicts + "\n\n")
	} else {
		s.WriteString(create + " · " + replace + " · " + unchanged + " · " + conflicts + "\n\n")
	}
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
		s.WriteString("  " + style(label) + " " + t.Truncate(op.Path, max(1, t.Width()-15)) + "\n")
	}
	for _, warning := range p.Warnings {
		s.WriteString("\n" + t.Warn("! "+t.Truncate(warning, max(1, t.Width()-2))) + "\n")
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
