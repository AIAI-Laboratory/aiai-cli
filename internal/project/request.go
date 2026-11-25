package project

import "github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"

// InitRequest captures user-provided inputs and options for planning project or agent scaffolding.
type InitRequest struct {
	TemplateID          string
	TargetDir           string
	ProjectName         string
	PackageName         string
	Force               bool
	Components          []string
	ComponentsSpecified bool
	IncludeAgent        bool
	AgentsMDMode        scaffold.ExistingMode
}
