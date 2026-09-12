package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	values map[string]string
	err    error
}

func (f *fakeKeyring) Get(_, account string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeKeyring) Set(_, account, secret string) error {
	if f.err != nil {
		return f.err
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[account] = secret
	return nil
}

func (f *fakeKeyring) Delete(_, account string) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.values[account]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, account)
	return nil
}

func TestFileStoreRoundTripPermissionsAndOrigins(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "aiai")
	store := &FileStore{Dir: dir}
	now := time.Now().UTC()
	first := StoredCredential{Credential: testCredential(now), User: testUser()}
	second := first
	second.User.Login = "hubot"
	if err := store.Save("https://one.example", first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("https://two.example", second); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("https://one.example")
	if err != nil || loaded.User.Login != "octocat" {
		t.Fatal(loaded, err)
	}
	if runtime.GOOS != "windows" {
		dirInfo, _ := os.Stat(dir)
		fileInfo, _ := os.Stat(filepath.Join(dir, "credentials.json"))
		if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("permissions: dir=%o file=%o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
		}
	}
	if err := store.Delete("https://one.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("https://two.example"); err != nil {
		t.Fatal("deleting one origin removed another", err)
	}
}

func TestFileStoreRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{Dir: dir}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(`{"schema_version":1,"credentials":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "credentials.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := store.Load("https://api.example.com"); !errors.Is(err, ErrIO) {
		t.Fatalf("expected unsafe path error, got %v", err)
	}
}

func TestFileStoreRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file permissions are ACL-based")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"credentials":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &FileStore{Dir: dir}
	if _, err := store.Load("https://api.example.com"); !errors.Is(err, ErrIO) {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestHybridStoreFallsBackAndMigrates(t *testing.T) {
	dir := t.TempDir()
	ring := &fakeKeyring{err: errors.New("keyring unavailable")}
	store := &HybridStore{Keyring: ring, File: &FileStore{Dir: dir}}
	origin := "https://api.example.com"
	credential := StoredCredential{Credential: testCredential(time.Now()), User: testUser()}
	storage, err := store.Save(origin, credential)
	if err != nil || storage != "file" {
		t.Fatal(storage, err)
	}
	ring.err = nil
	loaded, storage, err := store.Load(origin)
	if err != nil || storage != "keyring" || loaded.User.Login != "octocat" {
		t.Fatal(loaded, storage, err)
	}
	if _, err := store.File.Load(origin); !errors.Is(err, ErrNotFound) {
		t.Fatal("fallback credential was not removed after migration")
	}
}
