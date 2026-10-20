package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	zentui "github.com/AIAI-Laboratory/aiai-cli/internal/tui/zen"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

func (s *state) zenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zen",
		Short: "A little peace inside your terminal",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runZen(cmd.Context(), zentui.StartAquarium, zen.Config{})
		},
	}
	cmd.AddCommand(
		&cobra.Command{Use: "aquarium", Short: "Open the interactive aquarium", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runZen(cmd.Context(), zentui.StartAquarium, zen.Config{})
		}},
		s.zenFocusCommand(), s.zenStatsCommand(), s.zenHistoryCommand(),
		&cobra.Command{Use: "settings", Short: "Manage Zen preferences", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runZen(cmd.Context(), zentui.StartSettings, zen.Config{})
		}},
	)
	return cmd
}

func (s *state) zenFocusCommand() *cobra.Command {
	config := zen.DefaultConfig()
	var work, shortBreak, longBreak time.Duration
	cmd := &cobra.Command{Use: "focus", Short: "Focus with a Pomodoro aquarium", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		config.WorkSeconds, config.BreakSeconds, config.LongBreakSeconds = int64(work.Seconds()), int64(shortBreak.Seconds()), int64(longBreak.Seconds())
		return s.runZen(cmd.Context(), zentui.StartFocus, config)
	}}
	cmd.Flags().DurationVar(&work, "work", 25*time.Minute, "Focus duration")
	cmd.Flags().DurationVar(&shortBreak, "break", 5*time.Minute, "Short break duration")
	cmd.Flags().DurationVar(&longBreak, "long-break", 15*time.Minute, "Long break duration")
	cmd.Flags().IntVar(&config.Cycles, "cycles", 4, "Focus sessions before a long break")
	return cmd
}

func (s *state) zenStatsCommand() *cobra.Command {
	var today, week bool
	cmd := &cobra.Command{Use: "stats", Short: "Show focus statistics", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if today && week {
			return errors.New("choose only one of --today or --week")
		}
		config, sessions, err := s.loadZen(cmd.Context())
		if err != nil {
			return err
		}
		stats := zen.CalculateStatistics(sessions, time.Now())
		s.zen = &output.Zen{Action: "stats", Statistics: &stats, Config: &config}
		if !s.json {
			printZenStats(s.human(), stats, today, week)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&today, "today", false, "Show today's summary")
	cmd.Flags().BoolVar(&week, "week", false, "Show this week's summary")
	return cmd
}

func (s *state) zenHistoryCommand() *cobra.Command {
	return &cobra.Command{Use: "history", Short: "Show focus session history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, sessions, err := s.loadZen(cmd.Context())
		if err != nil {
			return err
		}
		s.zen = &output.Zen{Action: "history", Sessions: &sessions}
		if !s.json {
			printZenHistory(s.human(), sessions)
		}
		return nil
	}}
}

func (s *state) loadZen(ctx context.Context) (zen.Config, []zen.Session, error) {
	if s.deps.Zen == nil {
		return zen.Config{}, nil, errors.New("zen storage is unavailable")
	}
	return s.deps.Zen.Load(ctx)
}

func (s *state) runZen(ctx context.Context, mode zentui.StartMode, override zen.Config) error {
	if s.json {
		return errors.New("--json is available for zen stats and zen history, not interactive Zen modes")
	}
	if !s.interactive() {
		return errors.New("interactive terminal required; use `aiai zen stats --json` or `aiai zen history --json` for non-interactive output")
	}
	config, sessions, err := s.loadZen(ctx)
	if err != nil {
		return err
	}
	if override.WorkSeconds > 0 {
		config.WorkSeconds, config.BreakSeconds, config.LongBreakSeconds, config.Cycles = override.WorkSeconds, override.BreakSeconds, override.LongBreakSeconds, override.Cycles
	}
	if err := config.Validate(); err != nil {
		return err
	}
	return zentui.Run(ctx, s.deps.In, s.human(), zentui.Options{Mode: mode, Config: config, Sessions: sessions, Repo: s.deps.Zen, NoColor: s.noColor})
}

func printZenStats(w io.Writer, stats zen.Statistics, onlyToday, onlyWeek bool) {
	if !onlyWeek {
		fmt.Fprintf(w, "Today\n  Focus: %s\n  Completed: %d\n  Interrupted: %d\n  Completion rate: %d%%\n", humanDuration(stats.Today.FocusSeconds), stats.Today.Completed, stats.Today.Interrupted, stats.Today.CompletionRate)
	}
	if !onlyToday {
		fmt.Fprintln(w, "This week")
		for _, day := range stats.Week {
			fmt.Fprintf(w, "  %s  %-9s %s\n", day.Date, strings.Repeat("█", min(20, int(day.FocusSeconds/900))), humanDuration(day.FocusSeconds))
		}
	}
}

func printZenHistory(w io.Writer, sessions []zen.Session) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "No Zen sessions recorded yet.")
		return
	}
	fmt.Fprintln(w, "DATE                 TYPE          PLANNED  ACTUAL   STATUS")
	for _, item := range sessions {
		fmt.Fprintf(w, "%-20s %-13s %-8s %-8s %s\n", item.StartedAt.Local().Format("2006-01-02 15:04"), item.Type, humanDuration(item.PlannedSeconds), humanDuration(item.ActiveSeconds), item.Status)
	}
}

func humanDuration(seconds int64) string {
	return (time.Duration(seconds) * time.Second).Round(time.Second).String()
}
