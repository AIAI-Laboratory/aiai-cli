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
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

func (s *state) initCommand() *cobra.Command {
	var name, pkg, target, componentsRaw, agentsMDRaw string
	var dryRun, yes, force bool
	init := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project or Agent Setup pack (AGENTS.md, rules, skills, docs)",
		Args:  cobra.NoArgs,
	}
	init.PersistentFlags().StringVar(&name, "name", "", "Project metadata name (defaults to destination basename)")
	init.PersistentFlags().StringVar(&pkg, "package", "", "Python package name (defaults to normalized project name)")
	init.PersistentFlags().StringVar(&target, "target", "", "Destination directory (overrides positional destination)")
	init.PersistentFlags().StringVar(&componentsRaw, "components", "", "Comma-separated Agent Setup components (default: all)")
	init.PersistentFlags().StringVar(&agentsMDRaw, "agents-md", "", "Existing AGENTS.md policy: append (default), replace, or skip")
	init.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview without creating files; does not require --yes")
	init.PersistentFlags().BoolVar(&yes, "yes", false, "Authorize generation in non-interactive mode")
	init.PersistentFlags().BoolVar(&force, "force", false, "Replace conflicting regular files listed in the plan (protected skills and rules are never overwritten)")
	init.RunE = func(cmd *cobra.Command, _ []string) error {
		agentsMode, err := scaffold.ParseExistingMode(agentsMDRaw)
		if err != nil {
			return fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
		}
		comps, specified := parseComponentsFlag(componentsRaw)
		return s.wizard(cmd.Context(), project.InitRequest{
			ProjectName:         name,
			PackageName:         pkg,
			TargetDir:           target,
			Force:               force,
			Components:          comps,
			ComponentsSpecified: specified,
			IncludeAgent:        true,
			AgentsMDMode:        agentsMode,
		}, dryRun, true)
	}

	agent := &cobra.Command{
		Use:   "agent [target]",
		Short: "Set up AGENTS.md, rules, skills, and GitHub Flow docs in a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentsMode, err := scaffold.ParseExistingMode(agentsMDRaw)
			if err != nil {
				return fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
			}
			comps, specified := parseComponentsFlag(componentsRaw)
			destination := target
			if destination == "" {
				if len(args) > 0 {
					destination = args[0]
				} else {
					destination = "."
				}
			}
			req := project.InitRequest{
				TemplateID:          templates.AgentKitID,
				TargetDir:           destination,
				Force:               force,
				Components:          comps,
				ComponentsSpecified: specified,
				AgentsMDMode:        agentsMode,
			}
			return s.initialize(cmd.Context(), req, dryRun, yes, cmd.Flags().Changed("agents-md"))
		},
	}

	var noAgent bool
	python := &cobra.Command{
		Use:   "python [name-or-dot]",
		Short: "Generate a minimal Python 3.13+ project with uv, pre-commit, and optional Agent Setup",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentsMode, err := scaffold.ParseExistingMode(agentsMDRaw)
			if err != nil {
				return fmt.Errorf("%w: %v", scaffold.ErrValidation, err)
			}
			comps, specified := parseComponentsFlag(componentsRaw)
			if len(args) == 0 && name == "" && target == "" {
				return s.wizard(cmd.Context(), project.InitRequest{
					TemplateID:          templates.PythonMinimalID,
					PackageName:         pkg,
					Force:               force,
					Components:          comps,
					ComponentsSpecified: specified,
					IncludeAgent:        !noAgent,
					AgentsMDMode:        agentsMode,
				}, dryRun, true)
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
			req := project.InitRequest{
				TemplateID:          templates.PythonMinimalID,
				ProjectName:         projectName,
				PackageName:         pkg,
				TargetDir:           destination,
				Force:               force,
				Components:          comps,
				ComponentsSpecified: specified,
				IncludeAgent:        !noAgent,
				AgentsMDMode:        agentsMode,
			}
			return s.initialize(cmd.Context(), req, dryRun, yes, cmd.Flags().Changed("agents-md"))
		},
	}
	python.Flags().BoolVar(&noAgent, "no-agent", false, "Skip default Agent Setup files (AGENTS.md, rules, skills, and docs)")

	init.AddCommand(agent, python)
	return init
}

func (s *state) initialize(ctx context.Context, req project.InitRequest, dryRun, yes, agentsModeExplicit bool) error {
	var reader *bufio.Reader
	if s.interactive() {
		reader = bufio.NewReader(s.deps.In)
		if !yes && !agentsModeExplicit && requestsAgentsMD(req) {
			exists, err := scaffold.HasExistingFile(req.TargetDir, "AGENTS.md")
			if err == nil && exists {
				fmt.Fprintf(s.human(), "AGENTS.md already exists in %s. [A]ppend (default) / [r]eplace / [s]kip: ", req.TargetDir)
				mode, err := promptExistingMode(ctx, reader)
				if err != nil {
					return err
				}
				req.AgentsMDMode = mode
			}
		}
	}
	s.log("planning project", "target", req.TargetDir)
	p, err := s.deps.Planner.Plan(ctx, req)
	if err != nil {
		return err
	}
	s.plan = &p
	if !s.json || s.interactive() {
		fmt.Fprint(s.human(), output.PlanText(p))
	}
	if dryRun {
		if p.HasConflicts() {
			return fmt.Errorf("%w: existing files differ; inspect the preview and use --force to replace them", scaffold.ErrConflict)
		}
		s.status = "planned"
		return nil
	}
	if !s.interactive() {
		if p.HasConflicts() {
			return fmt.Errorf("%w: existing files differ; inspect the preview and use --force to replace them", scaffold.ErrConflict)
		}
		if !yes {
			return fmt.Errorf("non-interactive generation requires --yes; use --dry-run to preview")
		}
	} else if !yes {
		switch {
		case p.HasConflicts():
			fmt.Fprint(s.human(), "\nConflicting files exist. [S]kip conflicts (default) / [r]eplace conflicts / [c]ancel: ")
			replace, err := promptConflictResolution(ctx, reader)
			if err != nil {
				return err
			}
			p = p.ResolveExisting(replace)
			s.plan = &p
		case p.HasOverwrites():
			fmt.Fprint(s.human(), "\nReplace existing files listed above? [y/N] ")
			confirmed, err := confirmReader(ctx, reader)
			if err != nil {
				return err
			}
			if !confirmed {
				return context.Canceled
			}
		}
	} else if p.HasConflicts() {
		return fmt.Errorf("%w: existing files differ; inspect the preview and use --force to replace them", scaffold.ErrConflict)
	}
	s.log("applying project plan", "files", len(p.Operations))
	r, err := s.deps.Executor.Apply(ctx, p)
	s.result = &r
	if err == nil && !s.json {
		fmt.Fprint(s.human(), "\n"+output.ResultText(r))
	}
	return err
}

func parseComponentsFlag(raw string) ([]string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out, true
}

func requestsAgentsMD(req project.InitRequest) bool {
	if req.TemplateID != templates.AgentKitID && !req.IncludeAgent {
		return false
	}
	if !req.ComponentsSpecified {
		return true
	}
	for _, c := range req.Components {
		if strings.EqualFold(c, "all") || c == "agents-md" {
			return true
		}
	}
	return false
}

func readLine(ctx context.Context, r *bufio.Reader) (string, error) {
	line, _, err := readLineEOF(ctx, r)
	return line, err
}

func readLineEOF(ctx context.Context, r *bufio.Reader) (string, bool, error) {
	type answer struct {
		line string
		err  error
	}
	ch := make(chan answer, 1)
	go func() { line, err := r.ReadString('\n'); ch <- answer{line, err} }()
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case a := <-ch:
		if a.err != nil && a.err != io.EOF {
			return "", false, fmt.Errorf("read confirmation: %w", a.err)
		}
		trimmed := strings.ToLower(strings.TrimSpace(a.line))
		return trimmed, a.err == io.EOF && trimmed == "", nil
	}
}

func confirm(ctx context.Context, in io.Reader) (bool, error) {
	return confirmReader(ctx, bufio.NewReader(in))
}

func confirmReader(ctx context.Context, r *bufio.Reader) (bool, error) {
	line, err := readLine(ctx, r)
	if err != nil {
		return false, err
	}
	return line == "y" || line == "yes", nil
}

func promptExistingMode(ctx context.Context, r *bufio.Reader) (scaffold.ExistingMode, error) {
	line, err := readLine(ctx, r)
	if err != nil {
		return "", err
	}
	switch line {
	case "", "a", "append":
		return scaffold.ExistingAppend, nil
	case "r", "replace":
		return scaffold.ExistingReplace, nil
	case "s", "skip":
		return scaffold.ExistingSkip, nil
	default:
		return "", fmt.Errorf("%w: invalid choice %q (expected append, replace, or skip)", scaffold.ErrValidation, line)
	}
}

func promptConflictResolution(ctx context.Context, r *bufio.Reader) (bool, error) {
	line, eof, err := readLineEOF(ctx, r)
	if err != nil {
		return false, err
	}
	if eof {
		return false, context.Canceled
	}
	switch line {
	case "", "s", "skip":
		return false, nil
	case "r", "replace":
		return true, nil
	case "c", "cancel":
		return false, context.Canceled
	default:
		return false, fmt.Errorf("%w: invalid choice %q (expected skip, replace, or cancel)", scaffold.ErrValidation, line)
	}
}
