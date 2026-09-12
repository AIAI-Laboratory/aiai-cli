package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitHubClientContract(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var body gitHubDeviceCodeRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ClientID != "test-client-id" {
				t.Fatalf("unexpected client_id: %s", body.ClientID)
			}
			_ = json.NewEncoder(w).Encode(gitHubDeviceCodeResponse{
				DeviceCode:      "device-flow-code",
				UserCode:        "WDJB-NJTJ",
				VerificationURI: "https://github.com/login/device",
				ExpiresIn:       900,
				Interval:        5,
			})
		case "/login/oauth/access_token":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var body gitHubTokenRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.GrantType == "refresh_token" {
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					AccessToken: "refreshed-token",
					TokenType:   "bearer",
					Scope:       "read:user",
				})
				return
			}
			polls++
			if polls == 1 {
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					Error:            "authorization_pending",
					ErrorDescription: "The authorization request is still pending.",
					Interval:         5,
				})
				return
			}
			if polls == 2 {
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					Error:            "slow_down",
					ErrorDescription: "To avoid hitting rate limits, wait at least 5 more seconds.",
					Interval:         5,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
				AccessToken: "gho_test_access_token",
				TokenType:   "bearer",
				Scope:       "read:user",
			})
		case "/user":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer gho_test_access_token" && authHeader != "Bearer refreshed-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(gitHubUserResponse{
				ID:        583231,
				Login:     "octocat",
				Name:      "The Octocat",
				AvatarURL: "https://avatars.githubusercontent.com/u/583231",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := Client{
		ClientID:  "test-client-id",
		BaseURL:   server.URL,
		HTTP:      server.Client(),
		UserAgent: "test-agent",
	}

	// 1. StartDevice
	device, err := client.StartDevice(context.Background())
	if err != nil {
		t.Fatalf("StartDevice failed: %v", err)
	}
	if device.AuthID != "device-flow-code" || device.UserCode != "WDJB-NJTJ" || device.IntervalSeconds != 5 {
		t.Fatalf("unexpected device auth: %+v", device)
	}

	// 2. PollDevice: authorization_pending
	resPending, err := client.PollDevice(context.Background(), "device-flow-code")
	if err != nil || !resPending.Pending || resPending.RetryAfterSeconds != 5 {
		t.Fatalf("expected pending poll result, got %+v (err: %v)", resPending, err)
	}

	// 3. PollDevice: slow_down
	resSlowDown, err := client.PollDevice(context.Background(), "device-flow-code")
	if err != nil || !resSlowDown.Pending || resSlowDown.RetryAfterSeconds != 10 {
		t.Fatalf("expected slow_down with 10s retry, got %+v (err: %v)", resSlowDown, err)
	}

	// 4. PollDevice: authorized
	resAuthorized, err := client.PollDevice(context.Background(), "device-flow-code")
	if err != nil || resAuthorized.Pending {
		t.Fatalf("expected authorized poll result, got %+v (err: %v)", resAuthorized, err)
	}
	if resAuthorized.Credential.AccessToken != "gho_test_access_token" {
		t.Fatalf("unexpected access token: %s", resAuthorized.Credential.AccessToken)
	}
	if resAuthorized.User.Login != "octocat" || resAuthorized.User.GitHubID != "583231" || resAuthorized.User.ID != "583231" {
		t.Fatalf("unexpected user: %+v", resAuthorized.User)
	}

	// 5. Me
	user, err := client.Me(context.Background(), "gho_test_access_token")
	if err != nil || user.Login != "octocat" {
		t.Fatalf("expected octocat user, got %+v (err: %v)", user, err)
	}

	// 6. Refresh
	cred, refUser, err := client.Refresh(context.Background(), "dummy-refresh")
	if err != nil || cred.AccessToken != "refreshed-token" || refUser.Login != "octocat" {
		t.Fatalf("expected refresh success, got %+v, %+v (err: %v)", cred, refUser, err)
	}

	// 7. Logout
	if err := client.Logout(context.Background(), "dummy-refresh"); err != nil {
		t.Fatalf("expected logout success, got %v", err)
	}
}

func TestGitHubClientErrorsAndValidation(t *testing.T) {
	// Missing client ID
	emptyClient := Client{}
	if _, err := emptyClient.StartDevice(context.Background()); err == nil {
		t.Fatal("expected error when ClientID is empty")
	}
	if _, err := emptyClient.PollDevice(context.Background(), "code"); err == nil {
		t.Fatal("expected error when ClientID is empty")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/login/oauth/access_token":
			var req gitHubTokenRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.DeviceCode == "expired-code" {
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					Error:            "expired_token",
					ErrorDescription: "The device_code has expired.",
				})
				return
			}
			if req.DeviceCode == "denied-code" {
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					Error:            "access_denied",
					ErrorDescription: "The user has denied the request.",
				})
				return
			}
		case "/user":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
		}
	}))
	defer server.Close()

	client := Client{
		ClientID:  "test-client",
		BaseURL:   server.URL,
		HTTP:      server.Client(),
		UserAgent: "test",
	}

	// expired_token
	if _, err := client.PollDevice(context.Background(), "expired-code"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired, got %v", err)
	}

	// access_denied
	if _, err := client.PollDevice(context.Background(), "denied-code"); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected ErrDenied, got %v", err)
	}

	// Unauthorized Me
	if _, err := client.Me(context.Background(), "invalid-token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestResolveGitHubClientID(t *testing.T) {
	t.Setenv("AIAI_GITHUB_CLIENT_ID", "env-aiai-id")
	t.Setenv("GITHUB_CLIENT_ID", "env-github-id")
	if got := ResolveGitHubClientID("injected-id"); got != "env-aiai-id" {
		t.Fatalf("expected env-aiai-id, got %s", got)
	}

	t.Setenv("AIAI_GITHUB_CLIENT_ID", "")
	if got := ResolveGitHubClientID("injected-id"); got != "env-github-id" {
		t.Fatalf("expected env-github-id, got %s", got)
	}

	t.Setenv("GITHUB_CLIENT_ID", "")
	if got := ResolveGitHubClientID("injected-id"); got != "injected-id" {
		t.Fatalf("expected injected-id, got %s", got)
	}

	if got := ResolveGitHubClientID(""); got != DefaultGitHubClientID {
		t.Fatalf("expected DefaultGitHubClientID, got %s", got)
	}
}

func testCredential(now time.Time) Credential {
	return Credential{AccessToken: "access", AccessTokenExpiresAt: now.Add(time.Hour), RefreshToken: "refresh", RefreshTokenExpiresAt: now.Add(24 * time.Hour)}
}

func testUser() User {
	return User{ID: "user-1", GitHubID: "42", Login: "octocat", Name: "The Octocat"}
}
