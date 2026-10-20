package zen

import (
	"testing"
	"time"
)

func TestTimerUsesElapsedTimeAndExcludesPauses(t *testing.T) {
	start := time.Now()
	timer := NewTimer(10 * time.Minute)
	timer.Start(start)
	if got := timer.Remaining(start.Add(2 * time.Minute)); got != 8*time.Minute {
		t.Fatalf("remaining = %s", got)
	}
	timer.Pause(start.Add(2 * time.Minute))
	if got := timer.Remaining(start.Add(7 * time.Minute)); got != 8*time.Minute {
		t.Fatalf("paused remaining = %s", got)
	}
	timer.Resume(start.Add(7 * time.Minute))
	for minute := 8; minute < 15; minute++ {
		_ = timer.Remaining(start.Add(time.Duration(minute) * time.Minute))
	}
	if got := timer.Remaining(start.Add(15 * time.Minute)); got != 0 || timer.State != TimerCompleted {
		t.Fatalf("completion = %s, %s", got, timer.State)
	}
}

func TestTimerPausesOnUncertainSleepGap(t *testing.T) {
	start := time.Now()
	timer := NewTimer(25 * time.Minute)
	timer.Start(start)
	_ = timer.Remaining(start.Add(time.Second))
	got := timer.Remaining(start.Add(10 * time.Minute))
	if !timer.Uncertain() || timer.State != TimerPaused || got > 25*time.Minute || got < 24*time.Minute {
		t.Fatalf("sleep handling: uncertain=%v state=%s remaining=%s", timer.Uncertain(), timer.State, got)
	}
}

func TestStatisticsDistinguishCompletedAndInterrupted(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	sessions := []Session{
		{Type: SessionFocus, StartedAt: now.Add(-time.Hour), ActiveSeconds: 1500, Status: StatusCompleted},
		{Type: SessionFocus, StartedAt: now.Add(-2 * time.Hour), ActiveSeconds: 300, Status: StatusInterrupted},
		{Type: SessionShortBreak, StartedAt: now.Add(-3 * time.Hour), ActiveSeconds: 300, Status: StatusCompleted},
	}
	stats := CalculateStatistics(sessions, now)
	if stats.Today.FocusSeconds != 1800 || stats.Today.Completed != 1 || stats.Today.Interrupted != 1 || stats.Today.CompletionRate != 50 {
		t.Fatalf("stats = %+v", stats.Today)
	}
}
