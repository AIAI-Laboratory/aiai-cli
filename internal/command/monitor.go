package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/monitor"
	monitortui "github.com/AIAI-Laboratory/aiai-cli/internal/tui/monitor"
)

func (s *state) monitorCommand() *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{Use: "monitor", Short: "Live system monitor (read-only)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return s.runMonitor(cmd.Context(), interval) }}
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "Refresh interval (250ms–10s)")
	return cmd
}

func (s *state) metricsCollector() monitor.Collector {
	if s.deps.Monitor != nil {
		return s.deps.Monitor
	}
	return &monitor.NativeCollector{}
}

func (s *state) runMonitor(ctx context.Context, interval time.Duration) error {
	if err := monitor.ValidateInterval(interval); err != nil {
		return err
	}
	if s.json {
		return fmt.Errorf("interactive monitor does not support --json; use aiai system info --json")
	}
	if !s.interactive() {
		return fmt.Errorf("interactive terminal required; use aiai system info or aiai system info --json")
	}
	return monitortui.Run(ctx, s.deps.In, s.human(), s.metricsCollector(), monitortui.Options{Interval: interval, NoColor: s.noColor || os.Getenv("NO_COLOR") != ""})
}

func (s *state) systemCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "system", Short: "Inspect system information", Args: cobra.NoArgs}
	cmd.AddCommand(&cobra.Command{Use: "info", Short: "One-time system metrics snapshot", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
		defer cancel()
		collector := s.metricsCollector()
		if _, err := collector.Collect(ctx); err != nil {
			return err
		}
		// Rates require a real baseline, including in one-shot JSON output.
		timer := time.NewTimer(200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		snapshot, err := collector.Collect(ctx)
		if err != nil {
			return err
		}
		s.system = &snapshot
		if !s.json {
			s.printSystem(snapshot)
		}
		return nil
	}})
	return cmd
}

func (s *state) printSystem(snapshot monitor.Snapshot) {
	fmt.Fprintf(s.human(), "System: %s/%s\nHost: %s\nCPU: %s · %d logical cores\n", snapshot.System.OS, snapshot.System.Architecture, snapshot.System.Hostname, snapshot.System.CPUModel, snapshot.System.LogicalCores)
	if snapshot.System.UptimeSeconds != nil {
		fmt.Fprintf(s.human(), "Uptime: %s\n", time.Duration(*snapshot.System.UptimeSeconds)*time.Second)
	}
	if snapshot.CPU.Percent == nil {
		fmt.Fprintln(s.human(), "CPU utilization: unavailable")
	} else {
		fmt.Fprintf(s.human(), "CPU utilization: %.1f%%\n", *snapshot.CPU.Percent)
	}
	fmt.Fprint(s.human(), "Per-core CPU:")
	for i, percent := range snapshot.CPU.Cores {
		if percent == nil {
			fmt.Fprintf(s.human(), " %d:unavailable", i+1)
		} else {
			fmt.Fprintf(s.human(), " %d:%.1f%%", i+1, *percent)
		}
	}
	fmt.Fprintln(s.human())
	if snapshot.Memory == nil {
		fmt.Fprintln(s.human(), "RAM: unavailable")
	} else {
		ram := snapshot.Memory
		fmt.Fprintf(s.human(), "RAM: %.1f%% · %.2f / %.2f GiB · available %.2f GiB\n", ram.Percent, float64(ram.Used)/(1<<30), float64(ram.Total)/(1<<30), float64(ram.Available)/(1<<30))
	}
	if snapshot.Swap == nil {
		fmt.Fprintln(s.human(), "Swap: unavailable")
	} else {
		fmt.Fprintf(s.human(), "Swap: %d / %d bytes\n", snapshot.Swap.Used, snapshot.Swap.Total)
	}
	for _, item := range snapshot.Disks {
		if item.Usage == nil {
			fmt.Fprintf(s.human(), "Disk %s: unavailable\n", item.Mount)
		} else {
			fmt.Fprintf(s.human(), "Disk %s: %.1f%% · %.2f / %.2f GiB · free %.2f GiB\n", item.Mount, item.Usage.Percent, float64(item.Usage.Used)/(1<<30), float64(item.Usage.Total)/(1<<30), float64(item.Usage.Available)/(1<<30))
		}
	}
	for _, iface := range snapshot.Networks {
		if iface.Download == nil || iface.Upload == nil {
			fmt.Fprintf(s.human(), "Network %s: unavailable\n", iface.Name)
		} else {
			fmt.Fprintf(s.human(), "Network %s: ↓ %.0f B/s · ↑ %.0f B/s\n", iface.Name, *iface.Download, *iface.Upload)
		}
	}
	if snapshot.DiskIO.Read == nil || snapshot.DiskIO.Write == nil {
		fmt.Fprintln(s.human(), "Disk I/O: unavailable")
	} else {
		fmt.Fprintf(s.human(), "Disk I/O: read %.0f B/s · write %.0f B/s\n", *snapshot.DiskIO.Read, *snapshot.DiskIO.Write)
	}
	fmt.Fprintf(s.human(), "Processes: %d (names only; CPU uses 100%% per core)\n", len(snapshot.Processes))
	for _, warning := range snapshot.Warnings {
		fmt.Fprintln(s.human(), "Unavailable:", warning)
	}
}
