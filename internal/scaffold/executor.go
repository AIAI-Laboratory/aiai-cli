package scaffold

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

type FileExecutor struct{}

func (FileExecutor) Apply(ctx context.Context, p Plan) (Result, error) {
	r := NewResult(p.TargetDir)
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if p.HasConflicts() {
		return r, fmt.Errorf("%w: review the plan; use --force only to replace listed files", ErrConflict)
	}
	target, err := platform.ResolveTarget(p.TargetDir)
	if err != nil || target != p.TargetDir {
		return r, fmt.Errorf("%w: target changed since preview", ErrConflict)
	}
	// Validate the entire operation set before creating the target directory.
	seen := map[string]bool{}
	for _, op := range p.Operations {
		if !platform.ValidRelative(op.Path) || seen[op.Path] || (op.Action != Create && op.Action != Overwrite && op.Action != Skip) {
			return r, fmt.Errorf("%w: invalid operation %q", ErrValidation, op.Path)
		}
		seen[op.Path] = true
	}
	root, err := platform.OpenTarget(target)
	if err != nil {
		return r, fmt.Errorf("%w: %v", ErrExecution, err)
	}
	defer root.Close()
	for _, op := range p.Operations {
		if err := checkExpected(root, op); err != nil {
			return r, fmt.Errorf("%w: %v", ErrConflict, err)
		}
	}
	for _, op := range p.Operations {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if op.Action == Skip {
			r.Skipped = append(r.Skipped, op.Path)
			continue
		}
		if err := checkExpected(root, op); err != nil {
			return r, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		if err := writeAtomic(root, op); err != nil {
			return r, fmt.Errorf("%w: %s: %v", ErrExecution, op.Path, err)
		}
		r.Completed = append(r.Completed, op.Path)
	}
	r.NextSteps = []string{"git init", "corepack enable", "pnpm install", "uv lock", "uv sync"}
	return r, nil
}

func checkExpected(root *os.Root, op Operation) error {
	if err := platform.CheckPath(root, op.Path); err != nil {
		return err
	}
	data, err := root.ReadFile(op.Path)
	if op.Action == Create {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("%s appeared after preview; generate a new plan", op.Path)
	}
	if err != nil {
		return err
	}
	if digest(data) != op.ExpectedHash {
		return fmt.Errorf("%s changed after preview; generate a new plan", op.Path)
	}
	return nil
}

func writeAtomic(root *os.Root, op Operation) error {
	if err := root.MkdirAll(path.Dir(op.Path), 0o755); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(op.Path), ".aiai-"+rand.Text()+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, op.Mode.Perm())
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()
	if _, err := f.Write(op.Content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(op.Mode.Perm()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := checkExpected(root, op); err != nil {
		return err
	}
	if op.Action == Create {
		// Atomic no-clobber publication: rename would replace a concurrently
		// created user file on Unix. Hard-link then unlink the temporary sibling.
		return root.Link(tmp, op.Path)
	}
	return root.Rename(tmp, op.Path)
}
