package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	DefaultRepo = "AIAI-Laboratory/aiai-cli"
)

var (
	ErrNoReleaseFound = errors.New("no release found")
	ErrAssetNotFound  = errors.New("matching release asset not found for current OS and architecture")
)

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Assets      []githubAsset `json:"assets"`
}

type GitHubClient struct {
	BaseURL    string
	Repo       string
	HTTPClient *http.Client
	Token      string
	UserAgent  string
}

func NewGitHubClient(repo, token, userAgent string) *GitHubClient {
	if repo == "" {
		repo = DefaultRepo
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
		if token == "" {
			token = os.Getenv("GH_TOKEN")
		}
	}
	if userAgent == "" {
		userAgent = "aiai-cli"
	}
	return &GitHubClient{
		BaseURL: "https://api.github.com",
		Repo:    repo,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		Token:     token,
		UserAgent: userAgent,
	}
}

func (c *GitHubClient) FetchLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.BaseURL, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoReleaseFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var ghRelease githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("decode github release: %w", err)
	}

	targetExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		targetExt = ".zip"
	}

	var matchedAsset, checksumAsset *githubAsset
	for i := range ghRelease.Assets {
		asset := &ghRelease.Assets[i]
		name := strings.ToLower(asset.Name)
		if name == "checksums.txt" {
			checksumAsset = asset
			continue
		}

		// Pattern: aiai_{version}_{goos}_{goarch}.(tar.gz|zip)
		osArchPattern := fmt.Sprintf("_%s_%s", runtime.GOOS, runtime.GOARCH)
		if strings.HasPrefix(name, "aiai_") && strings.Contains(name, osArchPattern) && strings.HasSuffix(name, targetExt) {
			matchedAsset = asset
		}
	}

	if matchedAsset == nil {
		return nil, fmt.Errorf("%w: %s/%s (%s)", ErrAssetNotFound, runtime.GOOS, runtime.GOARCH, targetExt)
	}

	version := strings.TrimPrefix(ghRelease.TagName, "v")
	rel := &Release{
		TagName:     ghRelease.TagName,
		Version:     version,
		Name:        ghRelease.Name,
		Body:        ghRelease.Body,
		PublishedAt: ghRelease.PublishedAt,
		AssetURL:    matchedAsset.BrowserDownloadURL,
		AssetName:   matchedAsset.Name,
		HTMLURL:     ghRelease.HTMLURL,
	}
	if checksumAsset != nil {
		rel.ChecksumURL = checksumAsset.BrowserDownloadURL
	}

	return rel, nil
}

// NormalizeVersion ensures the version string has a leading 'v' for semver comparison.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// IsNewerVersion returns true if latest is semantically newer than current.
func IsNewerVersion(current, latest string) bool {
	normCurrent := NormalizeVersion(current)
	normLatest := NormalizeVersion(latest)

	if !semver.IsValid(normLatest) {
		return false
	}
	if current == "dev" || !semver.IsValid(normCurrent) {
		return false
	}

	return semver.Compare(normLatest, normCurrent) > 0
}
