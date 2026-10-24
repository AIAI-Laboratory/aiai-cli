package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/monitor"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	"github.com/AIAI-Laboratory/aiai-cli/internal/project"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
	"github.com/AIAI-Laboratory/aiai-cli/internal/tui"
	devtui "github.com/AIAI-Laboratory/aiai-cli/internal/tui/developer"
	"github.com/AIAI-Laboratory/aiai-cli/internal/updater"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

type Dependencies struct {
	Planner   project.Planner
	Executor  scaffold.Executor
	Registry  templates.TemplateRegistry
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
	Version   string
	Auth      auth.Authenticator
	Updater   updater.Service
	SSH       sshmanager.Manager
	Zen       zen.Repository
	Monitor   monitor.Collector
	Git       *gitlocal.Service
	Workspace *workspace.Service
	// Interactive can be injected by tests; production uses actual descriptors.
	Interactive func() bool
}

type state struct {
	deps                          Dependencies
	json                          bool
	noColor                       bool
	verbose                       bool
	plan                          *scaffold.Plan
	result                        *scaffold.Result
	version                       string
	status                        string
	auth                          *output.Auth
	update                        *output.Update
	ssh                           *output.SSH
	zen                           *output.Zen
	system                        *monitor.Snapshot
	git                           any
	workspace                     any
	zenProject, zenRepo, zenLabel string
}

func Run(ctx context.Context, args []string, deps Dependencies) int {
	if deps.Git == nil {
		deps.Git = &gitlocal.Service{}
	}
	if deps.Workspace == nil {
		deps.Workspace, _ = workspace.New("", deps.Git, deps.Zen)
	}
	s := &state{deps: deps, status: "ok"}
	root := s.root()
	// Detect machine output even when flag parsing stops at an unknown option.
	s.json = jsonRequested(args)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	code := ExitCode(err)
	if s.json {
		e := output.Envelope{Status: s.status, Plan: s.plan, Result: s.result, Version: s.version, Auth: s.auth, Update: s.update, SSH: s.ssh, Zen: s.zen, System: s.system, Git: s.git, Workspace: s.workspace}
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
	var processExit interface{ ExitCode() int }
	switch {
	case err == nil:
		return 0
	case errors.As(err, &processExit):
		return processExit.ExitCode()
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 130
	case errors.Is(err, scaffold.ErrConflict):
		return 3
	case errors.Is(err, scaffold.ErrExecution):
		return 4
	case errors.Is(err, auth.ErrIO):
		return 4
	case errors.Is(err, sshmanager.ErrStorage), errors.Is(err, sshmanager.ErrSSHMissing):
		return 4
	case errors.Is(err, zen.ErrStorage):
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
		Short:         "Create projects and manage local SSH connections",
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
	root.AddCommand(s.initCommand(), s.versionCommand(), s.loginCommand(), s.whoamiCommand(), s.logoutCommand(), s.updateCommand(), s.serverCommand(), s.sshCommand(), s.logCommand(), s.keyCommand(), s.zenCommand(), s.monitorCommand(), s.systemCommand(), s.gitCommand(), s.workspaceCommand())
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
	opts := tui.Options{Request: req, DryRun: dryRun, StartInit: startInit, NoColor: s.noColor || os.Getenv("NO_COLOR") != "", Version: s.deps.Version, SSH: s.deps.SSH, Workspace: s.deps.Workspace}
	dir, _ := os.Getwd()
	opts.Terminal = &platform.ShellSession{Dir: dir}
	opts.Session = &tui.Session{}
	opts.Context = func(ctx context.Context, dir string) tui.ProjectContext {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		st, gitErr := s.deps.Git.Status(ctx, dir)
		name := filepath.Base(dir)
		if gitErr == nil {
			name = st.Name
		}
		if s.deps.Workspace != nil {
			items, e := s.deps.Workspace.List(ctx, false)
			if e == nil {
				for _, p := range items {
					if p.Root == dir || gitErr == nil && p.Root == st.Root {
						name = p.Name
						break
					}
				}
			}
		}
		result := tui.ProjectContext{Project: safe(name)}
		if gitErr == nil {
			result.Branch, result.Changes = safe(branchLabel(st)), len(st.Changes)
		}
		return result
	}
	result, plan, action, err := tui.Run(ctx, s.deps.Planner, s.deps.Executor, s.deps.Registry.List(), s.deps.Auth, s.deps.Updater, s.deps.In, s.human(), opts)
	for err == nil && (action.Monitor || action.Git || action.Workspace || action.Zen || action.SSHAlias != "" || action.GenerateKey != nil) {
		opts.Notice, err = s.homeAction(ctx, action)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			opts.Notice = "Could not complete the operation: " + err.Error()
		}
		result, plan, action, err = tui.Run(ctx, s.deps.Planner, s.deps.Executor, s.deps.Registry.List(), s.deps.Auth, s.deps.Updater, s.deps.In, s.human(), opts)
	}
	s.plan, s.result = plan, result
	if result != nil && err == nil && !s.json {
		fmt.Fprint(s.human(), output.ResultText(*result))
	}
	if result == nil && plan != nil {
		s.status = "planned"
	}
	return err
}

func (s *state) homeAction(ctx context.Context, action tui.Action) (string, error) {
	switch {
	case action.Monitor:
		return "", s.runMonitor(ctx, time.Second)
	case action.Git:
		return "", s.runDeveloper(ctx, devtui.Options{Mode: "git", Path: "."})
	case action.Workspace:
		return "", s.runDeveloper(ctx, devtui.Options{})
	case action.Zen:
		return "", s.runZen(ctx, 0, zen.Config{})
	case action.SSHAlias != "":
		err := s.deps.SSH.Connect(ctx, action.SSHAlias, sshmanager.Streams{In: s.deps.In, Out: s.deps.Out, Err: s.deps.Err})
		return "SSH session ended: " + action.SSHAlias, err
	case action.GenerateKey != nil:
		key, err := s.deps.SSH.GenerateKey(ctx, *action.GenerateKey, sshmanager.Streams{In: s.deps.In, Out: s.human(), Err: s.deps.Err})
		return fmt.Sprintf("SSH key generated: %s\nFingerprint: %s\nPublic key: %s\nOpen /keys to view or copy it.", key.Name, key.Fingerprint, key.PublicPath), err
	}
	return "", nil
}

func (s *state) log(message string, args ...any) {
	if s.verbose {
		slog.New(slog.NewTextHandler(s.deps.Err, nil)).Info(message, args...)
	}
}
