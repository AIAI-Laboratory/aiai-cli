// Package workspace joins explicit local registrations, live Git data and Zen
// history. Git history is queried, not copied into the registry.
package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	gitlocal "github.com/AIAI-Laboratory/aiai-cli/internal/git"
	"github.com/AIAI-Laboratory/aiai-cli/internal/localdata"
	"github.com/AIAI-Laboratory/aiai-cli/internal/zen"
)

type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Root         string    `json:"root_path"`
	RepositoryID string    `json:"repository_identity,omitempty"`
	Worktree     string    `json:"git_worktree_path,omitempty"`
	Description  string    `json:"description,omitempty"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastOpenedAt time.Time `json:"last_opened_at,omitempty"`
}
type Registry struct {
	SchemaVersion int       `json:"schema_version"`
	Projects      []Project `json:"projects"`
	Repositories  []Project `json:"repositories"`
}
type Overview struct {
	Project          Project          `json:"project"`
	Available        bool             `json:"available"`
	Git              *gitlocal.Status `json:"git,omitempty"`
	Warning          string           `json:"warning,omitempty"`
	Focus            zen.Statistics   `json:"focus"`
	Recent           []Event          `json:"recent_activity"`
	TodayCommits     int              `json:"today_commits_shown"`
	HistoryTruncated bool             `json:"today_git_history_truncated"`
}
type Event struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	ProjectID string    `json:"project_id"`
	Time      time.Time `json:"time"`
	Branch    string    `json:"branch,omitempty"`
	Text      string    `json:"text"`
}
type Timeline struct {
	Events    []Event        `json:"events"`
	Focus     zen.Statistics `json:"focus"`
	Commits   int            `json:"commits_shown"`
	Truncated bool           `json:"git_history_truncated"`
	Warning   string         `json:"warning,omitempty"`
}
type Service struct {
	Store localdata.File[Registry]
	Git   *gitlocal.Service
	Zen   zen.Repository
}

func New(path string, g *gitlocal.Service, z zen.Repository) (*Service, error) {
	if path == "" {
		var err error
		path, err = localdata.Path("workspaces.json")
		if err != nil {
			return nil, err
		}
	}
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if g == nil {
		g = &gitlocal.Service{}
	}
	return &Service{Store: localdata.File[Registry]{Path: path, Empty: func() Registry { return Registry{SchemaVersion: 1, Projects: []Project{}, Repositories: []Project{}} }, Validate: validate}, Git: g, Zen: z}, nil
}

func validate(r Registry) error {
	if r.SchemaVersion != 1 {
		return fmt.Errorf("unsupported workspace schema %d", r.SchemaVersion)
	}
	for _, items := range [][]Project{r.Projects, r.Repositories} {
		ids, names, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, p := range items {
			if p.ID == "" || strings.TrimSpace(p.Name) == "" || !filepath.IsAbs(p.Root) || ids[p.ID] || names[strings.ToLower(p.Name)] || paths[pathKey(p.Root)] {
				return errors.New("invalid or duplicate workspace entry")
			}
			ids[p.ID] = true
			names[strings.ToLower(p.Name)] = true
			paths[pathKey(p.Root)] = true
		}
	}
	return nil
}

func pathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func (s *Service) List(ctx context.Context, repositories bool) ([]Project, error) {
	r, err := s.Store.Read(ctx)
	if repositories {
		return r.Repositories, err
	}
	return r.Projects, err
}

func (s *Service) Find(ctx context.Context, name string, repositories bool) (Project, error) {
	items, err := s.List(ctx, repositories)
	if err != nil {
		return Project{}, err
	}
	for _, p := range items {
		if strings.EqualFold(name, p.Name) || name == p.ID {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("project %q is not registered", name)
}

func (s *Service) Add(ctx context.Context, path, name, description string, tags []string, repositories bool) (Project, error) {
	root, err := Canonical(path)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Project{}, err
	}
	if !info.IsDir() {
		return Project{}, errors.New("project path must be a directory")
	}
	p := Project{ID: rand.Text(), Name: strings.TrimSpace(name), Root: root, Description: description, Tags: tags, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	st, gitErr := s.Git.Status(ctx, root)
	if gitErr == nil {
		p.RepositoryID, p.Worktree = st.RepositoryID, st.Root
		if repositories {
			p.Root = st.Root
		}
	} else if repositories {
		return Project{}, gitErr
	}
	if p.Name == "" {
		p.Name = filepath.Base(p.Root)
	}
	if strings.IndexFunc(p.Name, unicode.IsControl) >= 0 {
		return Project{}, errors.New("project name must be one line")
	}
	err = s.Store.Update(ctx, func(r *Registry) error {
		items := &r.Projects
		if repositories {
			items = &r.Repositories
		}
		for _, old := range *items {
			oldInfo, _ := os.Stat(old.Root)
			if pathKey(old.Root) == pathKey(p.Root) || oldInfo != nil && os.SameFile(oldInfo, info) {
				return fmt.Errorf("directory already registered as %q", old.Name)
			}
			if strings.EqualFold(old.Name, p.Name) {
				return fmt.Errorf("name %q already exists; choose --name", p.Name)
			}
		}
		*items = append(*items, p)
		return nil
	})
	return p, err
}

func (s *Service) Remove(ctx context.Context, name string, repositories bool) error {
	return s.Store.Update(ctx, func(r *Registry) error {
		items := &r.Projects
		if repositories {
			items = &r.Repositories
		}
		for i, p := range *items {
			if strings.EqualFold(p.Name, name) || p.ID == name {
				*items = append((*items)[:i], (*items)[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("project %q is not registered", name)
	})
}

func (s *Service) Open(ctx context.Context, id string) error {
	return s.Store.Update(ctx, func(r *Registry) error {
		for i := range r.Projects {
			if r.Projects[i].ID == id {
				r.Projects[i].LastOpenedAt = time.Now().UTC()
				return nil
			}
		}
		return nil
	})
}

func (s *Service) Overview(ctx context.Context, p Project) Overview {
	o := Overview{Project: p, Recent: []Event{}}
	info, err := os.Stat(p.Root)
	o.Available = err == nil && info.IsDir()
	if !o.Available {
		o.Warning = "Directory unavailable; registration retained"
	}
	if o.Available {
		st, err := s.Git.Status(ctx, p.Root)
		if err == nil {
			o.Git = &st
			if !st.Unborn {
				now := time.Now()
				since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				activity, e := s.Git.Activity(ctx, p.Root, gitlocal.Filter{Since: since, Until: now, Limit: 100})
				if e != nil {
					o.Warning = e.Error()
				} else {
					o.TodayCommits = len(activity.Commits)
					o.HistoryTruncated = activity.Truncated
					for _, c := range activity.Commits {
						o.Recent = append(o.Recent, Event{ID: "git:" + st.RepositoryID + ":" + c.Hash, Source: "git", ProjectID: p.ID, Time: c.Date, Branch: st.Branch, Text: c.Subject + " — " + c.Author})
					}
				}
				if len(o.Recent) == 0 && st.LastCommit != nil {
					c := st.LastCommit
					o.Recent = append(o.Recent, Event{ID: "git:" + st.RepositoryID + ":" + c.Hash, Source: "git", ProjectID: p.ID, Time: c.Date, Text: c.Subject + " — " + c.Author})
				}
			}
		} else if p.RepositoryID != "" {
			o.Warning = err.Error()
		}
	}
	if s.Zen != nil {
		_, sessions, err := s.Zen.Load(ctx)
		if err != nil {
			o.Warning = err.Error()
		} else {
			associated := ProjectSessions(sessions, p)
			o.Focus = zen.CalculateStatistics(associated, time.Now())
			for _, item := range associated {
				if !item.FinishedAt.IsZero() {
					o.Recent = append(o.Recent, Event{ID: "zen:" + item.ID + ":end", Source: "zen", ProjectID: p.ID, Time: item.FinishedAt, Branch: item.BranchAtEnd, Text: fmt.Sprintf("%s %s · %s active", item.Type, item.Status, (time.Duration(item.ActiveSeconds) * time.Second).String())})
				}
			}
		}
	}
	sort.SliceStable(o.Recent, func(i, j int) bool { return o.Recent[i].Time.After(o.Recent[j].Time) })
	if len(o.Recent) > 8 {
		o.Recent = o.Recent[:8]
	}
	return o
}

// Board uses a fixed worker pool, so one unavailable project does not hold up
// all the others and large registries cannot spawn unbounded subprocesses.
func (s *Service) Board(ctx context.Context, repositories bool) ([]Overview, error) {
	projects, err := s.List(ctx, repositories)
	if err != nil {
		return nil, err
	}
	result := make([]Overview, len(projects))
	jobs := make(chan int)
	var group sync.WaitGroup
	for range min(4, len(projects)) {
		group.Go(func() {
			for i := range jobs {
				query, done := context.WithTimeout(ctx, 4*time.Second)
				result[i] = s.Overview(query, projects[i])
				done()
			}
		})
	}
	for i := range projects {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	group.Wait()
	return result, ctx.Err()
}

func ProjectSessions(sessions []zen.Session, p Project) []zen.Session {
	result := []zen.Session{}
	for _, s := range sessions {
		if s.ProjectID == p.ID || s.ProjectID == "" && s.WorktreePath != "" && pathKey(s.WorktreePath) == pathKey(p.Root) {
			result = append(result, s)
		}
	}
	return result
}

func (s *Service) Timeline(ctx context.Context, p Project, f gitlocal.Filter) (Timeline, error) {
	result := Timeline{Events: []Event{}}
	seen := map[string]bool{}
	add := func(e Event) {
		if seen[e.ID] || !f.Since.IsZero() && e.Time.Before(f.Since) || !f.Until.IsZero() && e.Time.After(f.Until) {
			return
		}
		seen[e.ID] = true
		result.Events = append(result.Events, e)
	}
	status, err := s.Git.Status(ctx, p.Root)
	if err == nil && !status.Unborn {
		activity, e := s.Git.Activity(ctx, p.Root, f)
		if e != nil {
			result.Warning = e.Error()
		} else {
			result.Truncated = activity.Truncated
			result.Commits = len(activity.Commits)
			branch := f.Branch
			if branch == "" {
				branch = status.Branch
			}
			for _, c := range activity.Commits {
				add(Event{ID: "git:" + status.RepositoryID + ":" + c.Hash, Source: "git", ProjectID: p.ID, Time: c.Date, Branch: branch, Text: c.Subject + " — " + c.Author})
			}
		}
	} else if p.RepositoryID != "" {
		result.Warning = "Git unavailable; showing stored focus history"
	}
	if s.Zen != nil {
		_, sessions, err := s.Zen.Load(ctx)
		if err != nil {
			return result, err
		}
		filtered := []zen.Session{}
		for _, session := range ProjectSessions(sessions, p) {
			if f.Branch != "" && session.BranchAtStart != f.Branch && session.BranchAtEnd != f.Branch {
				continue
			}
			if !f.Since.IsZero() && session.StartedAt.Before(f.Since) && (session.FinishedAt.IsZero() || session.FinishedAt.Before(f.Since)) || !f.Until.IsZero() && session.StartedAt.After(f.Until) {
				continue
			}
			filtered = append(filtered, session)
			add(Event{ID: "zen:" + session.ID + ":start", Source: "zen", ProjectID: p.ID, Time: session.StartedAt, Branch: session.BranchAtStart, Text: string(session.Type) + " started"})
			if !session.FinishedAt.IsZero() {
				add(Event{ID: "zen:" + session.ID + ":end", Source: "zen", ProjectID: p.ID, Time: session.FinishedAt, Branch: session.BranchAtEnd, Text: fmt.Sprintf("%s %s (%s active)", session.Type, session.Status, (time.Duration(session.ActiveSeconds) * time.Second).String())})
			}
		}
		result.Focus = zen.CalculateStatistics(filtered, time.Now())
	}
	sort.SliceStable(result.Events, func(i, j int) bool {
		if result.Events[i].Time.Equal(result.Events[j].Time) {
			return result.Events[i].ID < result.Events[j].ID
		}
		return result.Events[i].Time.Before(result.Events[j].Time)
	})
	return result, nil
}
