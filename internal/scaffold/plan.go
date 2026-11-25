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
	for _, op := range p.Operations {
		if op.Action == Conflict {
			return true
		}
	}
	return false
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
