package output

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	"github.com/AIAI-Laboratory/aiai-cli/internal/skill"
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

func SkillText(result skill.Result) string {
	var s strings.Builder
	switch result.Action {
	case "available":
		fmt.Fprintf(&s, "Available skills from %s:\n", skill.DefaultRepo)
	case "list":
		fmt.Fprintf(&s, "Installed %s skills in %s:\n", result.Scope, result.Destination)
	case "install":
		fmt.Fprintf(&s, "Installed %d skill(s) in %s:\n", len(result.Skills), result.Destination)
	case "update":
		fmt.Fprintf(&s, "Updated %d skill(s) in %s:\n", len(result.Skills), result.Destination)
	case "remove":
		fmt.Fprintf(&s, "Removed %d skill(s) from %s:\n", len(result.Skills), result.Destination)
	}
	if len(result.Skills) == 0 {
		s.WriteString("  none\n")
		return s.String()
	}
	for _, item := range result.Skills {
		fmt.Fprintf(&s, "  %s", item.Name)
		if item.Description != "" {
			fmt.Fprintf(&s, " — %s", strings.Join(strings.Fields(item.Description), " "))
		}
		s.WriteByte('\n')
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
