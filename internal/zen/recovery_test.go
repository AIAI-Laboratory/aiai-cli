package zen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyMigrationAndInterruptedRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "zen.json")
	now := time.Now().UTC()
	legacy := storageFile{SchemaVersion: 1, Config: DefaultConfig(), Sessions: []Session{{ID: "legacy", Type: SessionFocus, StartedAt: now, FinishedAt: now.Add(time.Minute), PlannedSeconds: 60, ActiveSeconds: 40, Status: StatusCompleted}}}
	payload, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, _ := NewJSONRepository(path)
	_, sessions, err := repo.Load(ctx)
	if err != nil || len(sessions) != 1 || sessions[0].ActiveMS != 40000 {
		t.Fatalf("legacy %+v %v", sessions, err)
	}
	if err = repo.SaveConfig(ctx, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	data, err := repo.file().Read(ctx)
	if err != nil || data.SchemaVersion != SchemaVersion || data.Sessions[0].PlannedMS != 60000 {
		t.Fatalf("migration %+v %v", data, err)
	}
	unfinished := Session{ID: "dead-owner", Type: SessionFocus, StartedAt: now, UpdatedAt: now.Add(10 * time.Second), Status: StatusRunning, PlannedSeconds: 1500, ActiveSeconds: 10}
	if err = repo.AddSession(ctx, unfinished); err != nil {
		t.Fatal(err)
	}
	_, sessions, err = repo.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sessions {
		if s.ID == unfinished.ID {
			found = true
			if s.Status != StatusInterrupted || s.ActiveSeconds != 10 || s.FinishedAt.IsZero() {
				t.Fatalf("bad recovery %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("session lost")
	}
}

func TestLiveOwnerAndTerminalRecordOrdering(t *testing.T) {
	ctx := context.Background()
	repo, _ := NewJSONRepository(filepath.Join(t.TempDir(), "zen.json"))
	now := time.Now().UTC()
	s := Session{ID: "live", Type: SessionFocus, StartedAt: now, UpdatedAt: now, PlannedSeconds: 60, Status: StatusRunning, OwnerPID: os.Getpid()}
	if err := repo.AddSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, items, err := repo.Load(ctx)
	if err != nil || items[0].Status != StatusRunning {
		t.Fatalf("live owner interrupted %+v %v", items, err)
	}
	final := s
	final.Status = StatusCompleted
	final.UpdatedAt = now.Add(time.Minute)
	final.FinishedAt = final.UpdatedAt
	final.ActiveSeconds = 60
	if err = repo.AddSession(ctx, final); err != nil {
		t.Fatal(err)
	}
	s.UpdatedAt = now.Add(2 * time.Minute)
	if err = repo.AddSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	_, items, err = repo.Load(ctx)
	if err != nil || len(items) != 1 || items[0].Status != StatusCompleted {
		t.Fatalf("terminal overwritten %+v %v", items, err)
	}
}

func TestTimerResumesAfterSleepAndFreezesCompletedTime(t *testing.T) {
	now := time.Now()
	timer := NewTimer(time.Minute)
	timer.Start(now)
	_ = timer.Remaining(now.Add(time.Second))
	_ = timer.Remaining(now.Add(time.Hour))
	if timer.State != TimerPaused || !timer.Uncertain() {
		t.Fatal("sleep not detected")
	}
	timer.Resume(now.Add(2 * time.Hour))
	if timer.Uncertain() {
		t.Fatal("resume still uncertain")
	}
	_ = timer.Remaining(now.Add(2*time.Hour + time.Minute))
	if timer.State != TimerCompleted || timer.Active(now.Add(24*time.Hour)) != time.Minute {
		t.Fatal("completed duration grew")
	}
}

func TestStatisticsDSTAndCompletedAverage(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	original := time.Local
	time.Local = location
	t.Cleanup(func() { time.Local = original })
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, location)
	stats := CalculateStatistics([]Session{{Type: SessionFocus, StartedAt: now, ActiveSeconds: 60, Status: StatusCompleted}, {Type: SessionFocus, StartedAt: now, ActiveSeconds: 30, Status: StatusInterrupted}}, now)
	if stats.Week[6].FocusSeconds != 90 || stats.Today.AverageCompleted != 60 || stats.AverageCompleted != 60 || stats.WeekFocusSeconds != 90 {
		t.Fatalf("statistics %+v", stats)
	}
}
