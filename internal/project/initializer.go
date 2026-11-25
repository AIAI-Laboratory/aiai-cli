package project

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

// Planner converts an InitRequest into a validated, deterministic scaffold Plan.
type Planner interface {
	Plan(context.Context, InitRequest) (scaffold.Plan, error)
}

// Initializer validates project inputs and delegates file planning to scaffold.Engine.
type Initializer struct{ Engine scaffold.Engine }

// Plan validates destination and metadata for the requested template, composing the
// default agent setup kit when IncludeAgent is enabled on a project template.
func (i Initializer) Plan(ctx context.Context, r InitRequest) (scaffold.Plan, error) {
	if r.TemplateID == "" {
		r.TemplateID = templates.PythonMinimalID
	}
	if r.TargetDir == "" {
		r.TargetDir = "."
	}
	target, err := platform.ResolveTarget(r.TargetDir)
	if err != nil {
		return scaffold.Plan{}, fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
	}
	tmpl, err := i.Engine.Registry.Get(r.TemplateID)
	if err != nil {
		return scaffold.Plan{}, fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
	}
	if tmpl.Manifest.Kind == templates.KindAgent {
		return i.Engine.PlanWithOptions(ctx, r.TemplateID, target, scaffold.Variables{}, scaffold.PlanOptions{
			Force:               r.Force,
			Components:          r.Components,
			ComponentsSpecified: r.ComponentsSpecified,
			AgentsMDMode:        r.AgentsMDMode,
		})
	}
	if r.ProjectName == "" {
		r.ProjectName = filepath.Base(target)
	}
	name, err := NormalizeName(r.ProjectName)
	if err != nil {
		return scaffold.Plan{}, fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
	}
	pkg := r.PackageName
	if pkg == "" {
		pkg, err = PackageName(name)
	} else {
		err = ValidatePackage(pkg)
	}
	if err != nil {
		return scaffold.Plan{}, fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
	}
	opts := scaffold.PlanOptions{
		Force:        r.Force,
		AgentsMDMode: r.AgentsMDMode,
	}
	if r.IncludeAgent {
		opts.ExtraTemplateID = templates.AgentKitID
		opts.ExtraComponents = r.Components
		opts.ExtraComponentsSpecified = r.ComponentsSpecified
	}
	return i.Engine.PlanWithOptions(ctx, r.TemplateID, target, scaffold.Variables{ProjectName: name, PackageName: pkg}, opts)
}
