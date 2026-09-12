package updater

import "time"

// Release represents a published release from GitHub.
type Release struct {
	TagName     string    `json:"tag_name"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	AssetURL    string    `json:"asset_url"`
	AssetName   string    `json:"asset_name"`
	ChecksumURL string    `json:"checksum_url"`
	HTMLURL     string    `json:"html_url"`
}

// State represents the cached updater state persisted in user config directory.
type State struct {
	LastCheckedAt  time.Time `json:"last_checked_at"`
	LatestVersion  string    `json:"latest_version"`
	SkippedVersion string    `json:"skipped_version,omitempty"`
	RemindAfter    time.Time `json:"remind_after,omitempty"`
}

// CheckResult contains the result of checking for available updates.
type CheckResult struct {
	HasUpdate      bool      `json:"has_update"`
	CurrentVersion string    `json:"current_version"`
	LatestRelease  *Release  `json:"latest_release,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}
