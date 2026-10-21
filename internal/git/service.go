package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Change struct {
	Path      string `json:"path"`
	Original  string `json:"original,omitempty"`
	Index     string `json:"index"`
	Worktree  string `json:"worktree"`
	Conflict  bool   `json:"conflict"`
	Untracked bool   `json:"untracked"`
}
type Commit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Subject string    `json:"subject"`
}
type Status struct {
	Root         string   `json:"root"`
	Name         string   `json:"name"`
	RepositoryID string   `json:"repository_id"`
	Branch       string   `json:"branch"`
	Head         string   `json:"head"`
	Upstream     string   `json:"upstream"`
	Ahead        int      `json:"ahead"`
	Behind       int      `json:"behind"`
	Detached     bool     `json:"detached"`
	Unborn       bool     `json:"unborn"`
	Changes      []Change `json:"changes"`
	Staged       int      `json:"staged"`
	Unstaged     int      `json:"unstaged"`
	Untracked    int      `json:"untracked"`
	Conflicts    int      `json:"conflicts"`
	Operation    string   `json:"operation,omitempty"`
	LastCommit   *Commit  `json:"last_commit,omitempty"`
}
type Branch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Head     string `json:"head"`
	Upstream string `json:"upstream"`
	Tracking string `json:"tracking"`
}
type Worktree struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch"`
	Detached bool   `json:"detached"`
	Locked   string `json:"locked,omitempty"`
	Prunable string `json:"prunable,omitempty"`
}
type Filter struct {
	Limit          int
	Since, Until   time.Time
	Author, Branch string
}
type Activity struct {
	Commits   []Commit       `json:"commits"`
	Days      map[string]int `json:"commits_by_day"`
	Weeks     map[string]int `json:"commits_by_week"`
	Truncated bool           `json:"truncated"`
}

// Service invokes explicit local Git operations without a shell. Read queries
// disable hooks and optional index writes; mutations retain repository hooks.
type (
	Service      struct{ Executable string }
	cappedBuffer struct {
		bytes.Buffer
		Limit int
	}
)

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.Limit-b.Len() {
		return 0, fmt.Errorf("git output exceeds %d bytes; narrow the query", b.Limit)
	}
	return b.Buffer.Write(p)
}

func (s *Service) run(ctx context.Context, path string, args ...string) ([]byte, error) {
	return s.execute(ctx, path, false, args...)
}

func (s *Service) execute(ctx context.Context, path string, mutation bool, args ...string) ([]byte, error) {
	timeout := 12 * time.Second
	if mutation {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	exe := s.Executable
	if exe == "" {
		exe = "git"
	}
	base := []string{"--no-pager", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "color.ui=false"}
	if !mutation {
		base = append(base, "-c", "core.hooksPath=")
	}
	cmd := exec.CommandContext(ctx, exe, append(base, args...)...)
	cmd.Dir = path
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = gitEnvironment()
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if mutation {
		cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=1", "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:")
	}
	out, stderr := &cappedBuffer{Limit: 4 << 20}, &cappedBuffer{Limit: 16 << 10}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// stderr can contain credential-bearing URLs. Expose operation/exit code,
		// not arbitrary Git diagnostics; safe.directory remains enforced by Git.
		if _, ok := err.(*exec.Error); ok {
			return nil, fmt.Errorf("git executable unavailable: install Git and put it in PATH")
		}
		if mutation {
			return nil, fmt.Errorf("git %s failed: %s (%v)", args[0], mutationHint(args[0], stderr.String()), err)
		}
		return nil, fmt.Errorf("git %s failed: repository unavailable, untrusted, or query exceeded limits (%v)", args[0], err)
	}
	return out.Bytes(), nil
}

func (s *Service) Status(ctx context.Context, path string) (Status, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Status{}, err
	}
	root, err := s.run(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return Status{}, err
	}
	st := Status{Root: strings.TrimSuffix(string(root), "\n"), Changes: []Change{}}
	st.Name = filepath.Base(st.Root)
	common, err := s.run(ctx, st.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return st, err
	}
	identity := strings.TrimSuffix(string(common), "\n")
	if canonical, err := filepath.EvalSymlinks(identity); err == nil {
		identity = canonical
	}
	st.RepositoryID = fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(identity))))
	payload, err := s.run(ctx, st.Root, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return st, err
	}
	if err := parseStatus(payload, &st); err != nil {
		return st, err
	}
	if gitDir, err := s.run(ctx, st.Root, "rev-parse", "--absolute-git-dir"); err == nil {
		for _, state := range []struct{ file, operation string }{
			{"rebase-merge", "rebase"},
			{"rebase-apply", "rebase"},
			{"CHERRY_PICK_HEAD", "cherry-pick"},
			{"REVERT_HEAD", "revert"},
			{"MERGE_HEAD", "merge"},
		} {
			if _, err := os.Stat(filepath.Join(strings.TrimSuffix(string(gitDir), "\n"), state.file)); err == nil {
				st.Operation = state.operation
				break
			}
		}
	}
	if !st.Unborn {
		commits, err := s.History(ctx, st.Root, Filter{Limit: 1})
		if err == nil && len(commits) > 0 {
			st.LastCommit = &commits[0]
		}
	}
	return st, nil
}

func parseStatus(payload []byte, st *Status) error {
	records := strings.Split(string(payload), "\x00")
	for i := 0; i < len(records); i++ {
		line := records[i]
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			key, value, _ := strings.Cut(line[2:], " ")
			switch key {
			case "branch.oid":
				st.Unborn = value == "(initial)"
				if !st.Unborn {
					st.Head = value
				}
			case "branch.head":
				st.Detached = value == "(detached)"
				if !st.Detached {
					st.Branch = value
				}
			case "branch.upstream":
				st.Upstream = value
			case "branch.ab":
				_, _ = fmt.Sscanf(value, "+%d -%d", &st.Ahead, &st.Behind)
			}
			continue
		}
		var c Change
		switch line[0] {
		case '?':
			c = Change{Path: line[2:], Index: "?", Worktree: "?", Untracked: true}
			st.Untracked++
		case '1', '2', 'u':
			fields := 9
			if line[0] == '2' {
				fields = 10
			}
			if line[0] == 'u' {
				fields = 11
			}
			parts := strings.SplitN(line, " ", fields)
			if len(parts) != fields || len(parts[1]) != 2 {
				return fmt.Errorf("invalid Git porcelain record")
			}
			c = Change{Path: parts[fields-1], Index: parts[1][:1], Worktree: parts[1][1:], Conflict: line[0] == 'u'}
			if line[0] == '2' {
				i++
				if i >= len(records) {
					return fmt.Errorf("missing rename source")
				}
				c.Original = records[i]
			}
			if c.Conflict {
				st.Conflicts++
			}
			if c.Index != "." {
				st.Staged++
			}
			if c.Worktree != "." {
				st.Unstaged++
			}
		default:
			continue
		}
		st.Changes = append(st.Changes, c)
	}
	return nil
}

func (s *Service) History(ctx context.Context, path string, f Filter) ([]Commit, error) {
	limit := f.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("history limit must be 1–1000")
	}
	args := []string{"log", "--no-show-signature", "--no-decorate", "--format=%H%x00%an%x00%cI%x00%s%x00", "-z", fmt.Sprintf("--max-count=%d", limit)}
	if !f.Since.IsZero() {
		args = append(args, "--since="+f.Since.Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		args = append(args, "--until="+f.Until.Format(time.RFC3339))
	}
	if f.Author != "" {
		args = append(args, "--author="+f.Author)
	}
	if f.Branch != "" {
		if strings.HasPrefix(f.Branch, "-") || strings.ContainsAny(f.Branch, "\x00\r\n") {
			return nil, fmt.Errorf("invalid branch filter")
		}
		args = append(args, f.Branch)
	}
	args = append(args, "--")
	payload, err := s.run(ctx, path, args...)
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(payload, []byte{0})
	commits := []Commit{}
	for i := 0; i < len(parts); {
		if len(parts[i]) == 0 {
			i++
			continue
		}
		if i+3 >= len(parts) {
			return nil, fmt.Errorf("invalid Git history record")
		}
		date, err := time.Parse(time.RFC3339, string(parts[i+2]))
		if err != nil {
			return nil, err
		}
		commits = append(commits, Commit{Hash: string(parts[i]), Author: string(parts[i+1]), Date: date.UTC(), Subject: string(parts[i+3])})
		i += 4
	}
	return commits, nil
}

func (s *Service) Branches(ctx context.Context, path string) ([]Branch, error) {
	payload, err := s.run(ctx, path, "for-each-ref", "--format=%(refname:short)%00%(HEAD)%00%(objectname)%00%(upstream:short)%00%(upstream:track)%00", "refs/heads/")
	if err != nil {
		return nil, err
	}
	result := []Branch{}
	for _, line := range bytes.Split(payload, []byte{'\n'}) {
		parts := bytes.Split(line, []byte{0})
		if len(parts) < 5 {
			continue
		}
		result = append(result, Branch{Name: string(parts[0]), Current: string(parts[1]) == "*", Head: string(parts[2]), Upstream: string(parts[3]), Tracking: string(parts[4])})
	}
	return result, nil
}

func (s *Service) Worktrees(ctx context.Context, path string) ([]Worktree, error) {
	payload, err := s.run(ctx, path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	result := []Worktree{}
	var item Worktree
	for _, part := range strings.Split(string(payload), "\x00") {
		key, value, _ := strings.Cut(part, " ")
		switch key {
		case "worktree":
			if item.Path != "" {
				result = append(result, item)
			}
			item = Worktree{Path: value}
		case "HEAD":
			item.Head = value
		case "branch":
			item.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			item.Detached = true
		case "locked":
			item.Locked = value
			if value == "" {
				item.Locked = "locked"
			}
		case "prunable":
			item.Prunable = value
			if value == "" {
				item.Prunable = "prunable"
			}
		}
	}
	if item.Path != "" {
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) Diff(ctx context.Context, path, file string, staged bool) (string, error) {
	clean := filepath.Clean(file)
	if filepath.IsAbs(file) || clean == ".." || strings.HasPrefix(filepath.ToSlash(clean), "../") || strings.ContainsRune(file, 0) {
		return "", fmt.Errorf("diff path must be repository-relative")
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=all"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	if file != "" {
		args = append(args, file)
	}
	payload, err := s.run(ctx, path, args...)
	return string(payload), err
}

func (s *Service) Activity(ctx context.Context, path string, f Filter) (Activity, error) {
	commits, err := s.History(ctx, path, f)
	a := Activity{Commits: commits, Days: map[string]int{}, Weeks: map[string]int{}, Truncated: len(commits) == f.Limit || len(commits) == 100 && f.Limit == 0}
	for _, c := range commits {
		local := c.Date.Local()
		a.Days[local.Format("2006-01-02")]++
		year, week := local.ISOWeek()
		a.Weeks[fmt.Sprintf("%d-W%02d", year, week)]++
	}
	return a, err
}

func (s *Service) Observe(ctx context.Context, path, start, branch string) (string, string, string) {
	st, err := s.Status(ctx, path)
	if err != nil {
		return "", "", "End Git snapshot unavailable"
	}
	if _, err := hex.DecodeString(start); err != nil || len(start) != 40 && len(start) != 64 {
		return st.Head, st.Branch, "Start HEAD unavailable; commit comparison uncertain"
	}
	if st.Branch != branch || start == "" || st.Head == "" {
		return st.Head, st.Branch, "Branch changed or HEAD unavailable; commit comparison uncertain"
	}
	if _, err := s.run(ctx, st.Root, "merge-base", "--is-ancestor", start, st.Head); err != nil {
		return st.Head, st.Branch, "History changed; commit comparison uncertain"
	}
	payload, err := s.run(ctx, st.Root, "rev-list", "--count", start+".."+st.Head)
	if err != nil {
		return st.Head, st.Branch, "Commit comparison unavailable"
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(payload)))
	return st.Head, st.Branch, fmt.Sprintf("%d new reachable commits observed; authorship is not inferred", count)
}
