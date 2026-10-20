package monitor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const HistoryLimit = 60

type Collector interface {
	Collect(context.Context) (Snapshot, error)
}

type System struct {
	OS            string  `json:"os"`
	Architecture  string  `json:"architecture"`
	Hostname      string  `json:"hostname"`
	CPUModel      string  `json:"cpu_model"`
	LogicalCores  int     `json:"logical_cores"`
	UptimeSeconds *uint64 `json:"uptime_seconds"`
}

type Memory struct {
	Total     uint64  `json:"total_bytes"`
	Used      uint64  `json:"used_bytes"`
	Available uint64  `json:"available_bytes"`
	Percent   float64 `json:"used_percent"`
}

type CPU struct {
	Percent *float64   `json:"percent"`
	Cores   []*float64 `json:"per_core_percent"`
}

type Disk struct {
	Device     string  `json:"device"`
	Mount      string  `json:"mount"`
	Filesystem string  `json:"filesystem"`
	Usage      *Memory `json:"usage"`
}

type Rates struct {
	Read  *float64 `json:"read_bytes_per_second"`
	Write *float64 `json:"write_bytes_per_second"`
}

type Network struct {
	Name     string   `json:"name"`
	Download *float64 `json:"download_bytes_per_second"`
	Upload   *float64 `json:"upload_bytes_per_second"`
}

type Process struct {
	PID    int32    `json:"pid"`
	Name   string   `json:"name"`
	CPU    *float64 `json:"cpu_percent"`
	Memory *uint64  `json:"memory_bytes"`
}

type Snapshot struct {
	At        time.Time `json:"sampled_at"`
	System    System    `json:"system"`
	CPU       CPU       `json:"cpu"`
	Memory    *Memory   `json:"memory"`
	Swap      *Memory   `json:"swap"`
	Disks     []Disk    `json:"disks"`
	DiskIO    Rates     `json:"disk_io"`
	Networks  []Network `json:"networks"`
	Processes []Process `json:"processes"`
	Warnings  []string  `json:"warnings"`
}

func ValidateInterval(interval time.Duration) error {
	if interval < 250*time.Millisecond || interval > 10*time.Second {
		return fmt.Errorf("refresh interval must be between 250ms and 10s")
	}
	return nil
}

// CPUPercent uses cumulative time deltas; guest time must not be added twice.
func CPUPercent(previousTotal, previousIdle, total, idle float64) *float64 {
	delta := total - previousTotal
	idleDelta := idle - previousIdle
	if delta <= 0 || idleDelta < 0 || idleDelta > delta {
		return nil
	}
	value := math.Max(0, math.Min(100, 100*(delta-idleDelta)/delta))
	return &value
}

func CounterRate(previous, current uint64, elapsed time.Duration) *float64 {
	if elapsed <= 0 || current < previous {
		return nil
	}
	value := float64(current-previous) / elapsed.Seconds()
	return &value
}

func Processes(items []Process, query, order string) []Process {
	result := make([]Process, 0, len(items))
	query = strings.ToLower(strings.TrimSpace(query))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strconv.Itoa(int(item.PID)), query) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch order {
		case "cpu":
			if a.CPU != nil && b.CPU == nil {
				return true
			}
			if a.CPU == nil && b.CPU != nil {
				return false
			}
			if a.CPU != nil && b.CPU != nil && *a.CPU != *b.CPU {
				return *a.CPU > *b.CPU
			}
		case "memory":
			if a.Memory != nil && b.Memory == nil {
				return true
			}
			if a.Memory == nil && b.Memory != nil {
				return false
			}
			if a.Memory != nil && b.Memory != nil && *a.Memory != *b.Memory {
				return *a.Memory > *b.Memory
			}
		}
		return a.PID < b.PID
	})
	return result
}

func AppendHistory(history []float64, value *float64) []float64 {
	point := math.NaN()
	if value != nil {
		point = *value
	}
	if len(history) == HistoryLimit {
		history = history[1:]
	}
	return append(history, point)
}
