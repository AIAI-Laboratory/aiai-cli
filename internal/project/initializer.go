package project

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

type Planner interface {
	Plan(context.Context, InitRequest) (scaffold.Plan, error)
}

type Initializer struct{ Engine scaffold.Engine }

func (i Initializer) Plan(ctx context.Context, r InitRequest) (scaffold.Plan, error) {
	if r.TemplateID == "" {
		r.TemplateID = "python-minimal"
	}
	if r.TargetDir == "" {
		r.TargetDir = "."
	}
	target, err := platform.ResolveTarget(r.TargetDir)
	if err != nil {
		return scaffold.Plan{}, fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
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
	return i.Engine.Plan(ctx, r.TemplateID, target, scaffold.Variables{ProjectName: name, PackageName: pkg}, r.Force)
}
