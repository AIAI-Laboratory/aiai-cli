package monitor

import (
	"context"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// NativeCollector is stateful: call it serially so every rate shares a baseline.
type NativeCollector struct {
	previousAt time.Time
	cpuTimes   []cpu.TimesStat
	network    map[string]gnet.IOCountersStat
	disks      map[string]disk.IOCountersStat
	processes  map[int32]processSample
	info       System
}

type processSample struct {
	seconds float64
	created int64
	at      time.Time
}

func (c *NativeCollector) Collect(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{At: time.Now(), Disks: []Disk{}, Networks: []Network{}, Processes: []Process{}, Warnings: []string{}}
	if c.info.OS == "" {
		c.info.OS, c.info.Architecture = runtime.GOOS, runtime.GOARCH
		c.info.Hostname, c.info.CPUModel = "unavailable", "unavailable"
		if name, err := os.Hostname(); err == nil {
			c.info.Hostname = name
		}
		if info, err := host.InfoWithContext(ctx); err == nil {
			c.info.Hostname = info.Hostname
		}
		if info, err := cpu.InfoWithContext(ctx); err == nil && len(info) > 0 {
			c.info.CPUModel = info[0].ModelName
		}
		c.info.LogicalCores, _ = cpu.CountsWithContext(ctx, true)
		if c.info.LogicalCores < 1 {
			c.info.LogicalCores = runtime.NumCPU()
		}
	}
	s.System = c.info
	if uptime, err := host.UptimeWithContext(ctx); err == nil {
		s.System.UptimeSeconds = &uptime
	} else {
		s.Warnings = append(s.Warnings, "uptime unavailable")
	}
	if current, err := cpu.TimesWithContext(ctx, true); err == nil {
		var total, idle, prevTotal, prevIdle float64
		for i, item := range current {
			t, id := cpuTime(item)
			total += t
			idle += id
			var percent *float64
			if i < len(c.cpuTimes) && c.cpuTimes[i].CPU == item.CPU {
				pt, pi := cpuTime(c.cpuTimes[i])
				percent = CPUPercent(pt, pi, t, id)
				prevTotal += pt
				prevIdle += pi
			}
			s.CPU.Cores = append(s.CPU.Cores, percent)
		}
		if len(current) > 0 && len(current) == len(c.cpuTimes) {
			s.CPU.Percent = CPUPercent(prevTotal, prevIdle, total, idle)
		}
		c.cpuTimes = current
	} else {
		s.Warnings = append(s.Warnings, "CPU utilization unavailable")
		c.cpuTimes = nil
	}
	if ram, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.Memory = &Memory{Total: ram.Total, Used: ram.Used, Available: ram.Available, Percent: ram.UsedPercent}
	} else {
		s.Warnings = append(s.Warnings, "RAM unavailable")
	}
	if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
		s.Swap = &Memory{Total: swap.Total, Used: swap.Used, Available: swap.Free, Percent: swap.UsedPercent}
	} else {
		s.Warnings = append(s.Warnings, "swap unavailable")
	}
	if partitions, err := disk.PartitionsWithContext(ctx, false); err == nil {
		for _, partition := range partitions {
			if ctx.Err() != nil {
				return Snapshot{}, ctx.Err()
			}
			item := Disk{Device: partition.Device, Mount: partition.Mountpoint, Filesystem: partition.Fstype}
			if usage, err := disk.UsageWithContext(ctx, partition.Mountpoint); err == nil {
				item.Usage = &Memory{Total: usage.Total, Used: usage.Used, Available: usage.Free, Percent: usage.UsedPercent}
			}
			s.Disks = append(s.Disks, item)
		}
	} else {
		s.Warnings = append(s.Warnings, "disk partitions unavailable")
	}
	elapsed := s.At.Sub(c.previousAt)
	if counters, err := disk.IOCountersWithContext(ctx); err == nil {
		var read, write float64
		valid := false
		for name, item := range counters {
			if previous, ok := c.disks[name]; ok && !c.previousAt.IsZero() {
				r, w := CounterRate(previous.ReadBytes, item.ReadBytes, elapsed), CounterRate(previous.WriteBytes, item.WriteBytes, elapsed)
				if r != nil && w != nil {
					read += *r
					write += *w
					valid = true
				}
			}
		}
		if valid {
			s.DiskIO = Rates{Read: &read, Write: &write}
		}
		c.disks = counters
	} else {
		c.disks = nil
		s.Warnings = append(s.Warnings, "disk throughput unavailable")
	}
	if counters, err := gnet.IOCountersWithContext(ctx, true); err == nil {
		next := make(map[string]gnet.IOCountersStat, len(counters))
		for _, item := range counters {
			entry := Network{Name: item.Name}
			if previous, ok := c.network[item.Name]; ok && !c.previousAt.IsZero() {
				entry.Download = CounterRate(previous.BytesRecv, item.BytesRecv, elapsed)
				entry.Upload = CounterRate(previous.BytesSent, item.BytesSent, elapsed)
			}
			next[item.Name] = item
			s.Networks = append(s.Networks, entry)
		}
		c.network = next
		sort.Slice(s.Networks, func(i, j int) bool { return s.Networks[i].Name < s.Networks[j].Name })
	} else {
		c.network = nil
		s.Warnings = append(s.Warnings, "network unavailable")
	}
	if processes, err := process.ProcessesWithContext(ctx); err == nil {
		next := make(map[int32]processSample, len(processes))
		for _, proc := range processes {
			if ctx.Err() != nil {
				return Snapshot{}, ctx.Err()
			}
			name, err := proc.NameWithContext(ctx)
			if err != nil {
				continue
			} // Exited or inaccessible process; ordinary monitoring never escalates.
			entry := Process{PID: proc.Pid, Name: name}
			if memory, err := proc.MemoryInfoWithContext(ctx); err == nil && (memory.RSS > 0 || memory.VMS > 0) {
				entry.Memory = &memory.RSS
			}
			if times, err := proc.TimesWithContext(ctx); err == nil {
				// libproc can return an empty task struct for protected macOS processes
				// without surfacing an error. Do not present those zeros as real metrics.
				if runtime.GOOS == "darwin" && entry.Memory == nil && times.User+times.System == 0 {
					s.Processes = append(s.Processes, entry)
					continue
				}
				if created, err := proc.CreateTimeWithContext(ctx); err == nil {
					now := time.Now()
					seconds := times.User + times.System
					if prev, ok := c.processes[proc.Pid]; ok && prev.created == created && seconds >= prev.seconds && now.After(prev.at) {
						value := 100 * (seconds - prev.seconds) / now.Sub(prev.at).Seconds()
						entry.CPU = &value // htop convention: 100% per fully occupied logical core.
					}
					next[proc.Pid] = processSample{seconds: seconds, created: created, at: now}
				}
			}
			s.Processes = append(s.Processes, entry)
		}
		c.processes = next
	} else {
		c.processes = nil
		s.Warnings = append(s.Warnings, "process list unavailable")
	}
	c.previousAt = s.At
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

func cpuTime(t cpu.TimesStat) (float64, float64) {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal, t.Idle + t.Iowait
}
