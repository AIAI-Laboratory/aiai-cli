package zen

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJSONRepositoryPersistsAndOrdersSessions(t *testing.T) {
	repo, err := NewJSONRepository(filepath.Join(t.TempDir(), "aiai", "zen.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	config := DefaultConfig()
	config.ReducedMotion = true
	if err := repo.SaveConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	for _, started := range []time.Time{time.Unix(10, 0), time.Unix(20, 0)} {
		if err := repo.AddSession(ctx, Session{ID: started.String(), Type: SessionFocus, StartedAt: started, FinishedAt: started.Add(time.Minute), PlannedSeconds: 60, ActiveSeconds: 60, Status: StatusCompleted, CreatedAt: started}); err != nil {
			t.Fatal(err)
		}
	}
	loaded, sessions, err := repo.Load(ctx)
	if err != nil || !loaded.ReducedMotion || len(sessions) != 2 || !sessions[0].StartedAt.After(sessions[1].StartedAt) {
		t.Fatalf("load config=%+v sessions=%+v err=%v", loaded, sessions, err)
	}
	info, err := os.Stat(repo.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("unsafe permissions: %o", info.Mode().Perm())
	}
}

func TestJSONRepositoryRejectsCorruptData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zen.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo, _ := NewJSONRepository(path)
	_, _, err := repo.Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "corrupt JSON") {
		t.Fatalf("error = %v", err)
	}
	payload, _ := os.ReadFile(path)
	if string(payload) != "{broken" {
		t.Fatal("corrupt data was overwritten")
	}
}

func TestJSONRepositoryConcurrentWrites(t *testing.T) {
	repo, _ := NewJSONRepository(filepath.Join(t.TempDir(), "zen.json"))
	var group sync.WaitGroup
	for i := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			now := time.Unix(int64(i), 0)
			if err := repo.AddSession(context.Background(), Session{ID: now.String(), Type: SessionFocus, StartedAt: now, Status: StatusInterrupted}); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	group.Wait()
	_, sessions, err := repo.Load(context.Background())
	if err != nil || len(sessions) != 8 {
		t.Fatalf("sessions=%d err=%v", len(sessions), err)
	}
}
