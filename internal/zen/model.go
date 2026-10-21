package zen

import "time"

const SchemaVersion = 2

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
	StatusRunning     SessionStatus = "running"
	StatusPaused      SessionStatus = "paused"
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
	ProjectID      string        `json:"project_id,omitempty"`
	ProjectName    string        `json:"project_name,omitempty"`
	RepositoryID   string        `json:"repository_id,omitempty"`
	WorktreePath   string        `json:"worktree_path,omitempty"`
	BranchAtStart  string        `json:"branch_at_start,omitempty"`
	BranchAtEnd    string        `json:"branch_at_end,omitempty"`
	StartHead      string        `json:"start_head_commit,omitempty"`
	EndHead        string        `json:"end_head_commit,omitempty"`
	PlannedMS      int64         `json:"planned_duration_ms"`
	ActiveMS       int64         `json:"actual_active_duration_ms"`
	UpdatedAt      time.Time     `json:"updated_at,omitempty"`
	OwnerPID       int           `json:"owner_pid,omitempty"`
	Observation    string        `json:"git_observation,omitempty"`
	GitSnapshotAt  time.Time     `json:"git_start_snapshot_at,omitempty"`
}

// WithStartFrom keeps the first observed Git snapshot immutable across delayed
// asynchronous pause/resume writes. Snapshot time is not claimed as commit time.
func (s Session) WithStartFrom(old Session) Session {
	if !old.GitSnapshotAt.IsZero() && (s.GitSnapshotAt.IsZero() || !old.GitSnapshotAt.After(s.GitSnapshotAt)) {
		s.StartHead, s.BranchAtStart, s.GitSnapshotAt = old.StartHead, old.BranchAtStart, old.GitSnapshotAt
		if s.RepositoryID == "" {
			s.RepositoryID = old.RepositoryID
		}
	}
	return s
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
	if c.WorkSeconds < 1 || c.BreakSeconds < 1 || c.LongBreakSeconds < 1 || c.WorkSeconds > 86400 || c.BreakSeconds > 86400 || c.LongBreakSeconds > 86400 {
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
	completedSeconds int64
}

type Statistics struct {
	Today            DayStat   `json:"today"`
	Week             []DayStat `json:"week"`
	WeekFocusSeconds int64     `json:"week_focus_seconds"`
	WeekCompleted    int       `json:"week_completed"`
	AverageCompleted int64     `json:"week_average_completed_seconds"`
}
