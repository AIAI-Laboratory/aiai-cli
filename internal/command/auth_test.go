package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/auth"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

type fakeAuthenticator struct {
	result       auth.Result
	logoutResult auth.LogoutResult
	err          error
}

func (f fakeAuthenticator) StartLogin(context.Context, auth.LoginOptions) (auth.LoginStart, error) {
	return auth.LoginStart{}, f.err
}

func (f fakeAuthenticator) PollLogin(context.Context, auth.DeviceAuthorization) (auth.PollResult, error) {
	return auth.PollResult{}, f.err
}

func (f fakeAuthenticator) Login(_ context.Context, _ auth.LoginOptions, ready func(auth.DeviceAuthorization)) (auth.Result, error) {
	ready(auth.DeviceAuthorization{UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device"})
	return f.result, f.err
}

func (f fakeAuthenticator) WhoAmI(context.Context) (auth.Result, error) {
	return f.result, f.err
}

func (f fakeAuthenticator) RequireSession(context.Context) (auth.Credential, auth.User, error) {
	return auth.Credential{}, f.result.User, f.err
}
func (f fakeAuthenticator) Cached() (auth.Result, error) { return f.result, f.err }
func (f fakeAuthenticator) Logout(context.Context) (auth.LogoutResult, error) {
	return f.logoutResult, f.err
}

func TestLoginJSONContractAndTokenRedaction(t *testing.T) {
	user := auth.User{ID: "user-1", GitHubID: "42", Login: "octocat"}
	authenticator := fakeAuthenticator{result: auth.Result{Authenticated: true, User: user, Storage: "keyring"}}
	code, stdout, stderr := runWithAuth(t, []string{"login", "--json", "--no-browser"}, false, "", authenticator)
	if code != 0 || !strings.Contains(stderr, "ABCD-EFGH") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Auth == nil || !envelope.Auth.Authenticated || envelope.Auth.User.Login != "octocat" {
		t.Fatal(stdout)
	}
	if strings.Contains(stdout+stderr, "access-secret") || strings.Contains(stdout+stderr, "refresh-secret") {
		t.Fatal("credential leaked to output")
	}
}

func TestWhoamiAndLogoutErrors(t *testing.T) {
	code, stdout, _ := runWithAuth(t, []string{"whoami", "--json"}, false, "", fakeAuthenticator{err: auth.ErrUnauthenticated})
	if code != 2 || !strings.Contains(stdout, "run aiai login") {
		t.Fatalf("code=%d output=%s", code, stdout)
	}
	remote := fmtAuthIO("offline")
	code, stdout, _ = runWithAuth(t, []string{"logout", "--json"}, false, "", fakeAuthenticator{logoutResult: auth.LogoutResult{Warning: "local credentials deleted"}, err: remote})
	if code != 4 {
		t.Fatalf("code=%d output=%s", code, stdout)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Auth == nil || envelope.Auth.Authenticated || envelope.Auth.Warning == "" {
		t.Fatal(stdout)
	}
}

func TestAuthExitCodes(t *testing.T) {
	if got := ExitCode(fmtAuthIO("network")); got != 4 {
		t.Fatal(got)
	}
	if got := ExitCode(context.Canceled); got != 130 {
		t.Fatal(got)
	}
}

func fmtAuthIO(message string) error {
	return errors.Join(auth.ErrIO, errors.New(message))
}
