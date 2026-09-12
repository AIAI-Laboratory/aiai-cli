package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultGitHubClientID  = "Ov23litbDiAsvS9VB4Dy"
	DefaultGitHubOrigin    = "https://github.com"
	DefaultDeviceCodeURL   = "https://github.com/login/device/code"
	DefaultTokenURL        = "https://github.com/login/oauth/access_token"
	DefaultUserAPIURL      = "https://api.github.com/user"
	DefaultPublicUserScope = "read:user"
)

type API interface {
	StartDevice(context.Context) (DeviceAuthorization, error)
	PollDevice(context.Context, string) (PollResult, error)
	Refresh(context.Context, string) (Credential, User, error)
	Me(context.Context, string) (User, error)
	Logout(context.Context, string) error
}

type Client struct {
	ClientID      string
	Scopes        []string
	DeviceCodeURL string
	TokenURL      string
	UserAPIURL    string
	BaseURL       string
	HTTP          *http.Client
	UserAgent     string
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

type gitHubDeviceCodeRequest struct {
	ClientID string `json:"client_id"`
	Scope    string `json:"scope,omitempty"`
}

type gitHubDeviceCodeResponse struct {
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	ExpiresIn        int    `json:"expires_in"`
	Interval         int    `json:"interval"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

type gitHubTokenRequest struct {
	ClientID   string `json:"client_id"`
	DeviceCode string `json:"device_code"`
	GrantType  string `json:"grant_type"`
}

type gitHubTokenResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	ExpiresIn             int    `json:"expires_in,omitempty"`
	RefreshToken          string `json:"refresh_token,omitempty"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in,omitempty"`
	Error                 string `json:"error,omitempty"`
	ErrorDescription      string `json:"error_description,omitempty"`
	Interval              int    `json:"interval,omitempty"`
}

type gitHubUserResponse struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (c Client) StartDevice(ctx context.Context) (DeviceAuthorization, error) {
	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		return DeviceAuthorization{}, fmt.Errorf("%w: GitHub client ID is not configured; set AIAI_GITHUB_CLIENT_ID", ErrIO)
	}
	scope := DefaultPublicUserScope
	if len(c.Scopes) > 0 {
		scope = strings.Join(c.Scopes, " ")
	}
	reqBody := gitHubDeviceCodeRequest{
		ClientID: clientID,
		Scope:    scope,
	}
	var resp gitHubDeviceCodeResponse
	_, err := c.doJSON(ctx, http.MethodPost, c.deviceCodeEndpoint(), "", reqBody, &resp, http.StatusOK)
	if err != nil {
		return DeviceAuthorization{}, err
	}
	if resp.Error != "" {
		msg := resp.ErrorDescription
		if msg == "" {
			msg = resp.Error
		}
		return DeviceAuthorization{}, fmt.Errorf("%w: %s", ErrIO, msg)
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return DeviceAuthorization{}, fmt.Errorf("%w: invalid device authorization response from GitHub", ErrIO)
	}
	verificationURI := resp.VerificationURI
	if verificationURI == "" {
		verificationURI = "https://github.com/login/device"
	}
	if !validVerificationURI(verificationURI) {
		return DeviceAuthorization{}, fmt.Errorf("%w: invalid verification URI %q", ErrIO, verificationURI)
	}
	interval := resp.Interval
	if interval < 5 {
		interval = 5
	}
	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	return DeviceAuthorization{
		AuthID:          resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: verificationURI,
		ExpiresAt:       time.Now().UTC().Add(time.Duration(expiresIn) * time.Second),
		IntervalSeconds: interval,
	}, nil
}

func (c Client) PollDevice(ctx context.Context, authID string) (PollResult, error) {
	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		return PollResult{}, fmt.Errorf("%w: GitHub client ID is not configured; set AIAI_GITHUB_CLIENT_ID", ErrIO)
	}
	reqBody := gitHubTokenRequest{
		ClientID:   clientID,
		DeviceCode: authID,
		GrantType:  "urn:ietf:params:oauth:grant-type:device_code",
	}
	var tokenResp gitHubTokenResponse
	_, err := c.doJSON(ctx, http.MethodPost, c.tokenEndpoint(), "", reqBody, &tokenResp, http.StatusOK, http.StatusBadRequest)
	if err != nil {
		return PollResult{}, err
	}

	switch tokenResp.Error {
	case "":
		// authorization granted, proceed below
	case "authorization_pending":
		interval := tokenResp.Interval
		if interval < 1 {
			interval = 5
		}
		return PollResult{Pending: true, RetryAfterSeconds: interval}, nil
	case "slow_down":
		interval := tokenResp.Interval
		if interval < 1 {
			interval = 10
		} else {
			interval += 5
		}
		return PollResult{Pending: true, RetryAfterSeconds: interval}, nil
	case "expired_token":
		return PollResult{}, ErrExpired
	case "access_denied":
		return PollResult{}, ErrDenied
	default:
		msg := tokenResp.ErrorDescription
		if msg == "" {
			msg = tokenResp.Error
		}
		return PollResult{}, fmt.Errorf("%w: GitHub OAuth: %s", ErrIO, msg)
	}

	if tokenResp.AccessToken == "" {
		return PollResult{}, fmt.Errorf("%w: missing access token in GitHub response", ErrIO)
	}

	user, err := c.Me(ctx, tokenResp.AccessToken)
	if err != nil {
		return PollResult{}, fmt.Errorf("%w: fetch GitHub user profile: %v", ErrIO, err)
	}

	now := time.Now().UTC()
	var accessExpiresAt, refreshExpiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		accessExpiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	if tokenResp.RefreshTokenExpiresIn > 0 {
		refreshExpiresAt = now.Add(time.Duration(tokenResp.RefreshTokenExpiresIn) * time.Second)
	}

	cred := Credential{
		AccessToken:           tokenResp.AccessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          tokenResp.RefreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}

	return PollResult{
		Credential: cred,
		User:       user,
	}, nil
}

func (c Client) Refresh(ctx context.Context, refreshToken string) (Credential, User, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Credential{}, User{}, ErrUnauthenticated
	}
	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		return Credential{}, User{}, fmt.Errorf("%w: GitHub client ID is not configured; set AIAI_GITHUB_CLIENT_ID", ErrIO)
	}
	reqBody := map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}
	var tokenResp gitHubTokenResponse
	resp, err := c.doJSON(ctx, http.MethodPost, c.tokenEndpoint(), "", reqBody, &tokenResp, http.StatusOK, http.StatusBadRequest, http.StatusUnauthorized)
	if err != nil {
		return Credential{}, User{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || tokenResp.Error == "bad_refresh_token" || tokenResp.Error == "unauthorized" {
		return Credential{}, User{}, ErrUnauthenticated
	}
	if tokenResp.Error != "" {
		return Credential{}, User{}, fmt.Errorf("%w: GitHub refresh error: %s", ErrIO, tokenResp.ErrorDescription)
	}
	if tokenResp.AccessToken == "" {
		return Credential{}, User{}, fmt.Errorf("%w: missing access token in refresh response", ErrIO)
	}
	user, err := c.Me(ctx, tokenResp.AccessToken)
	if err != nil {
		return Credential{}, User{}, err
	}
	now := time.Now().UTC()
	var accessExpiresAt, refreshExpiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		accessExpiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	if tokenResp.RefreshTokenExpiresIn > 0 {
		refreshExpiresAt = now.Add(time.Duration(tokenResp.RefreshTokenExpiresIn) * time.Second)
	}
	return Credential{
		AccessToken:           tokenResp.AccessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          tokenResp.RefreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, user, nil
}

func (c Client) Me(ctx context.Context, accessToken string) (User, error) {
	if strings.TrimSpace(accessToken) == "" {
		return User{}, ErrUnauthenticated
	}
	var userResp gitHubUserResponse
	resp, err := c.doJSON(ctx, http.MethodGet, c.userAPIEndpoint(), accessToken, nil, &userResp, http.StatusOK, http.StatusUnauthorized, http.StatusForbidden)
	if err != nil {
		return User{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return User{}, ErrUnauthenticated
	}
	if userResp.Login == "" || userResp.ID == 0 {
		return User{}, fmt.Errorf("%w: invalid GitHub user profile response", ErrIO)
	}
	idStr := strconv.FormatInt(userResp.ID, 10)
	return User{
		ID:        idStr,
		GitHubID:  idStr,
		Login:     userResp.Login,
		Name:      userResp.Name,
		AvatarURL: userResp.AvatarURL,
	}, nil
}

func (c Client) Logout(context.Context, string) error {
	// Native clients in GitHub Device Flow do not have a client_secret to call GitHub's
	// token revocation endpoint. Logout deletes local credentials in OS keyring/file store.
	return nil
}

func (c Client) deviceCodeEndpoint() string {
	if c.DeviceCodeURL != "" {
		return c.DeviceCodeURL
	}
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/") + "/login/device/code"
	}
	return DefaultDeviceCodeURL
}

func (c Client) tokenEndpoint() string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/") + "/login/oauth/access_token"
	}
	return DefaultTokenURL
}

func (c Client) userAPIEndpoint() string {
	if c.UserAPIURL != "" {
		return c.UserAPIURL
	}
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/") + "/user"
	}
	return DefaultUserAPIURL
}

func (c Client) doJSON(ctx context.Context, method, fullURL, token string, body, result any, expected ...int) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	} else {
		req.Header.Set("User-Agent", "aiai-cli")
	}
	if strings.Contains(fullURL, "api.github.com") || strings.HasSuffix(fullURL, "/user") {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = DefaultHTTPClient()
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: contact GitHub: %v", ErrIO, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read GitHub response: %v", ErrIO, err)
	}

	accepted := false
	for _, status := range expected {
		if resp.StatusCode == status {
			accepted = true
			break
		}
	}
	if !accepted {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("%w: GitHub returned %d: %s", ErrIO, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if result != nil && len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, result); err != nil {
			return nil, fmt.Errorf("%w: decode GitHub response: %v", ErrIO, err)
		}
	}
	return resp, nil
}

func ResolveGitHubClientID(injected string) string {
	return strings.TrimSpace(firstNonEmpty(
		os.Getenv("AIAI_GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_ID"),
		injected,
		DefaultGitHubClientID,
	))
}

func validVerificationURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}

func validatedBaseURL(value string) (*url.URL, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("API URL must be an absolute origin without credentials, query, or fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !loopback) {
		return nil, errors.New("API URL must use HTTPS (HTTP is allowed only for loopback development)")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}
