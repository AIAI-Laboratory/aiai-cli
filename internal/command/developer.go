package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	devtui "github.com/AIAI-Laboratory/aiai-cli/internal/tui/developer"
	zentui "github.com/AIAI-Laboratory/aiai-cli/internal/tui/zen"
	"github.com/AIAI-Laboratory/aiai-cli/internal/workspace"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

func queryFlags(cmd *cobra.Command, f *gitlocal.Filter) {
	cmd.Flags().IntVar(&f.Limit, "limit", 100, "Maximum commits (1–1000)")
	cmd.Flags().StringVar(&f.Author, "author", "", "Explicit Git author pattern (all authors by default)")
	cmd.Flags().StringVar(&f.Branch, "branch", "", "Git revision/branch and session branch filter")
	var since, until string
	cmd.Flags().StringVar(&since, "since", "", "Start date YYYY-MM-DD or RFC3339")
	cmd.Flags().StringVar(&until, "until", "", "End date YYYY-MM-DD (inclusive) or RFC3339")
	cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		if f.Limit < 1 || f.Limit > 1000 {
			return errors.New("--limit must be 1–1000")
		}
		var err error
		f.Since, err = parseDate(since, false)
		if err != nil {
			return err
		}
		f.Until, err = parseDate(until, true)
		if err != nil {
			return err
		}
		if !f.Since.IsZero() && !f.Until.IsZero() && f.Until.Before(f.Since) {
			return errors.New("--until must not precede --since")
		}
		return nil
	}
}

func parseDate(value string, end bool) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}
	t, err = time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return t, fmt.Errorf("invalid date %q; use YYYY-MM-DD or RFC3339", value)
	}
	if end {
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return t, nil
}

func (s *state) gitCommand() *cobra.Command {
	path := "."
	cmd := &cobra.Command{Use: "git", Short: "Read-only local Git workspace", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if s.json || !s.interactive() {
			return s.gitRead(cmd.Context(), "status", path, "", false, gitlocal.Filter{})
		}
		return s.runDeveloper(cmd.Context(), devtui.Options{Mode: "git", Path: path})
	}}
	cmd.PersistentFlags().StringVar(&path, "repo", ".", "Local repository path")
	for _, mode := range []string{"status", "changes", "diff", "history", "branches", "worktrees", "activity"} {
		var file string
		var staged bool
		filter := gitlocal.Filter{}
		child := &cobra.Command{Use: mode, Short: "Show Git " + mode, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return s.gitRead(cmd.Context(), mode, path, file, staged, filter)
		}}
		if mode == "diff" {
			child.Flags().StringVar(&file, "file", "", "Repository-relative file")
			child.Flags().BoolVar(&staged, "staged", false, "Show index changes")
			child.Flags().Bool("interactive", false, "Open file list and diff preview")
			child.RunE = func(cmd *cobra.Command, _ []string) error {
				interactive, _ := cmd.Flags().GetBool("interactive")
				if interactive && !s.json {
					return s.runDeveloper(cmd.Context(), devtui.Options{Mode: "git", Path: path, Page: 1})
				}
				return s.gitRead(cmd.Context(), "diff", path, file, staged, filter)
			}
		}
		if mode == "history" || mode == "activity" {
			queryFlags(child, &filter)
		}
		cmd.AddCommand(child)
	}
	cmd.AddCommand(&cobra.Command{Use: "board", Short: "Registered Git repositories", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if s.json || !s.interactive() {
			return s.registryList(cmd.Context(), true)
		}
		return s.runDeveloper(cmd.Context(), devtui.Options{Repositories: true})
	}})
	repo := &cobra.Command{Use: "repo", Short: "Explicit repository registrations"}
	repo.AddCommand(s.registrationAdd(true), &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return s.registryList(cmd.Context(), true) }}, s.registrationRemove(true))
	cmd.AddCommand(repo)
	return cmd
}

func (s *state) gitRead(ctx context.Context, mode, path, file string, staged bool, f gitlocal.Filter) error {
	st, err := s.deps.Git.Status(ctx, path)
	if err != nil {
		return err
	}
	var data any
	switch mode {
	case "status":
		data = st
		if !s.json {
			fmt.Fprintf(s.human(), "%s  %s\n%s\nHEAD %s\n%d staged · %d unstaged · %d untracked · %d conflicts\n", safe(st.Name), safe(branchLabel(st)), safe(st.Root), st.Head, st.Staged, st.Unstaged, st.Untracked, st.Conflicts)
			if st.Upstream == "" {
				fmt.Fprintln(s.human(), "No upstream configured (local information only)")
			} else {
				fmt.Fprintf(s.human(), "%s  +%d / -%d (local tracking refs; no network check)\n", safe(st.Upstream), st.Ahead, st.Behind)
			}
		}
	case "changes":
		data = st.Changes
		if !s.json {
			if len(st.Changes) == 0 {
				fmt.Fprintln(s.human(), "Working tree clean")
			}
			for _, c := range st.Changes {
				fmt.Fprintf(s.human(), "%s%s  %q", c.Index, c.Worktree, c.Path)
				if c.Original != "" {
					fmt.Fprintf(s.human(), " ← %q", c.Original)
				}
				fmt.Fprintln(s.human())
			}
		}
	case "diff":
		var diff string
		diff, err = s.deps.Git.Diff(ctx, st.Root, file, staged)
		data = struct {
			Staged bool   `json:"staged"`
			File   string `json:"file"`
			Diff   string `json:"diff"`
		}{staged, file, diff}
		if !s.json {
			if diff == "" {
				fmt.Fprintln(s.human(), "No tracked changes in this view; untracked files have no Git diff.")
			} else {
				fmt.Fprint(s.human(), safe(diff))
			}
		}
	case "history":
		var commits []gitlocal.Commit
		if st.Unborn {
			commits = []gitlocal.Commit{}
		} else {
			commits, err = s.deps.Git.History(ctx, st.Root, f)
		}
		data = commits
		if !s.json {
			printCommits(s, commits)
		}
	case "branches":
		var branches []gitlocal.Branch
		branches, err = s.deps.Git.Branches(ctx, st.Root)
		data = branches
		if !s.json {
			for _, b := range branches {
				mark := " "
				if b.Current {
					mark = ">"
				}
				fmt.Fprintf(s.human(), "%s %s  %s  %s %s\n", mark, safe(b.Name), shortHash(b.Head), safe(b.Upstream), safe(b.Tracking))
			}
		}
	case "worktrees":
		var trees []gitlocal.Worktree
		trees, err = s.deps.Git.Worktrees(ctx, st.Root)
		data = trees
		if !s.json {
			for _, w := range trees {
				fmt.Fprintf(s.human(), "%s  %s  detached=%v locked=%s prunable=%s\n", safe(w.Path), safe(w.Branch), w.Detached, safe(w.Locked), safe(w.Prunable))
			}
		}
	case "activity":
		var a gitlocal.Activity
		if st.Unborn {
			a = gitlocal.Activity{Commits: []gitlocal.Commit{}, Days: map[string]int{}, Weeks: map[string]int{}}
		} else {
			a, err = s.deps.Git.Activity(ctx, st.Root, f)
		}
		data = a
		if !s.json {
			fmt.Fprintln(s.human(), "Commits during this period · all authors unless --author is set")
			printCommits(s, a.Commits)
			for _, day := range sortedDays(a.Days) {
				fmt.Fprintf(s.human(), "%s  %s %d\n", day, strings.Repeat("█", min(40, a.Days[day])), a.Days[day])
			}
			if a.Truncated {
				fmt.Fprintln(s.human(), "History limit reached; narrow dates or increase --limit")
			}
		}
	}
	s.git = struct {
		Action string `json:"action"`
		Data   any    `json:"data"`
	}{mode, data}
	return err
}

func branchLabel(st gitlocal.Status) string {
	if st.Detached {
		return "detached HEAD"
	}
	if st.Unborn {
		return st.Branch + " (no commits)"
	}
	return st.Branch
}
func shortHash(hash string) string { return hash[:min(8, len(hash))] }
func printCommits(s *state, commits []gitlocal.Commit) {
	if len(commits) == 0 {
		fmt.Fprintln(s.human(), "No commits in this view")
	}
	for _, c := range commits {
		fmt.Fprintf(s.human(), "%s  %s  %s — %s\n", shortHash(c.Hash), c.Date.Local().Format("2006-01-02 15:04"), safe(c.Subject), safe(c.Author))
	}
}

func safe(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return '�'
	}, v)
}

func sortedDays(days map[string]int) []string {
	result := make([]string, 0, len(days))
	for day := range days {
		result = append(result, day)
	}
	slices.Sort(result)
	return result
}

func (s *state) registrationAdd(repositories bool) *cobra.Command {
	var name, description string
	var tags []string
	cmd := &cobra.Command{Use: "add <path>", Short: "Register an existing local directory", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if s.deps.Workspace == nil {
			return errors.New("workspace storage unavailable")
		}
		p, err := s.deps.Workspace.Add(cmd.Context(), args[0], name, description, tags, repositories)
		if err != nil {
			return err
		}
		if repositories {
			s.git = p
		} else {
			s.workspace = p
		}
		if !s.json {
			fmt.Fprintf(s.human(), "Registered %s → %s\n", safe(p.Name), safe(p.Root))
		}
		return nil
	}}
	cmd.Flags().StringVar(&name, "name", "", "Unique display name / alias")
	cmd.Flags().StringVar(&description, "description", "", "Project description")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Project tags (comma separated)")
	return cmd
}

func (s *state) registrationRemove(repositories bool) *cobra.Command {
	return &cobra.Command{Use: "remove <name>", Short: "Unregister metadata only; never delete files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if s.deps.Workspace == nil {
			return errors.New("workspace storage unavailable")
		}
		if err := s.deps.Workspace.Remove(cmd.Context(), args[0], repositories); err != nil {
			return err
		}
		v := map[string]string{"removed": args[0], "scope": "metadata only"}
		if repositories {
			s.git = v
		} else {
			s.workspace = v
		}
		if !s.json {
			fmt.Fprintln(s.human(), "Registration removed; project files unchanged.")
		}
		return nil
	}}
}

func (s *state) registryList(ctx context.Context, repositories bool) error {
	if s.deps.Workspace == nil {
		return errors.New("workspace storage unavailable")
	}
	result, err := s.deps.Workspace.Board(ctx, repositories)
	if err != nil {
		return err
	}
	if repositories {
		s.git = result
	} else {
		s.workspace = result
	}
	if !s.json {
		if len(result) == 0 {
			fmt.Fprintln(s.human(), "No registered projects. Use add <path> --name <name>.")
		}
		for _, o := range result {
			state := "non-Git"
			if !o.Available {
				state = "unavailable"
			} else if o.Git == nil && o.Project.RepositoryID != "" {
				state = "Git unavailable"
			} else if o.Git != nil {
				state = fmt.Sprintf("%s · %d changes · +%d/-%d", branchLabel(*o.Git), len(o.Git.Changes), o.Git.Ahead, o.Git.Behind)
			}
			fmt.Fprintf(s.human(), "%-22s %s\n  %s · focus today %s\n", safe(o.Project.Name), safe(state), safe(o.Project.Root), humanDuration(o.Focus.Today.FocusSeconds))
			if o.Warning != "" {
				fmt.Fprintln(s.human(), "  Warning: "+safe(o.Warning))
			}
		}
	}
	return nil
}

func (s *state) workspaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Your local projects, Git and focus timeline", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if s.json || !s.interactive() {
			return s.registryList(cmd.Context(), false)
		}
		return s.runDeveloper(cmd.Context(), devtui.Options{})
	}}
	cmd.AddCommand(s.registrationAdd(false), s.registrationRemove(false), &cobra.Command{Use: "list", Short: "List registered projects and local status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return s.registryList(cmd.Context(), false) }}, &cobra.Command{Use: "board", Short: "Open the multi-project dashboard", Args: cobra.NoArgs, RunE: cmd.RunE})
	for _, mode := range []string{"show", "path", "activity"} {
		f := gitlocal.Filter{}
		c := &cobra.Command{Use: mode + " <name>", Short: map[string]string{"show": "Show project details", "path": "Print the absolute project directory", "activity": "Merge project Git and focus events"}[mode], Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if s.deps.Workspace == nil {
				return errors.New("workspace storage unavailable")
			}
			p, err := s.deps.Workspace.Find(cmd.Context(), args[0], false)
			if err != nil {
				return err
			}
			switch mode {
			case "path":
				s.workspace = map[string]string{"path": p.Root}
				if !s.json {
					fmt.Fprintln(s.human(), p.Root)
				}
			case "show":
				o := s.deps.Workspace.Overview(cmd.Context(), p)
				s.workspace = o
				if !s.json {
					fmt.Fprintf(s.human(), "%s\n%s\n%s\nTags: %s\nRegistered: %s\nFocus today: %s\n", safe(p.Name), safe(p.Root), safe(p.Description), safe(strings.Join(p.Tags, ", ")), p.CreatedAt.Local().Format(time.RFC3339), humanDuration(o.Focus.Today.FocusSeconds))
					if o.Git != nil {
						fmt.Fprintf(s.human(), "Git: %s · HEAD %.8s · %d changes · +%d/-%d (local refs)\n", safe(branchLabel(*o.Git)), o.Git.Head, len(o.Git.Changes), o.Git.Ahead, o.Git.Behind)
					}
					if o.Warning != "" {
						fmt.Fprintln(s.human(), "Warning: "+safe(o.Warning))
					}
				}
			case "activity":
				timeline, err := s.deps.Workspace.Timeline(cmd.Context(), p, f)
				if err != nil {
					return err
				}
				s.workspace = timeline
				if !s.json {
					s.printTimeline(p.Name, timeline)
				}
			}
			return nil
		}}
		if mode == "activity" {
			queryFlags(c, &f)
		}
		cmd.AddCommand(c)
	}
	cmd.AddCommand(&cobra.Command{Use: "today", Short: "Today's associated focus and Git events (all authors)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := s.deps.Workspace.List(cmd.Context(), false)
		if err != nil {
			return err
		}
		now := time.Now()
		since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		result := map[string]workspace.Timeline{}
		for _, p := range items {
			t, e := s.deps.Workspace.Timeline(cmd.Context(), p, gitlocal.Filter{Since: since, Until: now, Limit: 100})
			if e != nil {
				return e
			}
			result[p.Name] = t
			if !s.json {
				s.printTimeline(p.Name, t)
			}
		}
		s.workspace = result
		return nil
	}})
	return cmd
}

func (s *state) printTimeline(name string, t workspace.Timeline) {
	fmt.Fprintln(s.human(), safe(name)+" · commits and focus are separate facts")
	if len(t.Events) == 0 {
		fmt.Fprintln(s.human(), "No activity in this view")
	}
	for _, e := range t.Events {
		fmt.Fprintf(s.human(), "%s  %-3s %s\n", e.Time.Local().Format("01-02 15:04"), e.Source, safe(e.Text))
	}
	if t.Warning != "" {
		fmt.Fprintln(s.human(), safe(t.Warning))
	}
	if t.Truncated {
		fmt.Fprintln(s.human(), "Git history bounded; use --since / --until / --limit")
	}
}

func (s *state) runDeveloper(ctx context.Context, opts devtui.Options) error {
	if s.json || !s.interactive() {
		return errors.New("interactive terminal required; use git status / workspace list --json")
	}
	if s.deps.Workspace == nil {
		return errors.New("workspace storage unavailable")
	}
	opts.Git, opts.Workspace, opts.NoColor = s.deps.Git, s.deps.Workspace, s.noColor || os.Getenv("NO_COLOR") != ""
	for {
		action, err := devtui.Run(ctx, s.deps.In, s.human(), opts)
		if err != nil {
			return err
		}
		if action.Focus == nil {
			return nil
		}
		old := s.zenProject
		s.zenProject = action.Focus.Name
		opts.Selected = action.Focus.ID
		err = s.runZen(ctx, zentui.StartFocus, zen.Config{})
		s.zenProject = old
		if err != nil {
			return err
		}
	}
}
