package skill

import (
	"context"
	"errors"
)

const (
	DefaultRepo       = "AIAI-Laboratory/aiai-skills"
	DefaultArchiveURL = "https://codeload.github.com/AIAI-Laboratory/aiai-skills/tar.gz/refs/heads/main"

	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

var (
	ErrConflict   = errors.New("skill conflict")
	ErrIO         = errors.New("skill I/O failure")
	ErrValidation = errors.New("invalid skill request")
)

type Scope string

type Descriptor struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
}

type Result struct {
	Action      string       `json:"action"`
	Scope       Scope        `json:"scope,omitempty"`
	Destination string       `json:"destination,omitempty"`
	Skills      []Descriptor `json:"skills"`
}

type Service interface {
	Available(ctx context.Context) (*Result, error)
	List(scope Scope) (*Result, error)
	Install(ctx context.Context, names []string, scope Scope, all, overwrite bool) (*Result, error)
	Remove(names []string, scope Scope, all bool) (*Result, error)
	Update(ctx context.Context, names []string, scope Scope, all bool) (*Result, error)
}
