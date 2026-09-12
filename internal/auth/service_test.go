package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAPI struct {
	now         time.Time
	polls       int
	refreshes   int
	refreshErr  error
	logoutError error
}

func (f *fakeAPI) StartDevice(context.Context) (DeviceAuthorization, error) {
	return DeviceAuthorization{AuthID: "flow", UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device", ExpiresAt: f.now.Add(time.Minute), IntervalSeconds: 1}, nil
}

func (f *fakeAPI) PollDevice(context.Context, string) (PollResult, error) {
	f.polls++
	if f.polls == 1 {
		return PollResult{Pending: true, RetryAfterSeconds: 2}, nil
	}
	return PollResult{Credential: testCredential(f.now), User: testUser()}, nil
}

func (f *fakeAPI) Refresh(context.Context, string) (Credential, User, error) {
	f.refreshes++
	if f.refreshErr != nil {
		return Credential{}, User{}, f.refreshErr
	}
	return testCredential(f.now), testUser(), nil
}

func TestRevokedRefreshDeletesCredential(t *testing.T) {
	now := time.Now().UTC()
	api := &fakeAPI{now: now, refreshErr: ErrUnauthenticated}
	store := &memoryStore{storage: "keyring", values: map[string]StoredCredential{
		"https://api.example.com": {SchemaVersion: 1, Origin: "https://api.example.com", Credential: Credential{AccessToken: "old", AccessTokenExpiresAt: now.Add(-time.Minute), RefreshToken: "revoked"}, User: testUser()},
	}}
	service := NewService(api, store, "https://api.example.com")
	service.Now = func() time.Time { return now }
	if _, err := service.WhoAmI(context.Background()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
	if _, _, err := store.Load("https://api.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked credential was not deleted")
	}
}

func TestCanceledLogoutDeletesLocalCredential(t *testing.T) {
	now := time.Now().UTC()
	api := &fakeAPI{now: now, logoutError: context.Canceled}
	store := &memoryStore{storage: "keyring", values: map[string]StoredCredential{
		"https://api.example.com": {SchemaVersion: 1, Origin: "https://api.example.com", Credential: testCredential(now), User: testUser()},
	}}
	service := NewService(api, store, "https://api.example.com")
	result, err := service.Logout(context.Background())
	if !errors.Is(err, context.Canceled) || result.Warning == "" {
		t.Fatal(result, err)
	}
	if _, _, err := store.Load("https://api.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatal("canceled logout did not delete local credential")
	}
}

func (f *fakeAPI) Me(context.Context, string) (User, error) { return testUser(), nil }
func (f *fakeAPI) Logout(context.Context, string) error     { return f.logoutError }

type memoryStore struct {
	values  map[string]StoredCredential
	storage string
}

func (m *memoryStore) Load(origin string) (StoredCredential, string, error) {
	value, ok := m.values[origin]
	if !ok {
		return StoredCredential{}, "", ErrNotFound
	}
	return value, m.storage, nil
}

func (m *memoryStore) Save(origin string, value StoredCredential) (string, error) {
	if m.values == nil {
		m.values = map[string]StoredCredential{}
	}
	m.values[origin] = value
	return m.storage, nil
}

func (m *memoryStore) Delete(origin string) error {
	if _, ok := m.values[origin]; !ok {
		return ErrNotFound
	}
	delete(m.values, origin)
	return nil
}

func TestServiceLoginPollingAndFallbackWarning(t *testing.T) {
	now := time.Now().UTC()
	api := &fakeAPI{now: now}
	store := &memoryStore{values: map[string]StoredCredential{}, storage: "file"}
	service := NewService(api, store, "https://api.example.com")
	service.Now = func() time.Time { return now }
	var waits []time.Duration
	service.Wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	service.OpenBrowser = func(string) error { return errors.New("no browser") }
	ready := false
	result, err := service.Login(context.Background(), LoginOptions{}, func(device DeviceAuthorization) {
		ready = device.UserCode == "ABCD-EFGH"
	})
	if err != nil || !ready || result.User.Login != "octocat" || result.Storage != "file" {
		t.Fatal(result, err)
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Fatalf("unexpected polling cadence: %v", waits)
	}
	if result.Warning == "" || api.polls != 2 {
		t.Fatal("missing fallback/browser warning or polls")
	}
}

func TestServiceRefreshWhoamiAndPartialLogout(t *testing.T) {
	now := time.Now().UTC()
	api := &fakeAPI{now: now, logoutError: errors.New("offline")}
	store := &memoryStore{storage: "keyring", values: map[string]StoredCredential{
		"https://api.example.com": {SchemaVersion: 1, Origin: "https://api.example.com", Credential: Credential{AccessToken: "old", AccessTokenExpiresAt: now.Add(-time.Minute), RefreshToken: "refresh"}, User: testUser()},
	}}
	service := NewService(api, store, "https://api.example.com")
	service.Now = func() time.Time { return now }
	result, err := service.WhoAmI(context.Background())
	if err != nil || result.User.Login != "octocat" || api.refreshes != 1 {
		t.Fatal(result, err)
	}
	logout, err := service.Logout(context.Background())
	if !errors.Is(err, ErrIO) || logout.Warning == "" {
		t.Fatal(logout, err)
	}
	if _, _, err := store.Load("https://api.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatal("local credential was not deleted")
	}
}

func TestServiceNonExpiringGitHubToken(t *testing.T) {
	now := time.Now().UTC()
	api := &fakeAPI{now: now}
	store := &memoryStore{storage: "keyring", values: map[string]StoredCredential{
		DefaultGitHubOrigin: {
			SchemaVersion: 1,
			Origin:        DefaultGitHubOrigin,
			Credential:    Credential{AccessToken: "gho_token"},
			User:          testUser(),
		},
	}}
	service := NewService(api, store, "")
	service.Now = func() time.Time { return now }

	cred, user, err := service.RequireSession(context.Background())
	if err != nil || cred.AccessToken != "gho_token" || user.Login != "octocat" {
		t.Fatalf("expected valid session, got cred=%+v, user=%+v, err=%v", cred, user, err)
	}
	if api.refreshes != 0 {
		t.Fatalf("expected no refresh calls for non-expiring token, got %d", api.refreshes)
	}

	res, err := service.WhoAmI(context.Background())
	if err != nil || !res.Authenticated || res.User.Login != "octocat" {
		t.Fatalf("expected authenticated result, got %+v, err=%v", res, err)
	}
}
