package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Stage updates only the selected paths, including both sides of a rename.
func (s *Service) Stage(ctx context.Context, path string, change Change) error {
	return s.updateIndex(ctx, path, change, true, false)
}

func (s *Service) Unstage(ctx context.Context, path string, change Change) error {
	return s.updateIndex(ctx, path, change, false, false)
}

func (s *Service) StageAll(ctx context.Context, path string) error {
	return s.updateIndex(ctx, path, Change{}, true, true)
}

func (s *Service) UnstageAll(ctx context.Context, path string) error {
	return s.updateIndex(ctx, path, Change{}, false, true)
}

func (s *Service) updateIndex(ctx context.Context, path string, change Change, stage, all bool) error {
	paths := []string{"."}
	if !all {
		paths = []string{change.Path}
		if change.Original != "" {
			paths = append(paths, change.Original)
		}
		for _, file := range paths {
			p := filepath.Clean(file)
			if file == "" || p == "." || p == ".." || filepath.IsAbs(file) || strings.HasPrefix(filepath.ToSlash(p), "../") || strings.ContainsRune(file, 0) {
				return fmt.Errorf("file must be a repository-relative path")
			}
		}
	}
	st, err := s.Status(ctx, path)
	if err != nil {
		return err
	}
	args := []string{"add", "--all", "--"}
	if !stage {
		args = []string{"reset", "--quiet", "HEAD", "--"}
		if st.Unborn {
			// Only remove index entries; retain every working-tree file, even
			// when its contents changed after it was staged.
			args = []string{"rm", "--cached", "--force", "-r", "--ignore-unmatch", "--"}
		}
	}
	_, err = s.execute(ctx, st.Root, true, append(args, paths...)...)
	return err
}

// CommitStaged commits the index as prepared by the user, preserving hooks and
// signing configuration. It never implicitly stages working-tree changes.
func (s *Service) CommitStaged(ctx context.Context, path, message string) error {
	message = strings.TrimSpace(message)
	if message == "" || strings.ContainsRune(message, 0) || len(message) > 4096 {
		return fmt.Errorf("enter a commit message (maximum 4096 bytes)")
	}
	st, err := s.Status(ctx, path)
	if err != nil {
		return err
	}
	if st.Conflicts > 0 {
		return fmt.Errorf("resolve and stage conflicted files before committing")
	}
	if st.Staged == 0 {
		return fmt.Errorf("no staged changes; stage files in Changes first")
	}
	_, err = s.execute(ctx, st.Root, true, "commit", "--message", message, "--cleanup=strip")
	return err
}

func (s *Service) validateBranch(ctx context.Context, path, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("enter a valid local branch name")
	}
	if _, err := s.run(ctx, path, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("invalid branch name")
	}
	return nil
}

// SwitchBranch uses normal Git safeguards: no force, automatic stash, or remote
// branch guessing. Branches checked out in other worktrees remain protected.
func (s *Service) SwitchBranch(ctx context.Context, path, name string) error {
	st, err := s.Status(ctx, path)
	if err != nil {
		return err
	}
	if err := s.validateBranch(ctx, st.Root, name); err != nil {
		return err
	}
	branches, err := s.Branches(ctx, st.Root)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch.Name == name {
			_, err = s.execute(ctx, st.Root, true, "switch", "--no-guess", "--", name)
			return err
		}
	}
	return fmt.Errorf("local branch does not exist; refresh the branch list")
}

func (s *Service) CreateBranch(ctx context.Context, path, name string) error {
	st, err := s.Status(ctx, path)
	if err != nil {
		return err
	}
	if st.Unborn {
		return fmt.Errorf("create the first commit before creating another branch")
	}
	if err := s.validateBranch(ctx, st.Root, name); err != nil {
		return err
	}
	_, err = s.execute(ctx, st.Root, true, "switch", "--no-guess", "--create", name)
	return err
}

// Keep diagnostics useful without exposing arbitrary hook output or URLs.
func mutationHint(operation, diagnostic string) string {
	diagnostic = strings.ToLower(diagnostic)
	switch {
	case strings.Contains(diagnostic, "index.lock"):
		return "another Git operation is using the index; wait and refresh"
	case strings.Contains(diagnostic, "identity unknown"), strings.Contains(diagnostic, "unable to auto-detect email"):
		return "configure Git user.name and user.email, then retry"
	case strings.Contains(diagnostic, "gpg failed"), strings.Contains(diagnostic, "failed to sign"), strings.Contains(diagnostic, "signing failed"):
		return "commit signing failed; check your Git signing configuration"
	case strings.Contains(diagnostic, "already checked out"), strings.Contains(diagnostic, "already used by worktree"):
		return "this branch is already checked out in another worktree"
	case strings.Contains(diagnostic, "would be overwritten"), strings.Contains(diagnostic, "local changes"):
		return "local changes prevent switching; commit or stash them first"
	case strings.Contains(diagnostic, "already exists"):
		return "a branch with this name already exists"
	case operation == "commit":
		return "check staged files, repository hooks and signing; refresh to inspect the result"
	case operation == "switch":
		return "check the branch, local changes and repository state; refresh before retrying"
	default:
		return "could not update the index; check file permissions and refresh"
	}
}
