package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Service struct {
	API         API
	Store       Store
	BaseURL     string
	Now         func() time.Time
	Wait        func(context.Context, time.Duration) error
	OpenBrowser func(string) error
}

func NewService(api API, store Store, baseURL string) *Service {
	return &Service{API: api, Store: store, BaseURL: baseURL, Now: time.Now, Wait: waitContext, OpenBrowser: OpenBrowser}
}

func (s *Service) StartLogin(ctx context.Context, options LoginOptions) (LoginStart, error) {
	if !options.Force {
		if current, err := s.WhoAmI(ctx); err == nil {
			return LoginStart{Existing: &current}, nil
		} else if !errors.Is(err, ErrUnauthenticated) {
			return LoginStart{}, err
		}
	}
	device, err := s.API.StartDevice(ctx)
	if err != nil {
		return LoginStart{}, err
	}
	if !options.NoBrowser && s.OpenBrowser != nil {
		if err := s.OpenBrowser(device.VerificationURI); err != nil {
			device.BrowserOpenWarning = "Could not open a browser; use the verification URL manually."
		}
	}
	return LoginStart{Device: device}, nil
}

func (s *Service) PollLogin(ctx context.Context, attempt DeviceAuthorization) (PollResult, error) {
	if !attempt.ExpiresAt.After(s.now()) {
		return PollResult{}, ErrExpired
	}
	result, err := s.API.PollDevice(ctx, attempt.AuthID)
	if err != nil {
		return PollResult{}, err
	}
	if result.Pending {
		return result, nil
	}
	stored := StoredCredential{SchemaVersion: 1, Origin: s.origin(), Credential: result.Credential, User: result.User}
	_, err = s.Store.Save(s.origin(), stored)
	if err != nil {
		return PollResult{}, err
	}
	result.RetryAfterSeconds = 0
	// Storage is deliberately not part of the backend poll response. The caller can
	// retrieve it through Cached after this atomic save.
	return result, nil
}

func (s *Service) Login(ctx context.Context, options LoginOptions, ready func(DeviceAuthorization)) (Result, error) {
	start, err := s.StartLogin(ctx, options)
	if err != nil {
		return Result{}, err
	}
	if start.Existing != nil {
		return *start.Existing, nil
	}
	if ready != nil {
		ready(start.Device)
	}
	attempt := start.Device
	delay := time.Duration(attempt.IntervalSeconds) * time.Second
	for {
		if err := s.wait(ctx, delay); err != nil {
			return Result{}, err
		}
		poll, err := s.PollLogin(ctx, attempt)
		if err != nil {
			return Result{}, err
		}
		if !poll.Pending {
			result, err := s.Cached()
			if err != nil {
				return Result{}, err
			}
			result.Warning = joinWarnings(result.Warning, attempt.BrowserOpenWarning)
			return result, nil
		}
		seconds := poll.RetryAfterSeconds
		if seconds < 1 {
			seconds = attempt.IntervalSeconds
		}
		delay = time.Duration(seconds) * time.Second
	}
}

func (s *Service) Cached() (Result, error) {
	stored, storage, err := s.Store.Load(s.origin())
	if errors.Is(err, ErrNotFound) {
		return Result{Authenticated: false}, nil
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Authenticated: true, User: stored.User, Storage: storage, Warning: storageWarning(storage)}, nil
}

func (s *Service) RequireSession(ctx context.Context) (Credential, User, error) {
	origin := s.origin()
	stored, _, err := s.Store.Load(origin)
	if errors.Is(err, ErrNotFound) {
		return Credential{}, User{}, ErrUnauthenticated
	}
	if err != nil {
		return Credential{}, User{}, err
	}
	if stored.Credential.RefreshTokenExpiresAt.After(time.Time{}) && !stored.Credential.RefreshTokenExpiresAt.After(s.now()) {
		_ = s.Store.Delete(origin)
		return Credential{}, User{}, ErrUnauthenticated
	}
	if stored.Credential.AccessToken != "" && (stored.Credential.AccessTokenExpiresAt.IsZero() || stored.Credential.AccessTokenExpiresAt.After(s.now().Add(30*time.Second))) {
		return stored.Credential, stored.User, nil
	}
	if stored.Credential.RefreshToken != "" {
		credential, user, err := s.API.Refresh(ctx, stored.Credential.RefreshToken)
		if errors.Is(err, ErrUnauthenticated) {
			_ = s.Store.Delete(origin)
			return Credential{}, User{}, ErrUnauthenticated
		}
		if err != nil {
			return Credential{}, User{}, err
		}
		if user.ID == "" {
			user = stored.User
		}
		stored.Credential, stored.User = credential, user
		if _, err := s.Store.Save(origin, stored); err != nil {
			return Credential{}, User{}, err
		}
		return credential, user, nil
	}
	_ = s.Store.Delete(origin)
	return Credential{}, User{}, ErrUnauthenticated
}

func (s *Service) WhoAmI(ctx context.Context) (Result, error) {
	credential, _, err := s.RequireSession(ctx)
	if err != nil {
		return Result{}, err
	}
	user, err := s.API.Me(ctx, credential.AccessToken)
	if errors.Is(err, ErrUnauthenticated) {
		_ = s.Store.Delete(s.origin())
		return Result{}, ErrUnauthenticated
	}
	if err != nil {
		return Result{}, err
	}
	stored, _, err := s.Store.Load(s.origin())
	if err != nil {
		return Result{}, err
	}
	stored.User = user
	storage, err := s.Store.Save(s.origin(), stored)
	if err != nil {
		return Result{}, err
	}
	return Result{Authenticated: true, User: user, Storage: storage, Warning: storageWarning(storage)}, nil
}

func (s *Service) Logout(ctx context.Context) (LogoutResult, error) {
	origin := s.origin()
	stored, _, loadErr := s.Store.Load(origin)
	if errors.Is(loadErr, ErrNotFound) {
		return LogoutResult{}, nil
	}
	var remoteErr error
	if loadErr == nil {
		remoteErr = s.API.Logout(ctx, stored.Credential.RefreshToken)
	}
	localErr := s.Store.Delete(origin)
	if localErr != nil && !errors.Is(localErr, ErrNotFound) {
		return LogoutResult{}, localErr
	}
	if loadErr != nil {
		return LogoutResult{}, loadErr
	}
	if remoteErr != nil {
		warning := "Local credentials were deleted, but the remote session may remain active until it expires."
		if errors.Is(remoteErr, context.Canceled) || errors.Is(remoteErr, context.DeadlineExceeded) {
			return LogoutResult{Warning: warning}, remoteErr
		}
		return LogoutResult{Warning: warning}, fmt.Errorf("%w: %v", ErrIO, remoteErr)
	}
	return LogoutResult{}, nil
}

func (s *Service) origin() string {
	if strings.TrimSpace(s.BaseURL) != "" {
		parsed, err := validatedBaseURL(s.BaseURL)
		if err == nil && parsed != nil {
			return parsed.Scheme + "://" + parsed.Host + parsed.Path
		}
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultGitHubOrigin
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) wait(ctx context.Context, delay time.Duration) error {
	if s.Wait != nil {
		return s.Wait(ctx, delay)
	}
	return waitContext(ctx, delay)
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func storageWarning(storage string) string {
	if storage == "file" {
		return "OS keychain is unavailable; credentials are stored in a private 0600 file."
	}
	return ""
}

func joinWarnings(values ...string) string {
	var warnings []string
	for _, value := range values {
		if value != "" {
			warnings = append(warnings, value)
		}
	}
	return strings.Join(warnings, " ")
}

func ResolveAPIURL(injected string) string {
	return strings.TrimSpace(firstNonEmpty(os.Getenv("AIAI_API_URL"), injected))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
