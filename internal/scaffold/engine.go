package scaffold

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
	templates "github.com/AIAI-Laboratory/aiai-cli/internal/template"
)

// PlanOptions configures component selection, template composition, and existing-file policies.
type PlanOptions struct {
	// Force replaces conflicting regular files whose policy is block.
	// Protected skills and rules are never overwritten even when Force is true.
	Force bool
	// Components selects optional components from the primary template by ID.
	// When ComponentsSpecified is false, all components marked Default in the manifest are included.
	Components          []string
	ComponentsSpecified bool
	// ExtraTemplateID optionally composes an additional template (such as agent-kit) into the same plan.
	ExtraTemplateID string
	// ExtraComponents selects optional components from ExtraTemplateID by ID.
	// When ExtraComponentsSpecified is false, all default components from ExtraTemplateID are included.
	ExtraComponents          []string
	ExtraComponentsSpecified bool
	// AgentsMDMode controls how existing appendable files such as AGENTS.md are handled.
	// Defaults to ExistingAppend when empty.
	AgentsMDMode ExistingMode
}

// Engine renders embedded templates and inspects the target directory to build a deterministic Plan.
type Engine struct{ Registry templates.TemplateRegistry }

// Plan builds a deterministic scaffold plan for templateID using default component selection.
func (e Engine) Plan(ctx context.Context, templateID, target string, vars Variables, force bool) (Plan, error) {
	return e.PlanWithOptions(ctx, templateID, target, vars, PlanOptions{Force: force})
}

type renderedSpec struct {
	path         string
	content      []byte
	executable   bool
	policy       templates.ConflictPolicy
	appendMarker string
	protectRoot  string
	component    string
}

// PlanWithOptions builds a deterministic scaffold plan supporting component selection,
// template composition, idempotent append behavior, and skill/rule overwrite protection.
func (e Engine) PlanWithOptions(ctx context.Context, templateID, target string, vars Variables, opts PlanOptions) (Plan, error) {
	p := Plan{TemplateID: templateID, TargetDir: target, Operations: []Operation{}, Warnings: []string{}}
	agentsMode, err := normalizeExistingMode(opts.AgentsMDMode)
	if err != nil {
		return p, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	t, err := e.Registry.Get(templateID)
	if err != nil {
		return p, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	p.TemplateVersion = t.Manifest.Version
	p.NextSteps = append([]string(nil), t.Manifest.NextSteps...)
	p.NextStepsNote = t.Manifest.NextStepsNote

	specs := []renderedSpec{}
	specIndex := map[string]int{}
	if err := collectTemplateSpecs(ctx, t, vars, opts.Components, opts.ComponentsSpecified, true, &specs, specIndex); err != nil {
		return p, err
	}
	if opts.ExtraTemplateID != "" && opts.ExtraTemplateID != templateID {
		extra, err := e.Registry.Get(opts.ExtraTemplateID)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		if err := collectTemplateSpecs(ctx, extra, vars, opts.ExtraComponents, opts.ExtraComponentsSpecified, false, &specs, specIndex); err != nil {
			return p, err
		}
	}

	root, err := os.OpenRoot(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return p, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if root != nil {
		defer root.Close()
	}

	warnedProtected := map[string]bool{}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		op, warning, err := planSpec(root, spec, opts.Force, agentsMode)
		if err != nil {
			return p, err
		}
		if warning != "" {
			warnKey := strings.ToLower(spec.protectRoot)
			if warnKey == "" {
				warnKey = strings.ToLower(spec.path)
			}
			if !warnedProtected[warnKey] {
				warnedProtected[warnKey] = true
				p.Warnings = append(p.Warnings, warning)
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
	if opts.Force {
		p.Warnings = append(p.Warnings, "Only listed overwrite operations will replace existing files.")
	}
	return p, nil
}

// HasExistingFile reports whether rel currently exists as a regular file inside target.
// Missing target directories return false without error.
func HasExistingFile(target, rel string) (bool, error) {
	if !platform.ValidRelative(rel) {
		return false, fmt.Errorf("%w: unsafe path %q", ErrValidation, rel)
	}
	resolved, err := platform.ResolveTarget(target)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

// ParseExistingMode validates and normalizes a user-supplied existing-file mode string.
func ParseExistingMode(value string) (ExistingMode, error) {
	return normalizeExistingMode(ExistingMode(strings.ToLower(strings.TrimSpace(value))))
}

func normalizeExistingMode(mode ExistingMode) (ExistingMode, error) {
	if mode == "" {
		return ExistingAppend, nil
	}
	switch mode {
	case ExistingAppend, ExistingReplace, ExistingSkip:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid existing-file mode %q (expected append, replace, or skip)", mode)
	}
}

// resolveComponents validates the requested component IDs against manifest declarations
// and returns the active component set.
func resolveComponents(m templates.Manifest, requested []string, specified, requireNonEmpty bool) (map[string]bool, error) {
	if len(m.Components) == 0 {
		if len(requested) > 0 {
			return nil, fmt.Errorf("%w: template %q does not define selectable components", ErrValidation, m.ID)
		}
		return nil, nil
	}
	valid := make(map[string]bool, len(m.Components))
	defaults := make(map[string]bool, len(m.Components))
	for _, c := range m.Components {
		valid[c.ID] = true
		if c.Default {
			defaults[c.ID] = true
		}
	}
	if !specified {
		return defaults, nil
	}
	selected := make(map[string]bool, len(requested))
	for _, raw := range requested {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if strings.EqualFold(id, "all") {
			for defID := range defaults {
				selected[defID] = true
			}
			continue
		}
		if !valid[id] {
			return nil, fmt.Errorf("%w: unknown component %q for template %q", ErrValidation, id, m.ID)
		}
		selected[id] = true
	}
	if requireNonEmpty && len(selected) == 0 {
		return nil, fmt.Errorf("%w: select at least one component for %q", ErrValidation, m.ID)
	}
	return selected, nil
}

// collectTemplateSpecs renders selected files from a template and merges append_lines files
// when composing onto an earlier template's planned destination (such as .gitignore).
func collectTemplateSpecs(ctx context.Context, t templates.Template, vars Variables, requested []string, specified, requireNonEmpty bool, specs *[]renderedSpec, specIndex map[string]int) error {
	selected, err := resolveComponents(t.Manifest, requested, specified, requireNonEmpty)
	if err != nil {
		return err
	}
	for _, f := range t.Manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if f.Component != "" && selected != nil && !selected[f.Component] {
			continue
		}
		dest, err := Render("destination", []byte(f.Destination), vars)
		if err != nil || !platform.ValidRelative(string(dest)) {
			return fmt.Errorf("%w: unsafe destination %q", ErrValidation, dest)
		}
		relPath := string(dest)
		data, err := fs.ReadFile(t.Files, f.Source)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrValidation, err)
		}
		content, err := Render(f.Source, data, vars)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrValidation, err)
		}
		protectRoot := f.ProtectRoot
		if protectRoot != "" {
			renderedRoot, err := Render("protect_root", []byte(protectRoot), vars)
			if err != nil || !platform.ValidRelative(string(renderedRoot)) {
				return fmt.Errorf("%w: unsafe protect_root %q", ErrValidation, renderedRoot)
			}
			protectRoot = string(renderedRoot)
		}
		key := strings.ToLower(relPath)
		if idx, exists := specIndex[key]; exists {
			// When composing templates (e.g. python-minimal + agent-kit), merge line-append
			// snippets such as .gitignore entries into the already-planned file.
			if f.ConflictPolicy == templates.PolicyAppendLines {
				merged, _ := mergeMissingLines((*specs)[idx].content, content)
				(*specs)[idx].content = merged
				continue
			}
			return fmt.Errorf("%w: duplicate destination %s", ErrValidation, relPath)
		}
		specIndex[key] = len(*specs)
		*specs = append(*specs, renderedSpec{
			path:         relPath,
			content:      content,
			executable:   f.Executable,
			policy:       f.ConflictPolicy,
			appendMarker: f.AppendMarker,
			protectRoot:  protectRoot,
			component:    f.Component,
		})
	}
	return nil
}

// planSpec computes one Operation and optional warning for spec against root.
func planSpec(root *os.Root, spec renderedSpec, force bool, agentsMode ExistingMode) (Operation, string, error) {
	op := Operation{Path: spec.path, Action: Create, Mode: 0o644, Content: spec.content}
	if spec.executable {
		op.Mode = 0o755
	}
	if root == nil {
		return op, "", nil
	}

	switch spec.policy {
	case templates.PolicyProtect:
		protectRoot := spec.protectRoot
		if protectRoot == "" {
			protectRoot = spec.path
		}
		if err := checkAncestors(root, protectRoot); err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		exists, err := existsEntry(root, protectRoot)
		if err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		if !exists && protectRoot != spec.path {
			if err := checkAncestors(root, spec.path); err != nil {
				return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
			}
			exists, err = existsEntry(root, spec.path)
			if err != nil {
				return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
			}
		}
		if exists {
			op.Action = Protected
			op.Reason = "existing customization preserved"
			op.Content = nil
			return op, protectedWarning(spec, protectRoot), nil
		}
		if err := platform.CheckPath(root, spec.path); err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return op, "", nil

	case templates.PolicyAppend:
		if err := platform.CheckPath(root, spec.path); err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		existing, err := root.ReadFile(spec.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return op, "", nil
			}
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		info, err := root.Stat(spec.path)
		if err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		op.Mode = preserveMode(info.Mode().Perm(), spec.executable)
		op.ExpectedHash = digest(existing)
		switch agentsMode {
		case ExistingSkip:
			op.Action = Skip
			op.Reason = "existing file kept"
			op.Content = existing
		case ExistingReplace:
			if string(existing) == string(spec.content) && op.Mode == info.Mode().Perm() {
				op.Action = Skip
			} else {
				op.Action = Overwrite
				op.Reason = "replace existing file"
			}
		default: // ExistingAppend
			merged, changed := mergeMarkedBlock(existing, spec.content, spec.appendMarker)
			if !changed && op.Mode == info.Mode().Perm() {
				op.Action = Skip
				op.Reason = "already contains managed section"
				op.Content = existing
			} else {
				op.Action = Append
				op.Reason = "append managed section"
				op.Content = merged
			}
		}
		return op, "", nil

	case templates.PolicyAppendLines:
		if err := platform.CheckPath(root, spec.path); err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		existing, err := root.ReadFile(spec.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return op, "", nil
			}
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		info, err := root.Stat(spec.path)
		if err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		op.Mode = preserveMode(info.Mode().Perm(), spec.executable)
		op.ExpectedHash = digest(existing)
		merged, changed := mergeMissingLines(existing, spec.content)
		if !changed && op.Mode == info.Mode().Perm() {
			op.Action = Skip
			op.Content = existing
		} else {
			op.Action = Append
			op.Reason = "append missing entries"
			op.Content = merged
		}
		return op, "", nil

	default: // PolicyBlock
		if err := platform.CheckPath(root, spec.path); err != nil {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		existing, err := root.ReadFile(spec.path)
		if err == nil {
			info, err := root.Stat(spec.path)
			if err != nil {
				return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
			}
			op.Mode = preserveMode(info.Mode().Perm(), spec.executable)
			op.ExpectedHash = digest(existing)
			switch {
			case string(existing) == string(spec.content) && op.Mode == info.Mode().Perm():
				op.Action = Skip
			case force:
				op.Action = Overwrite
			default:
				op.Action = Conflict
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return op, "", fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return op, "", nil
	}
}

func preserveMode(existing fs.FileMode, executable bool) fs.FileMode {
	mode := existing
	if executable && runtime.GOOS != "windows" {
		mode |= 0o111
	}
	return mode
}

// checkAncestors ensures every existing ancestor strictly above rel is a real
// directory (never a symlink or regular file).
func checkAncestors(root *os.Root, rel string) error {
	if !platform.ValidRelative(rel) {
		return fmt.Errorf("unsafe template path %q", rel)
	}
	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts)-1; i++ {
		prefix := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink destination refused: %s", prefix)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent is not a directory: %s", prefix)
		}
	}
	return nil
}

// existsEntry reports whether rel (or a case-equivalent sibling in its parent directory)
// exists under root without traversing symlinks at rel.
func existsEntry(root *os.Root, rel string) (bool, error) {
	if _, err := root.Lstat(rel); err == nil {
		return true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	dir := path.Dir(rel)
	base := path.Base(rel)
	entries, err := fs.ReadDir(root.FS(), dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), base) {
			return true, nil
		}
	}
	return false, nil
}

func protectedWarning(spec renderedSpec, protectRoot string) string {
	switch {
	case strings.HasPrefix(spec.component, "skill:"):
		name := strings.TrimPrefix(spec.component, "skill:")
		return fmt.Sprintf("Protected existing skill %q (%s) — skipped so your files are not replaced.", name, protectRoot)
	case strings.HasPrefix(spec.component, "rule:"):
		name := strings.TrimPrefix(spec.component, "rule:")
		return fmt.Sprintf("Protected existing rule %q (%s) — skipped so your files are not replaced.", name, protectRoot)
	default:
		return fmt.Sprintf("Protected existing %s — skipped so your files are not replaced.", protectRoot)
	}
}

// mergeMarkedBlock appends addition wrapped in optional HTML markers to existing
// unless existing already equals or contains the managed section.
func mergeMarkedBlock(existing, addition []byte, marker string) ([]byte, bool) {
	trimmedAdd := strings.TrimSpace(string(addition))
	if trimmedAdd == "" {
		return existing, false
	}
	existingStr := string(existing)
	if strings.TrimSpace(existingStr) == trimmedAdd || strings.Contains(existingStr, trimmedAdd) {
		return existing, false
	}
	block := trimmedAdd
	if marker != "" {
		startTag := "<!-- " + marker + ":start -->"
		endTag := "<!-- " + marker + ":end -->"
		if strings.Contains(existingStr, startTag) && strings.Contains(existingStr, endTag) {
			return existing, false
		}
		block = startTag + "\n" + trimmedAdd + "\n" + endTag
	}
	base := strings.TrimRight(existingStr, "\r\n")
	if base == "" {
		return []byte(block + "\n"), true
	}
	return []byte(base + "\n\n" + block + "\n"), true
}

// mergeMissingLines appends any non-empty line in addition that is not already present in existing.
func mergeMissingLines(existing, addition []byte) ([]byte, bool) {
	existingStr := string(existing)
	present := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(existingStr, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		present[trimmed] = true
		present[strings.TrimSuffix(trimmed, "/")] = true
	}
	var missing []string
	for _, line := range strings.Split(strings.ReplaceAll(string(addition), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if present[trimmed] || present[strings.TrimSuffix(trimmed, "/")] {
			continue
		}
		present[trimmed] = true
		present[strings.TrimSuffix(trimmed, "/")] = true
		missing = append(missing, trimmed)
	}
	if len(missing) == 0 {
		return existing, false
	}
	base := strings.TrimRight(existingStr, "\r\n")
	if base == "" {
		return []byte(strings.Join(missing, "\n") + "\n"), true
	}
	return []byte(base + "\n" + strings.Join(missing, "\n") + "\n"), true
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
