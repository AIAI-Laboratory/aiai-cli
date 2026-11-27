package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

// Preview renders the Step 3 Create review screen with file operations, warnings,
// and inline conflict/overwrite resolution options when confirmation is required.
func Preview(p scaffold.Plan, confirmCursor int, dryRun bool, t Theme) string {
	var s strings.Builder
	title := "Review & create"
	switch {
	case dryRun:
		title = "Preview your plan"
	case p.HasConflicts():
		title = "Resolve conflicts & create"
	case p.HasOverwrites():
		title = "Confirm overwrites & create"
	}
	s.WriteString(t.Heading(title, p.TemplateID+" v"+p.TemplateVersion))
	s.WriteString(t.Muted("Destination  ") + t.Truncate(p.TargetDir, max(1, t.Width()-14)) + "\n\n")
	counts := make(map[scaffold.Action]int)
	for _, op := range p.Operations {
		counts[op.Action]++
	}
	parts := []string{
		t.Good(fmt.Sprintf("%d create", counts[scaffold.Create])),
	}
	if counts[scaffold.Append] > 0 {
		parts = append(parts, t.Good(fmt.Sprintf("%d append", counts[scaffold.Append])))
	}
	parts = append(parts,
		t.Warn(fmt.Sprintf("%d replace", counts[scaffold.Overwrite])),
		t.Muted(fmt.Sprintf("%d unchanged", counts[scaffold.Skip])),
	)
	if counts[scaffold.Protected] > 0 {
		parts = append(parts, t.Warn(fmt.Sprintf("%d protected", counts[scaffold.Protected])))
	}
	parts = append(parts, t.Accent(fmt.Sprintf("%d conflicts", counts[scaffold.Conflict])))

	if t.Width() < 58 && len(parts) > 2 {
		mid := (len(parts) + 1) / 2
		s.WriteString(strings.Join(parts[:mid], " · ") + "\n" + strings.Join(parts[mid:], " · ") + "\n\n")
	} else {
		s.WriteString(strings.Join(parts, " · ") + "\n\n")
	}

	for _, op := range p.Operations {
		label := fmt.Sprintf("%-11s", "+ create")
		style := t.Good
		switch op.Action {
		case scaffold.Append:
			label, style = fmt.Sprintf("%-11s", "++ append"), t.Good
		case scaffold.Overwrite:
			label, style = fmt.Sprintf("%-11s", "~ replace"), t.Warn
		case scaffold.Skip:
			label, style = fmt.Sprintf("%-11s", "= unchanged"), t.Muted
		case scaffold.Protected:
			label, style = fmt.Sprintf("%-11s", "⊘ protected"), t.Warn
		case scaffold.Conflict:
			label, style = fmt.Sprintf("%-11s", "! conflict"), t.Accent
		}
		s.WriteString("  " + style(label) + " " + t.Truncate(op.Path, max(1, t.Width()-15)) + "\n")
	}
	for _, warning := range p.Warnings {
		s.WriteString("\n" + t.Warn("! "+t.Truncate(warning, max(1, t.Width()-2))) + "\n")
	}
	if dryRun {
		if p.HasConflicts() {
			s.WriteString("\n" + t.Accent("Conflicts detected. Preview only; no files written.") + " Esc to back.\n")
		} else {
			s.WriteString("\n" + t.Muted("Preview only. No files written. Enter to finish.") + "\n")
		}
		return s.String()
	}
	if p.RequiresConfirmation() {
		s.WriteString("\n")
		conflicts := p.ConflictCount()
		overwrites := p.OverwriteCount()
		if conflicts > 0 {
			s.WriteString(t.Choice("Keep existing", fmt.Sprintf("Skip %d conflicting file(s) and create the rest", conflicts), confirmCursor == 0))
			s.WriteString(t.Choice("Overwrite", fmt.Sprintf("Replace %d conflicting file(s); skills/rules stay protected", conflicts), confirmCursor == 1))
		} else {
			s.WriteString(t.Choice("Overwrite", fmt.Sprintf("Replace %d existing file(s) as planned", overwrites), confirmCursor == 0))
			s.WriteString(t.Choice("Keep existing", fmt.Sprintf("Skip %d overwrite(s) and keep current files", overwrites), confirmCursor == 1))
		}
		s.WriteString(t.Choice("Back to configure", "Return to edit options without writing", confirmCursor == 2))
		return s.String()
	}
	s.WriteString("\n" + t.Muted("No conflicts. Enter to create · Esc to back.") + "\n")
	return s.String()
}
