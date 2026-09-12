package screens

import (
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func Preview(p scaffold.Plan, dryRun bool) string {
	s := "PLAN PREVIEW\n\n" + output.PlanText(p)
	if p.HasConflicts() {
		return s + "\nConflicts block generation. Esc to edit options.\n"
	}
	if dryRun {
		return s + "\nPreview only. No files written. Enter to finish; Esc to edit.\n"
	}
	return s + "\nEnter to continue to confirmation. Esc to edit.\n"
}
