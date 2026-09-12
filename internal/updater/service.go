package updater

import (
	"context"
	"fmt"
	"os"
	"time"
)

const DefaultCheckInterval = 24 * time.Hour

type Service interface {
	CheckForUpdate(ctx context.Context, currentVersion string, force bool) (*CheckResult, error)
	ApplyUpdate(ctx context.Context, release Release) error
	Postpone(release Release, skip bool) error
	Restart() error
}

type DefaultService struct {
	GitHub        *GitHubClient
	Store         StateStore
	CheckInterval time.Duration
}

func NewService(repo, token, userAgent, statePath string) (*DefaultService, error) {
	store, err := NewFileStateStore(statePath)
	if err != nil {
		return nil, err
	}
	client := NewGitHubClient(repo, token, userAgent)
	return &DefaultService{
		GitHub:        client,
		Store:         store,
		CheckInterval: DefaultCheckInterval,
	}, nil
}

func (s *DefaultService) CheckForUpdate(ctx context.Context, currentVersion string, force bool) (*CheckResult, error) {
	now := time.Now()
	res := &CheckResult{
		CurrentVersion: currentVersion,
		CheckedAt:      now,
	}

	if !force {
		// In dev mode, skip automatic check unless explicitly forced via env
		if currentVersion == "dev" && os.Getenv("AIAI_FORCE_UPDATE_CHECK") != "1" {
			return res, nil
		}

		if s.Store != nil {
			state, err := s.Store.Load()
			if err == nil {
				// If reminded later and the time hasn't passed, skip checking
				if !state.RemindAfter.IsZero() && now.Before(state.RemindAfter) {
					return res, nil
				}
			}
		}
	}

	release, err := s.GitHub.FetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	res.LatestRelease = release

	// Persist last checked
	if s.Store != nil {
		state, _ := s.Store.Load()
		state.LastCheckedAt = now
		state.LatestVersion = release.Version
		_ = s.Store.Save(state)

		if !force && state.SkippedVersion != "" && state.SkippedVersion == release.Version {
			res.HasUpdate = false
			return res, nil
		}
	}

	if force {
		res.HasUpdate = true
	} else {
		res.HasUpdate = IsNewerVersion(currentVersion, release.TagName)
	}

	return res, nil
}

func (s *DefaultService) ApplyUpdate(ctx context.Context, release Release) error {
	tmpBinary, err := DownloadAndExtract(ctx, s.GitHub.HTTPClient, release, s.GitHub.Token)
	if err != nil {
		return fmt.Errorf("download and extract: %w", err)
	}
	defer os.Remove(tmpBinary)

	if err := ReplaceExecutable(tmpBinary); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}

	if s.Store != nil {
		state, _ := s.Store.Load()
		state.SkippedVersion = ""
		state.RemindAfter = time.Time{}
		_ = s.Store.Save(state)
	}

	return nil
}

func (s *DefaultService) Postpone(release Release, skip bool) error {
	if s.Store == nil {
		return nil
	}
	state, _ := s.Store.Load()
	if skip {
		state.SkippedVersion = release.Version
		state.RemindAfter = time.Time{}
	} else {
		state.RemindAfter = time.Now().Add(s.CheckInterval)
	}
	return s.Store.Save(state)
}

func (s *DefaultService) Restart() error {
	return restartProcess()
}
