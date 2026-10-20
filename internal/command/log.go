package command

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func (s *state) logCommand() *cobra.Command {
	log := &cobra.Command{Use: "log", Short: "View local SSH access history", Args: cobra.NoArgs}
	var server string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List SSH access logs newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.listLogs(cmd, server, limit)
		},
	}
	list.Flags().StringVar(&server, "server", "", "Filter by server alias")
	list.Flags().IntVar(&limit, "limit", 50, "Maximum number of logs (1-1000)")
	log.AddCommand(list)
	return log
}

func (s *state) listLogs(cmd *cobra.Command, alias string, limit int) error {
	if err := s.requireSSH(); err != nil {
		return err
	}
	logs, err := s.deps.SSH.Logs(cmd.Context(), sshmanager.LogFilter{ServerAlias: alias, Limit: limit})
	if err != nil {
		return err
	}
	if alias != "" {
		alias, _ = sshmanager.NormalizeAlias(alias)
	}
	s.ssh = &output.SSH{Action: "log-list", Alias: alias, Logs: &logs}
	if !s.json {
		printLogs(s.human(), logs)
	}
	return nil
}

func printLogs(w io.Writer, logs []sshmanager.AccessLog) {
	if len(logs) == 0 {
		fmt.Fprintln(w, "No SSH access logs found.")
		return
	}
	fmt.Fprintf(w, "%-22s %-18s %-24s %-10s %-9s %s\n", "STARTED", "SERVER", "DESTINATION", "STATUS", "DURATION", "EXIT")
	for _, entry := range logs {
		duration := "-"
		if entry.DurationMS != nil {
			duration = (time.Duration(*entry.DurationMS) * time.Millisecond).Round(time.Millisecond).String()
		}
		exit := "-"
		if entry.ExitCode != nil {
			exit = strconv.Itoa(*entry.ExitCode)
		}
		destination := entry.Username + "@" + entry.Host
		fmt.Fprintf(w, "%-22s %-18s %-24s %-10s %-9s %s\n", entry.StartedAt.Local().Format("2006-01-02 15:04:05 MST"), entry.ServerAlias, destination, entry.Status, duration, exit)
	}
}
