package output

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func PlanText(p scaffold.Plan) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s v%s → %s\n\n", p.TemplateID, p.TemplateVersion, p.TargetDir)
	for _, op := range p.Operations {
		fmt.Fprintf(&s, "  %-9s %s\n", op.Action, op.Path)
	}
	for _, warning := range p.Warnings {
		fmt.Fprintf(&s, "\n%s\n", warning)
	}
	return s.String()
}

func ResultText(r scaffold.Result) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%d files written, %d unchanged in %s\n", len(r.Completed), len(r.Skipped), r.TargetDir)
	for _, path := range r.Completed {
		fmt.Fprintf(&s, "  wrote %s\n", path)
	}
	if len(r.NextSteps) > 0 {
		fmt.Fprintf(&s, "\nRun these commands inside %s:\n\n", r.TargetDir)
		for _, step := range r.NextSteps {
			fmt.Fprintf(&s, "  %s\n", step)
		}
		s.WriteString("\nCommit uv.lock and pnpm-lock.yaml before pushing.\n")
	}
	return s.String()
}
