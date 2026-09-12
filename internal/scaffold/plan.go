package scaffold

import (
	"context"
	"errors"
)

var (
	ErrValidation = errors.New("validation failed")
	ErrConflict   = errors.New("file conflict")
	ErrExecution  = errors.New("generation failed")
)

type Plan struct {
	TemplateID      string      `json:"template_id"`
	TemplateVersion string      `json:"template_version"`
	TargetDir       string      `json:"target_dir"`
	Operations      []Operation `json:"operations"`
	Warnings        []string    `json:"warnings"`
}

func (p Plan) HasConflicts() bool {
	for _, op := range p.Operations {
		if op.Action == Conflict {
			return true
		}
	}
	return false
}

type Result struct {
	TargetDir string   `json:"target_dir"`
	Completed []string `json:"completed"`
	Skipped   []string `json:"skipped"`
	NextSteps []string `json:"next_steps"`
}

func NewResult(target string) Result {
	return Result{TargetDir: target, Completed: []string{}, Skipped: []string{}, NextSteps: []string{}}
}

type Executor interface {
	Apply(context.Context, Plan) (Result, error)
}
