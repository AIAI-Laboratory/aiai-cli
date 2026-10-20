package ssh

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	result RunResult
	err    error
	args   []string
}

func (f *fakeRunner) Run(_ context.Context, args []string, _ Streams) (RunResult, error) {
	f.args = append([]string(nil), args...)
	return f.result, f.err
}

func newTestService(t *testing.T, runner Runner) (*Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aiai", "ssh.json")
	repository, err := NewJSONRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, runner)
	return service, path
}

func TestDefaultStoragePathHonorsConfigDirectory(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "portable-config")
	t.Setenv("AIAI_CONFIG_DIR", configured)

	path, err := DefaultStoragePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configured, "ssh.json")
	if path != want {
		t.Fatalf("DefaultStoragePath() = %q, want %q", path, want)
	}
}

func TestServerCRUDAndPersistence(t *testing.T) {
	service, path := newTestService(t, &fakeRunner{})
	created, err := service.Add(context.Background(), AddRequest{Alias: "Production", Host: "192.0.2.10", Username: "ubuntu"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Alias != "production" || created.Port != 22 {
		t.Fatalf("unexpected server: %+v", created)
	}
	if runtime.GOOS != "windows" {
		if info, statErr := os.Stat(path); statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("storage mode = %v, %v", info, statErr)
		}
		if info, statErr := os.Stat(filepath.Dir(path)); statErr != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("storage directory mode = %v, %v", info, statErr)
		}
	}
	if _, err := service.Add(context.Background(), AddRequest{Alias: "production", Host: "example.com", Username: "deploy"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}
	port := 2222
	description := "primary"
	updated, err := service.Update(context.Background(), "production", UpdateRequest{Port: &port, Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Port != 2222 || updated.Description != "primary" || updated.Host != created.Host {
		t.Fatalf("partial update lost fields: %+v", updated)
	}

	reopened, err := NewJSONRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := NewService(reopened, &fakeRunner{}).Show(context.Background(), "production")
	if err != nil || persisted.Port != 2222 {
		t.Fatalf("persisted server = %+v, %v", persisted, err)
	}
	if err := service.Remove(context.Background(), "production"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Show(context.Background(), "production"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed server error = %v", err)
	}
}

func TestKeyReferencesAndMissingKeyAtConnect(t *testing.T) {
	runner := &fakeRunner{}
	service, _ := newTestService(t, runner)
	if _, err := service.Add(context.Background(), AddRequest{Alias: "server", Host: "example.com", Username: "deploy"}); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(key, []byte("reference-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := service.SetKey(context.Background(), "server", key)
	if err != nil || server.IdentityFile != key {
		t.Fatalf("SetKey() = %+v, %v", server, err)
	}
	payload, err := os.ReadFile(service.Repository.(*JSONRepository).Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "reference-only") {
		t.Fatal("private key content was persisted")
	}
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	if err := service.Connect(context.Background(), "server", Streams{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("Connect() missing key error = %v", err)
	}
	logs, err := service.Logs(context.Background(), LogFilter{ServerAlias: "server", Limit: 10})
	if err != nil || len(logs) != 1 || logs[0].Status != StatusFailed {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
	if _, err := service.ClearKey(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
}

func TestConnectLogsAndExitStatus(t *testing.T) {
	runner := &fakeRunner{result: RunResult{ExitCode: 42}}
	service, _ := newTestService(t, runner)
	if _, err := service.Add(context.Background(), AddRequest{Alias: "prod", Host: "example.com", Port: 2222, Username: "deploy"}); err != nil {
		t.Fatal(err)
	}
	err := service.Connect(context.Background(), "prod", Streams{In: stringsReader{}, Out: io.Discard, Err: io.Discard})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 42 {
		t.Fatalf("Connect() error = %v", err)
	}
	wantArgs := []string{"-p", "2222", "-l", "deploy", "--", "example.com"}
	if len(runner.args) != len(wantArgs) {
		t.Fatalf("runner args = %#v", runner.args)
	}
	for i := range wantArgs {
		if runner.args[i] != wantArgs[i] {
			t.Fatalf("runner args = %#v", runner.args)
		}
	}
	logs, err := service.Logs(context.Background(), LogFilter{Limit: 10})
	if err != nil || len(logs) != 1 || logs[0].Status != StatusFailed || logs[0].ExitCode == nil || *logs[0].ExitCode != 42 || logs[0].EndedAt == nil {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
	if err := service.Remove(context.Background(), "prod"); err != nil {
		t.Fatal(err)
	}
	logs, err = service.Logs(context.Background(), LogFilter{ServerAlias: "prod", Limit: 10})
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs after removal = %+v, %v", logs, err)
	}
}

func TestConnectCancellation(t *testing.T) {
	runner := &fakeRunner{err: context.Canceled}
	service, _ := newTestService(t, runner)
	if _, err := service.Add(context.Background(), AddRequest{Alias: "prod", Host: "example.com", Username: "deploy"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Connect(context.Background(), "prod", Streams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect() error = %v", err)
	}
	logs, err := service.Logs(context.Background(), LogFilter{Limit: 1})
	if err != nil || len(logs) != 1 || logs[0].Status != StatusCanceled {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
}

func TestJSONRepositoryConcurrentWrites(t *testing.T) {
	service, _ := newTestService(t, &fakeRunner{})
	var wait sync.WaitGroup
	errorsCh := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			_, err := service.Add(context.Background(), AddRequest{Alias: "server-" + itoa(i), Host: "host-" + itoa(i) + ".example.com", Username: "deploy"})
			errorsCh <- err
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	servers, err := service.List(context.Background())
	if err != nil || len(servers) != 12 {
		t.Fatalf("servers = %d, %v", len(servers), err)
	}
}

func TestAbandonedLogIsRecoveredAsCanceled(t *testing.T) {
	service, path := newTestService(t, &fakeRunner{})
	repository := service.Repository.(*JSONRepository)
	started := time.Now().UTC().Add(-time.Minute)
	data := emptyStorage()
	data.AccessLogs = append(data.AccessLogs, storedAccessLog{AccessLog: AccessLog{ID: "abandoned", ServerAlias: "gone", Host: "example.com", Username: "user", StartedAt: started, Status: StatusPending}, OwnerPID: 99999999})
	if err := repository.ensureDirectory(); err != nil {
		t.Fatal(err)
	}
	if err := repository.write(data); err != nil {
		t.Fatal(err)
	}
	logs, err := service.Logs(context.Background(), LogFilter{Limit: 10})
	if err != nil || len(logs) != 1 || logs[0].Status != StatusCanceled || logs[0].EndedAt == nil {
		t.Fatalf("recovered logs from %s = %+v, %v", path, logs, err)
	}
}

func TestJSONRepositoryRejectsCorruptionAndSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aiai", "ssh.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := NewJSONRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ListServers(context.Background()); !errors.Is(err, ErrStorage) {
		t.Fatalf("corrupt storage error = %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":1,"servers":[],"ssh_access_logs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ListServers(context.Background()); !errors.Is(err, ErrStorage) {
		t.Fatalf("symlink storage error = %v", err)
	}
}

type stringsReader struct{}

func (stringsReader) Read([]byte) (int, error) { return 0, io.EOF }
