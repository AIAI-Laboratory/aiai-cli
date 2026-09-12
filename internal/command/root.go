package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui"
)

type Dependencies struct {
	Planner  project.Planner
	Executor scaffold.Executor
	Registry templates.TemplateRegistry
	In       io.Reader
	Out      io.Writer
	Err      io.Writer
	Version  string
	// Interactive can be injected by tests; production uses actual descriptors.
	Interactive func() bool
}

type state struct {
	deps    Dependencies
	json    bool
	noColor bool
	verbose bool
	plan    *scaffold.Plan
	result  *scaffold.Result
	version string
	status  string
}

func Run(ctx context.Context, args []string, deps Dependencies) int {
	s := &state{deps: deps, status: "ok"}
	root := s.root()
	// Detect machine output even when flag parsing stops at an unknown option.
	s.json = jsonRequested(args)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	code := ExitCode(err)
	if s.json {
		e := output.Envelope{Status: s.status, Plan: s.plan, Result: s.result, Version: s.version}
		if err != nil {
			e.Status = "error"
			e.Error = &output.Error{Code: code, Message: err.Error()}
		}
		if writeErr := output.JSON(deps.Out, e); writeErr != nil {
			fmt.Fprintln(deps.Err, writeErr)
			return 4
		}
	} else if err != nil {
		if s.result != nil {
			fmt.Fprint(deps.Err, output.ResultText(*s.result))
		}
		fmt.Fprintln(deps.Err, "Error:", err)
	}
	return code
}

func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 130
	case errors.Is(err, scaffold.ErrConflict):
		return 3
	case errors.Is(err, scaffold.ErrExecution):
		return 4
	default:
		return 2
	}
}

func (s *state) interactive() bool {
	if s.deps.Interactive != nil {
		return s.deps.Interactive()
	}
	return platform.Interactive(s.deps.In, s.deps.Out)
}

func (s *state) human() io.Writer {
	if s.json {
		return s.deps.Err
	}
	return s.deps.Out
}

func (s *state) root() *cobra.Command {
	root := &cobra.Command{
		Use:           "aiai",
		Short:         "Create projects from deterministic, offline templates",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !s.interactive() {
				if s.json {
					return fmt.Errorf("specify a command, for example: aiai init python demo --yes --json")
				}
				return cmd.Help()
			}
			return s.wizard(cmd.Context(), project.InitRequest{}, false, false)
		},
	}
	root.SetIn(s.deps.In)
	root.SetOut(s.deps.Out)
	root.SetErr(s.deps.Err)
	root.PersistentFlags().BoolVar(&s.json, "json", false, "Emit one JSON envelope (schema version 1)")
	root.PersistentFlags().BoolVar(&s.noColor, "no-color", false, "Disable terminal colors")
	root.PersistentFlags().BoolVar(&s.verbose, "verbose", false, "Write diagnostic logs to stderr")
	root.AddCommand(s.initCommand(), s.versionCommand())
	help := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		s.status = "help"
		cmd.SetOut(s.human())
		help(cmd, args)
	})
	return root
}

func jsonRequested(args []string) bool {
	requested := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		switch arg {
		case "--name", "--target", "--package":
			i++
		case "--json":
			requested = true
		default:
			if strings.HasPrefix(arg, "--json=") {
				requested = strings.TrimPrefix(arg, "--json=") == "true"
			}
		}
	}
	return requested
}

func (s *state) wizard(ctx context.Context, req project.InitRequest, dryRun, startInit bool) error {
	if !s.interactive() {
		return fmt.Errorf("interactive input is unavailable; use aiai init python <name-or-dot> --yes")
	}
	opts := tui.Options{Request: req, DryRun: dryRun, StartInit: startInit, NoColor: s.noColor || os.Getenv("NO_COLOR") != "", Version: s.deps.Version}
	result, plan, err := tui.Run(ctx, s.deps.Planner, s.deps.Executor, s.deps.Registry.List(), s.deps.In, s.human(), opts)
	s.plan, s.result = plan, result
	if result != nil && err == nil && !s.json {
		fmt.Fprint(s.human(), output.ResultText(*result))
	}
	if result == nil && plan != nil {
		s.status = "planned"
	}
	return err
}

func (s *state) log(message string, args ...any) {
	if s.verbose {
		slog.New(slog.NewTextHandler(s.deps.Err, nil)).Info(message, args...)
	}
}
