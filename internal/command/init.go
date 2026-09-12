package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func (s *state) initCommand() *cobra.Command {
	var name, pkg, target string
	var dryRun, yes, force bool
	init := &cobra.Command{Use: "init", Short: "Initialize a project", Args: cobra.NoArgs}
	init.PersistentFlags().StringVar(&name, "name", "", "Project metadata name (defaults to destination basename)")
	init.PersistentFlags().StringVar(&pkg, "package", "", "Python package name (defaults to normalized project name)")
	init.PersistentFlags().StringVar(&target, "target", "", "Destination directory (overrides positional destination)")
	init.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview without creating files; does not require --yes")
	init.PersistentFlags().BoolVar(&yes, "yes", false, "Authorize generation in non-interactive mode")
	init.PersistentFlags().BoolVar(&force, "force", false, "Replace conflicting regular files listed in the plan")
	init.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.wizard(cmd.Context(), project.InitRequest{ProjectName: name, PackageName: pkg, TargetDir: target, Force: force}, dryRun, true)
	}
	python := &cobra.Command{
		Use:   "python [name-or-dot]",
		Short: "Generate a minimal Python 3.13+ project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && name == "" && target == "" {
				return s.wizard(cmd.Context(), project.InitRequest{PackageName: pkg, Force: force}, dryRun, true)
			}
			projectName := name
			destination := target
			if len(args) > 0 {
				arg := args[0]
				if arg == "." {
					if destination == "" {
						destination = "."
					}
				} else {
					normalized, err := project.NormalizeName(arg)
					if err != nil {
						return err
					}
					if destination == "" {
						destination = normalized
					}
					if projectName == "" {
						projectName = normalized
					}
				}
			} else if destination == "" {
				var err error
				destination, err = project.NormalizeName(projectName)
				if err != nil {
					return err
				}
			}
			return s.initialize(cmd.Context(), project.InitRequest{TemplateID: "python-minimal", ProjectName: projectName, PackageName: pkg, TargetDir: destination, Force: force}, dryRun, yes)
		},
	}
	init.AddCommand(python)
	return init
}

func (s *state) initialize(ctx context.Context, req project.InitRequest, dryRun, yes bool) error {
	s.log("planning project", "target", req.TargetDir)
	p, err := s.deps.Planner.Plan(ctx, req)
	if err != nil {
		return err
	}
	s.plan = &p
	if !s.json || s.interactive() {
		fmt.Fprint(s.human(), output.PlanText(p))
	}
	if p.HasConflicts() {
		return fmt.Errorf("%w: existing files differ; inspect the preview and use --force to replace them", scaffold.ErrConflict)
	}
	if dryRun {
		s.status = "planned"
		return nil
	}
	if s.interactive() {
		fmt.Fprint(s.human(), "\nApply this plan? [y/N] ")
		confirmed, err := confirm(ctx, s.deps.In)
		if err != nil {
			return err
		}
		if !confirmed {
			return context.Canceled
		}
	} else if !yes {
		return fmt.Errorf("non-interactive generation requires --yes; use --dry-run to preview")
	}
	s.log("applying project plan", "files", len(p.Operations))
	r, err := s.deps.Executor.Apply(ctx, p)
	s.result = &r
	if err == nil && !s.json {
		fmt.Fprint(s.human(), "\n"+output.ResultText(r))
	}
	return err
}

func confirm(ctx context.Context, in io.Reader) (bool, error) {
	type answer struct {
		line string
		err  error
	}
	ch := make(chan answer, 1)
	go func() { line, err := bufio.NewReader(in).ReadString('\n'); ch <- answer{line, err} }()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case a := <-ch:
		if a.err != nil && a.err != io.EOF {
			return false, fmt.Errorf("read confirmation: %w", a.err)
		}
		line := strings.ToLower(strings.TrimSpace(a.line))
		return line == "y" || line == "yes", nil
	}
}
