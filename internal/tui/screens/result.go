package screens

import (
	"fmt"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func Result(r scaffold.Result, err error, t Theme) string {
	var s strings.Builder
	if err != nil {
		s.WriteString(t.Accent("! Generation stopped") + "\n" + t.Truncate(err.Error(), t.Width()) + "\n\n")
	} else {
		s.WriteString(t.Good("✓ Project ready") + "\n\n")
	}
	s.WriteString(t.Truncate(r.TargetDir, t.Width()) + "\n" + t.Muted(fmt.Sprintf("%d files written · %d unchanged", len(r.Completed), len(r.Skipped))) + "\n\n")
	if len(r.NextSteps) > 0 {
		s.WriteString(t.Title("Next steps") + "\n" + t.Muted("Run inside "+t.Truncate(r.TargetDir, max(1, t.Width()-12))) + "\n\n")
		for _, step := range r.NextSteps {
			s.WriteString("  " + t.Accent("❯") + " " + t.Truncate(step, max(1, t.Width()-4)) + "\n")
		}
		s.WriteString("\n" + t.Muted("Commit uv.lock and pnpm-lock.yaml before pushing.") + "\n\n")
	}
	if len(r.Completed) > 0 {
		s.WriteString(t.Title("Files written") + "\n")
		for _, path := range r.Completed {
			s.WriteString("  " + t.Good("+") + " " + t.Truncate(path, max(1, t.Width()-4)) + "\n")
		}
	}
	return s.String()
}
