package scaffold

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

type Engine struct{ Registry templates.TemplateRegistry }

func (e Engine) Plan(ctx context.Context, templateID, target string, vars Variables, force bool) (Plan, error) {
	p := Plan{TemplateID: templateID, TargetDir: target, Operations: []Operation{}, Warnings: []string{}}
	t, err := e.Registry.Get(templateID)
	if err != nil {
		return p, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	p.TemplateVersion = t.Manifest.Version
	root, err := os.OpenRoot(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return p, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if root != nil {
		defer root.Close()
	}
	seen := map[string]bool{}
	for _, f := range t.Manifest.Files {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		dest, err := Render("destination", []byte(f.Destination), vars)
		if err != nil || !platform.ValidRelative(string(dest)) {
			return p, fmt.Errorf("%w: unsafe destination %q", ErrValidation, dest)
		}
		path := string(dest)
		key := strings.ToLower(path)
		if seen[key] {
			return p, fmt.Errorf("%w: duplicate destination %s", ErrValidation, path)
		}
		seen[key] = true
		data, err := fs.ReadFile(t.Files, f.Source)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		content, err := Render(f.Source, data, vars)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		op := Operation{Path: path, Action: Create, Mode: 0o644, Content: content}
		if f.Executable {
			op.Mode = 0o755
		}
		if root != nil {
			if err := platform.CheckPath(root, path); err != nil {
				return p, fmt.Errorf("%w: %v", ErrConflict, err)
			}
			existing, err := root.ReadFile(path)
			if err == nil {
				info, err := root.Stat(path)
				if err != nil {
					return p, fmt.Errorf("%w: %v", ErrConflict, err)
				}
				op.Mode = info.Mode().Perm()
				if f.Executable && runtime.GOOS != "windows" {
					op.Mode |= 0o111
				}
				op.ExpectedHash = digest(existing)
				switch {
				case string(existing) == string(content) && op.Mode == info.Mode().Perm():
					op.Action = Skip
				case force:
					op.Action = Overwrite
				default:
					op.Action = Conflict
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return p, fmt.Errorf("%w: %v", ErrConflict, err)
			}
		}
		p.Operations = append(p.Operations, op)
	}
	sort.Slice(p.Operations, func(i, j int) bool { return p.Operations[i].Path < p.Operations[j].Path })
	for i, op := range p.Operations {
		for _, other := range p.Operations[i+1:] {
			if strings.HasPrefix(strings.ToLower(other.Path), strings.ToLower(op.Path)+"/") {
				return p, fmt.Errorf("%w: destination is both a file and a directory", ErrValidation)
			}
		}
	}
	if force {
		p.Warnings = append(p.Warnings, "Only listed overwrite operations will replace existing files.")
	}
	return p, nil
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
