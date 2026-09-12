package app

import (
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/command"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

func dependencies(in io.Reader, out, errOut io.Writer, version string) (command.Dependencies, error) {
	r, err := templates.Embedded()
	if err != nil {
		return command.Dependencies{}, err
	}
	return command.Dependencies{Planner: project.Initializer{Engine: scaffold.Engine{Registry: r}}, Executor: scaffold.FileExecutor{}, Registry: r, In: in, Out: out, Err: errOut, Version: version}, nil
}
