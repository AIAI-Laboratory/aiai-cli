package zen

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/AIAI-Laboratory/aiai-cli/internal/localdata"
)

var (
	ErrStorage        = errors.New("zen storage error")
	ErrInvalidConfig  = errors.New("invalid Zen configuration")
	ErrInvalidSession = errors.New("invalid Zen session")
)

type Repository interface {
	Load(context.Context) (Config, []Session, error)
	SaveConfig(context.Context, Config) error
	AddSession(context.Context, Session) error
}
type (
	JSONRepository struct{ Path string }
	storageFile    struct {
		SchemaVersion int       `json:"schema_version"`
		Config        Config    `json:"config"`
		Sessions      []Session `json:"sessions"`
	}
)

func DefaultStoragePath() (string, error) { return localdata.Path("zen.json") }
func NewJSONRepository(path string) (*JSONRepository, error) {
	if path == "" {
		var err error
		path, err = DefaultStoragePath()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &JSONRepository{Path: abs}, nil
}

func emptyStorage() storageFile {
	return storageFile{SchemaVersion: SchemaVersion, Config: DefaultConfig(), Sessions: []Session{}}
}

func (r *JSONRepository) file() localdata.File[storageFile] {
	return localdata.File[storageFile]{Path: r.Path, Empty: emptyStorage, Validate: func(data storageFile) error {
		if data.SchemaVersion != 1 && data.SchemaVersion != SchemaVersion {
			return fmt.Errorf("unsupported schema %d", data.SchemaVersion)
		}
		if err := data.Config.Validate(); err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, s := range data.Sessions {
			if err := validateSession(s); err != nil {
				return err
			}
			if ids[s.ID] {
				return errors.New("duplicate session ID")
			}
			ids[s.ID] = true
		}
		return nil
	}}
}
func active(s Session) bool { return s.Status == StatusRunning || s.Status == StatusPaused }

// Recovery is conservative: only a definitely dead owner is interrupted. We
// retain the last persisted active duration; an unobserved gap is never focus.
func stale(ctx context.Context, s Session) bool {
	if !active(s) {
		return false
	}
	if s.OwnerPID <= 0 {
		return true
	}
	alive, err := process.PidExistsWithContext(ctx, int32(s.OwnerPID))
	return err == nil && !alive
}

func (r *JSONRepository) Load(ctx context.Context) (Config, []Session, error) {
	data, err := r.file().Read(ctx)
	if err != nil {
		return Config{}, nil, fmt.Errorf("%w: %w", ErrStorage, err)
	}
	recoverNeeded := false
	for _, s := range data.Sessions {
		if stale(ctx, s) {
			recoverNeeded = true
			break
		}
	}
	if recoverNeeded {
		err = r.file().Update(ctx, func(d *storageFile) error {
			for i, s := range d.Sessions {
				if stale(ctx, s) {
					s.Status = StatusInterrupted
					s.FinishedAt = s.UpdatedAt
					if s.FinishedAt.IsZero() {
						s.FinishedAt = s.StartedAt
					}
					s.Observation = "Recovered unfinished session; only last persisted active time retained"
					d.Sessions[i] = s
				}
			}
			d.SchemaVersion = SchemaVersion
			data = *d
			return nil
		})
		if err != nil {
			return Config{}, nil, fmt.Errorf("%w: %w", ErrStorage, err)
		}
	}
	sessions := append([]Session{}, data.Sessions...)
	for i := range sessions {
		if data.SchemaVersion == 1 {
			sessions[i].PlannedMS = sessions[i].PlannedSeconds * 1000
			sessions[i].ActiveMS = sessions[i].ActiveSeconds * 1000
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartedAt.After(sessions[j].StartedAt) })
	return data.Config, sessions, nil
}

func (r *JSONRepository) update(ctx context.Context, fn func(*storageFile) error) error {
	err := r.file().Update(ctx, func(d *storageFile) error {
		if d.SchemaVersion == 1 {
			for i := range d.Sessions {
				d.Sessions[i].PlannedMS = d.Sessions[i].PlannedSeconds * 1000
				d.Sessions[i].ActiveMS = d.Sessions[i].ActiveSeconds * 1000
			}
		}
		d.SchemaVersion = SchemaVersion
		return fn(d)
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStorage, err)
	}
	return nil
}

func (r *JSONRepository) SaveConfig(ctx context.Context, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	return r.update(ctx, func(d *storageFile) error { d.Config = c; return nil })
}

// AddSession is an idempotent upsert. A delayed start/pause write cannot
// overwrite a terminal record; transition timestamps order concurrent updates.
func (r *JSONRepository) AddSession(ctx context.Context, s Session) error {
	if s.PlannedMS == 0 {
		s.PlannedMS = s.PlannedSeconds * 1000
	}
	if s.ActiveMS == 0 {
		s.ActiveMS = s.ActiveSeconds * 1000
	}
	if err := validateSession(s); err != nil {
		return err
	}
	return r.update(ctx, func(d *storageFile) error {
		for i, old := range d.Sessions {
			if old.ID == s.ID {
				if !active(old) && active(s) || old.UpdatedAt.After(s.UpdatedAt) {
					return nil
				}
				s = s.WithStartFrom(old)
				d.Sessions[i] = s
				return nil
			}
		}
		d.Sessions = append(d.Sessions, s)
		return nil
	})
}

func validateSession(s Session) error {
	if strings.TrimSpace(s.ID) == "" || s.PlannedSeconds < 0 || s.ActiveSeconds < 0 || s.PlannedMS < 0 || s.ActiveMS < 0 {
		return ErrInvalidSession
	}
	switch s.Type {
	case SessionFocus, SessionShortBreak, SessionLongBreak:
	default:
		return ErrInvalidSession
	}
	switch s.Status {
	case StatusCompleted, StatusInterrupted, StatusCanceled, StatusRunning, StatusPaused:
	default:
		return ErrInvalidSession
	}
	return nil
}
