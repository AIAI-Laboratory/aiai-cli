package localdata

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type counter struct {
	Version int `json:"version"`
	Count   int `json:"count"`
}

func testFile(t *testing.T) File[counter] {
	t.Helper()
	return File[counter]{Path: filepath.Join(t.TempDir(), "data.json"), Empty: func() counter { return counter{Version: 1} }, Validate: func(c counter) error { return nil }}
}

func TestAtomicConcurrentUpdates(t *testing.T) {
	f := testFile(t)
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			if err := f.Update(context.Background(), func(c *counter) error { c.Count++; return nil }); err != nil {
				t.Errorf("update: %v", err)
			}
		})
	}
	group.Wait()
	data, err := f.Read(context.Background())
	if err != nil || data.Count != 12 {
		t.Fatalf("lost update %+v %v", data, err)
	}
	info, _ := os.Stat(f.Path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("unsafe permissions")
	}
}

func TestLockCancellationAndSymlinks(t *testing.T) {
	f := testFile(t)
	lock, err := os.OpenFile(f.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ok, err := tryLock(lock)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer unlock(lock)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err = f.Update(ctx, func(*counter) error { return nil }); err == nil {
		t.Fatal("lock wait ignored cancellation")
	}
	if runtime.GOOS == "windows" {
		return
	}
	target := filepath.Join(t.TempDir(), "target")
	if err = os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, f.Path); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Read(context.Background()); err == nil {
		t.Fatal("followed symlink")
	}
	payload, _ := os.ReadFile(target)
	if string(payload) != "preserve" {
		t.Fatal("target changed")
	}
}
