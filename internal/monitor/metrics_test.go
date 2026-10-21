package monitor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestSampleDeltas(t *testing.T) {
	if value := CPUPercent(100, 40, 200, 70); value == nil || *value != 70 {
		t.Fatalf("CPU=%v", value)
	}
	for _, input := range [][4]float64{{100, 40, 99, 40}, {100, 40, 100, 40}, {100, 40, 101, 39}, {100, 40, 101, 45}} {
		if CPUPercent(input[0], input[1], input[2], input[3]) != nil {
			t.Fatal("reset/invalid CPU counter should be unavailable")
		}
	}
	if rate := CounterRate(100, 400, 2*time.Second); rate == nil || *rate != 150 {
		t.Fatalf("rate=%v", rate)
	}
	if CounterRate(400, 100, time.Second) != nil || CounterRate(100, 100, 0) != nil {
		t.Fatal("reset/invalid rate should be unavailable")
	}
	total, idle := cpuTime(cpu.TimesStat{User: 50, System: 10, Idle: 20, Iowait: 10, Guest: 5, GuestNice: 2})
	if total != 90 || idle != 30 {
		t.Fatal("guest time was double counted")
	}
}

func TestProcessesAndBoundedHistory(t *testing.T) {
	busy, quiet := 80.0, 2.0
	large, small := uint64(100), uint64(10)
	items := []Process{{PID: 30, Name: "zeta", CPU: &busy, Memory: &small}, {PID: 10, Name: "Alpha", CPU: &quiet, Memory: &large}, {PID: 20, Name: "gone"}}
	if Processes(items, "", "cpu")[0].PID != 30 || Processes(items, "", "memory")[0].PID != 10 || Processes(items, "", "pid")[0].PID != 10 {
		t.Fatal("sort order incorrect")
	}
	if result := Processes(items, "ALP", "cpu"); len(result) != 1 || result[0].PID != 10 {
		t.Fatal("name search incorrect")
	}
	if len(Processes(items, "30", "cpu")) != 1 || len(Processes(items, "missing", "pid")) != 0 {
		t.Fatal("PID or empty search incorrect")
	}
	var history []float64
	for range HistoryLimit + 10 {
		history = AppendHistory(history, &busy)
	}
	history = AppendHistory(history, nil)
	if len(history) != HistoryLimit || !math.IsNaN(history[len(history)-1]) {
		t.Fatal("history must be bounded and preserve missing samples")
	}
}

func TestIntervalAndCancellation(t *testing.T) {
	for _, interval := range []time.Duration{0, 100 * time.Millisecond, 11 * time.Second} {
		if ValidateInterval(interval) == nil {
			t.Fatal("invalid interval accepted")
		}
	}
	for _, interval := range []time.Duration{250 * time.Millisecond, time.Second, 10 * time.Second} {
		if err := ValidateInterval(interval); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	collector := &NativeCollector{}
	if _, err := collector.Collect(ctx); err != context.Canceled {
		t.Fatalf("cancellation=%v", err)
	}
}
