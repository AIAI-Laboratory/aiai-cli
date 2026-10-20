package zen

import "time"

type TimerState string

const (
	TimerIdle      TimerState = "idle"
	TimerRunning   TimerState = "running"
	TimerPaused    TimerState = "paused"
	TimerCompleted TimerState = "completed"
)

type Timer struct {
	State       TimerState
	Duration    time.Duration
	started     time.Time
	lastSeen    time.Time
	pausedAt    time.Time
	pausedTotal time.Duration
	uncertain   bool
}

func NewTimer(duration time.Duration) Timer { return Timer{State: TimerIdle, Duration: duration} }

func (t *Timer) Start(now time.Time) {
	t.State, t.started, t.lastSeen, t.pausedTotal, t.uncertain = TimerRunning, now, now, 0, false
}

func (t *Timer) Pause(now time.Time) {
	if t.State == TimerRunning {
		t.observe(now)
		if t.State == TimerPaused {
			now = t.pausedAt
		}
		t.State, t.pausedAt = TimerPaused, now
	}
}

func (t *Timer) Resume(now time.Time) {
	if t.State == TimerPaused {
		t.pausedTotal += now.Sub(t.pausedAt)
		t.State, t.lastSeen = TimerRunning, now
		t.uncertain = false
	}
}

func (t *Timer) Remaining(now time.Time) time.Duration {
	if t.State == TimerCompleted {
		return 0
	}
	if t.State == TimerIdle {
		return t.Duration
	}
	if t.State == TimerPaused {
		now = t.pausedAt
	}
	if t.State == TimerRunning {
		t.observe(now)
		if t.State == TimerPaused {
			now = t.pausedAt
		}
	}
	remaining := min(t.Duration, t.Duration-now.Sub(t.started)+t.pausedTotal)
	if remaining <= 0 {
		if !t.uncertain {
			t.State = TimerCompleted
		}
		return 0
	}
	return remaining
}

func (t *Timer) Active(now time.Time) time.Duration { return t.Duration - t.Remaining(now) }
func (t *Timer) Uncertain() bool                    { return t.uncertain }

func (t *Timer) observe(now time.Time) {
	if !t.lastSeen.IsZero() && now.UTC().Sub(t.lastSeen.UTC()) > 2*time.Minute {
		t.uncertain = true
		t.State = TimerPaused
		t.pausedAt = t.lastSeen
		return
	}
	t.lastSeen = now
}
