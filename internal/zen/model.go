package zen

import "time"

const SchemaVersion = 1

type SessionType string

const (
	SessionFocus      SessionType = "focus"
	SessionShortBreak SessionType = "short_break"
	SessionLongBreak  SessionType = "long_break"
)

type SessionStatus string

const (
	StatusCompleted   SessionStatus = "completed"
	StatusInterrupted SessionStatus = "interrupted"
	StatusCanceled    SessionStatus = "canceled"
)

type Session struct {
	ID             string        `json:"id"`
	Type           SessionType   `json:"type"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	PlannedSeconds int64         `json:"planned_seconds"`
	ActiveSeconds  int64         `json:"active_seconds"`
	Status         SessionStatus `json:"status"`
	Label          string        `json:"label,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

type Config struct {
	WorkSeconds      int64 `json:"work_seconds"`
	BreakSeconds     int64 `json:"break_seconds"`
	LongBreakSeconds int64 `json:"long_break_seconds"`
	Cycles           int   `json:"cycles"`
	AutoStart        bool  `json:"auto_start"`
	ReducedMotion    bool  `json:"reduced_motion"`
	Unicode          bool  `json:"unicode"`
}

func DefaultConfig() Config {
	return Config{WorkSeconds: 25 * 60, BreakSeconds: 5 * 60, LongBreakSeconds: 15 * 60, Cycles: 4, Unicode: true}
}

func (c Config) Validate() error {
	if c.WorkSeconds < 1 || c.BreakSeconds < 1 || c.LongBreakSeconds < 1 {
		return ErrInvalidConfig
	}
	if c.Cycles < 1 || c.Cycles > 24 {
		return ErrInvalidConfig
	}
	return nil
}

type DayStat struct {
	Date             string `json:"date"`
	FocusSeconds     int64  `json:"focus_seconds"`
	Completed        int    `json:"completed"`
	Interrupted      int    `json:"interrupted"`
	CompletionRate   int    `json:"completion_rate"`
	AverageCompleted int64  `json:"average_completed_seconds"`
}

type Statistics struct {
	Today DayStat   `json:"today"`
	Week  []DayStat `json:"week"`
}
