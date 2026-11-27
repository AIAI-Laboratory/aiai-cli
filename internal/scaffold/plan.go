package scaffold

import (
	"context"
	"errors"
)

var (
	// ErrValidation indicates invalid input, template selection, or plan metadata.
	ErrValidation = errors.New("validation failed")
	// ErrConflict indicates a filesystem conflict, unsafe symlink, or stale preview.
	ErrConflict = errors.New("file conflict")
	// ErrExecution indicates an I/O or execution error while writing files.
	ErrExecution = errors.New("generation failed")
)

// Plan is the deterministic, reviewed set of file operations produced before writing.
type Plan struct {
	TemplateID      string      `json:"template_id"`
	TemplateVersion string      `json:"template_version"`
	TargetDir       string      `json:"target_dir"`
	Operations      []Operation `json:"operations"`
	Warnings        []string    `json:"warnings"`
	NextSteps       []string    `json:"next_steps,omitempty"`
	NextStepsNote   string      `json:"next_steps_note,omitempty"`
}

// HasConflicts reports whether any operation in the plan is a blocking Conflict.
func (p Plan) HasConflicts() bool {
	return p.ConflictCount() > 0
}

// ConflictCount returns the number of blocking Conflict operations in the plan.
func (p Plan) ConflictCount() int {
	count := 0
	for _, op := range p.Operations {
		if op.Action == Conflict {
			count++
		}
	}
	return count
}

// OverwriteCount returns the number of Overwrite operations in the plan.
func (p Plan) OverwriteCount() int {
	count := 0
	for _, op := range p.Operations {
		if op.Action == Overwrite {
			count++
		}
	}
	return count
}

// HasOverwrites reports whether any operation in the plan overwrites an existing file.
func (p Plan) HasOverwrites() bool {
	return p.OverwriteCount() > 0
}

// RequiresConfirmation reports whether applying the plan involves conflicts or overwrites.
func (p Plan) RequiresConfirmation() bool {
	return p.HasConflicts() || p.HasOverwrites()
}

// ResolveExisting returns a copy of Plan that resolves existing-file conflicts and overwrites.
// When replace is true, Conflict operations become Overwrite; when replace is false, both
// Conflict and Overwrite operations become Skip so existing files stay untouched.
// Protected skills and rules are never modified.
func (p Plan) ResolveExisting(replace bool) Plan {
	resolved := p
	resolved.Operations = make([]Operation, len(p.Operations))
	resolved.Warnings = append([]string(nil), p.Warnings...)
	resolved.NextSteps = append([]string(nil), p.NextSteps...)
	for i, op := range p.Operations {
		if op.Content != nil {
			op.Content = append([]byte(nil), op.Content...)
		}
		switch op.Action {
		case Conflict:
			if replace {
				op.Action = Overwrite
				op.Reason = "replace existing file"
			} else {
				op.Action = Skip
				op.Reason = "existing file kept"
				op.Content = nil
			}
		case Overwrite:
			if !replace {
				op.Action = Skip
				op.Reason = "existing file kept"
				op.Content = nil
			}
		}
		resolved.Operations[i] = op
	}
	return resolved
}

// Result records files written, skipped, protected, and follow-up commands after Apply.
type Result struct {
	TargetDir     string   `json:"target_dir"`
	Completed     []string `json:"completed"`
	Skipped       []string `json:"skipped"`
	Protected     []string `json:"protected,omitempty"`
	NextSteps     []string `json:"next_steps"`
	NextStepsNote string   `json:"next_steps_note,omitempty"`
}

// NewResult creates an initialized Result for target with non-nil Completed, Skipped, and NextSteps slices.
func NewResult(target string) Result {
	return Result{TargetDir: target, Completed: []string{}, Skipped: []string{}, NextSteps: []string{}}
}

// Executor applies a previously planned set of filesystem operations.
type Executor interface {
	Apply(context.Context, Plan) (Result, error)
}
