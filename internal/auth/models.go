package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnauthenticated = errors.New("not authenticated")
	ErrDenied          = errors.New("authorization denied")
	ErrExpired         = errors.New("authorization expired")
	ErrIO              = errors.New("authentication I/O failure")
	ErrNotFound        = errors.New("credential not found")
)

type User struct {
	ID        string `json:"id"`
	GitHubID  string `json:"github_id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type Credential struct {
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at,omitempty"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`
}

type StoredCredential struct {
	SchemaVersion int        `json:"schema_version"`
	Origin        string     `json:"origin"`
	Credential    Credential `json:"credential"`
	User          User       `json:"user"`
}

type DeviceAuthorization struct {
	AuthID             string    `json:"auth_id"`
	UserCode           string    `json:"user_code"`
	VerificationURI    string    `json:"verification_uri"`
	ExpiresAt          time.Time `json:"expires_at"`
	IntervalSeconds    int       `json:"interval_seconds"`
	RetryAfterSeconds  int       `json:"retry_after_seconds,omitempty"`
	BrowserOpenWarning string    `json:"-"`
}

type LoginOptions struct {
	Force     bool
	NoBrowser bool
}

type LoginStart struct {
	Existing *Result
	Device   DeviceAuthorization
}

type PollResult struct {
	Pending           bool
	RetryAfterSeconds int
	Credential        Credential
	User              User
}

type Result struct {
	Authenticated bool
	User          User
	Storage       string
	Warning       string
}

type LogoutResult struct {
	Warning string
}

type Authenticator interface {
	StartLogin(ctx context.Context, options LoginOptions) (LoginStart, error)
	PollLogin(ctx context.Context, attempt DeviceAuthorization) (PollResult, error)
	Login(ctx context.Context, options LoginOptions, ready func(DeviceAuthorization)) (Result, error)
	WhoAmI(ctx context.Context) (Result, error)
	RequireSession(ctx context.Context) (Credential, User, error)
	Cached() (Result, error)
	Logout(ctx context.Context) (LogoutResult, error)
}
