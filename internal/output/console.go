package output

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

// PlanText formats a scaffold Plan as human-readable text for terminal output.
func PlanText(p scaffold.Plan) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s v%s → %s\n\n", p.TemplateID, p.TemplateVersion, p.TargetDir)
	for _, op := range p.Operations {
		if op.Reason != "" {
			fmt.Fprintf(&s, "  %-9s %s (%s)\n", op.Action, op.Path, op.Reason)
		} else {
			fmt.Fprintf(&s, "  %-9s %s\n", op.Action, op.Path)
		}
	}
	for _, warning := range p.Warnings {
		fmt.Fprintf(&s, "\n%s\n", warning)
	}
	return s.String()
}

// ResultText formats a scaffold Result as human-readable text including next steps.
func ResultText(r scaffold.Result) string {
	var s strings.Builder
	if len(r.Protected) > 0 {
		fmt.Fprintf(&s, "%d files written, %d unchanged, %d protected in %s\n", len(r.Completed), len(r.Skipped), len(r.Protected), r.TargetDir)
	} else {
		fmt.Fprintf(&s, "%d files written, %d unchanged in %s\n", len(r.Completed), len(r.Skipped), r.TargetDir)
	}
	for _, path := range r.Completed {
		fmt.Fprintf(&s, "  wrote %s\n", path)
	}
	for _, path := range r.Protected {
		fmt.Fprintf(&s, "  protected %s (kept existing)\n", path)
	}
	if len(r.NextSteps) > 0 {
		fmt.Fprintf(&s, "\nRun these commands inside %s:\n\n", r.TargetDir)
		for _, step := range r.NextSteps {
			fmt.Fprintf(&s, "  %s\n", step)
		}
		if r.NextStepsNote != "" {
			fmt.Fprintf(&s, "\n%s\n", r.NextStepsNote)
		}
	} else if r.NextStepsNote != "" {
		fmt.Fprintf(&s, "\n%s\n", r.NextStepsNote)
	}
	return s.String()
}
