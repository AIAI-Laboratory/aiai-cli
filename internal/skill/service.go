package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DefaultService struct {
	ArchiveURL string
	HTTPClient *http.Client
	UserAgent  string
	WorkDir    string
	HomeDir    string
}

func NewService(userAgent string) *DefaultService {
	if userAgent == "" {
		userAgent = "aiai-cli"
	}
	return &DefaultService{
		ArchiveURL: DefaultArchiveURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		UserAgent:  userAgent,
	}
}

func (s *DefaultService) Available(ctx context.Context) (*Result, error) {
	skills, err := s.fetch(ctx, archiveOptions{metadataOnly: true})
	if err != nil {
		return nil, err
	}
	return &Result{Action: "available", Skills: descriptors(skills)}, nil
}

func (s *DefaultService) List(scope Scope) (*Result, error) {
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	tracked, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	installed, err := installedDescriptors(root, tracked)
	if err != nil {
		return nil, err
	}
	return &Result{Action: "list", Scope: scope, Destination: root, Skills: installed}, nil
}

func (s *DefaultService) Install(ctx context.Context, names []string, scope Scope, all, overwrite bool) (*Result, error) {
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	tracked, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	if err := validateSelectionRequest(names, all); err != nil {
		return nil, err
	}
	if !all {
		if err := preflightInstall(root, names, overwrite); err != nil {
			return nil, err
		}
	}
	options := archiveOptions{}
	if !all {
		options.names = make(map[string]struct{}, len(names))
		for _, name := range names {
			options.names[name] = struct{}{}
		}
	}
	available, err := s.fetch(ctx, options)
	if err != nil {
		return nil, err
	}
	selected, err := selectSkills(available, names, all)
	if err != nil {
		return nil, err
	}
	if err := installSkills(root, selected, overwrite); err != nil {
		return nil, err
	}
	for _, item := range selected {
		tracked.Skills[item.descriptor.Name] = manifestSkill{Description: item.descriptor.Description}
	}
	if err := writeManifest(root, tracked); err != nil {
		return nil, err
	}
	result := descriptors(selected)
	for i := range result {
		result[i].Path = filepath.Join(root, result[i].Name)
	}
	return &Result{Action: "install", Scope: scope, Destination: root, Skills: result}, nil
}

func (s *DefaultService) Remove(names []string, scope Scope, all bool) (*Result, error) {
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	tracked, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	selected, err := selectDescriptors(manifestDescriptors(root, tracked), names, all)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return &Result{Action: "remove", Scope: scope, Destination: root, Skills: []Descriptor{}}, nil
	}
	for _, item := range selected {
		destination := filepath.Join(root, item.Name)
		info, err := os.Lstat(destination)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: inspect %s: %v", ErrIO, destination, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: refusing to remove non-directory skill %s", ErrValidation, destination)
		}
		if err := os.RemoveAll(destination); err != nil {
			return nil, fmt.Errorf("%w: remove %s: %v", ErrIO, destination, err)
		}
	}
	for _, item := range selected {
		delete(tracked.Skills, item.Name)
	}
	if err := writeManifest(root, tracked); err != nil {
		return nil, err
	}
	return &Result{Action: "remove", Scope: scope, Destination: root, Skills: selected}, nil
}

func (s *DefaultService) Update(ctx context.Context, names []string, scope Scope, all bool) (*Result, error) {
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	tracked, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	selected, err := selectDescriptors(manifestDescriptors(root, tracked), names, all || len(names) == 0)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return &Result{Action: "update", Scope: scope, Destination: root, Skills: []Descriptor{}}, nil
	}
	selectedNames := make([]string, 0, len(selected))
	for _, item := range selected {
		selectedNames = append(selectedNames, item.Name)
	}
	result, err := s.Install(ctx, selectedNames, scope, false, true)
	if err != nil {
		return nil, err
	}
	result.Action = "update"
	return result, nil
}

func (s *DefaultService) fetch(ctx context.Context, options archiveOptions) ([]archiveSkill, error) {
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	archiveURL := s.ArchiveURL
	if archiveURL == "" {
		archiveURL = DefaultArchiveURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create skill download request: %v", ErrIO, err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", s.UserAgent)
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: download skills from %s: %v", ErrIO, DefaultRepo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%w: download skills from %s returned status %d: %s", ErrIO, DefaultRepo, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if resp.ContentLength > maxArchiveSize {
		return nil, fmt.Errorf("%w: skill archive exceeds %d bytes", ErrValidation, maxArchiveSize)
	}
	limited := &io.LimitedReader{R: resp.Body, N: maxArchiveSize + 1}
	skills, err := parseArchive(limited, options)
	if limited.N == 0 {
		return nil, fmt.Errorf("%w: skill archive exceeds %d bytes", ErrValidation, maxArchiveSize)
	}
	return skills, err
}

func (s *DefaultService) root(scope Scope) (string, error) {
	var base string
	switch scope {
	case ScopeProject:
		base = s.WorkDir
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("%w: resolve working directory: %v", ErrIO, err)
			}
		}
	case ScopeGlobal:
		base = s.HomeDir
		if base == "" {
			var err error
			base, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("%w: resolve home directory: %v", ErrIO, err)
			}
		}
	default:
		return "", fmt.Errorf("%w: unknown skill scope %q", ErrValidation, scope)
	}
	abs, err := filepath.Abs(filepath.Join(base, ".agents", "skills"))
	if err != nil {
		return "", fmt.Errorf("%w: resolve skill directory: %v", ErrIO, err)
	}
	return abs, nil
}

func githubToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GH_TOKEN")
}
