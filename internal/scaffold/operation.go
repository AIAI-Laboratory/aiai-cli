package scaffold

import "io/fs"

// Action describes the planned filesystem action for a single destination path.
type Action string

const (
	// Create writes a new file atomically without replacing any existing file.
	Create Action = "create"
	// Append merges managed content into an existing regular file atomically.
	Append Action = "append"
	// Overwrite replaces an existing regular file atomically.
	Overwrite Action = "overwrite"
	// Skip leaves an existing file unchanged because it already matches or was skipped.
	Skip Action = "skip"
	// Protected leaves an existing user-owned skill or rule untouched and never overwrites it.
	Protected Action = "protected"
	// Conflict blocks execution because an existing regular file differs without --force.
	Conflict Action = "conflict"
)

// ExistingMode controls how appendable files such as AGENTS.md are handled when already present.
type ExistingMode string

const (
	// ExistingAppend appends managed content into the existing file (default).
	ExistingAppend ExistingMode = "append"
	// ExistingReplace overwrites the existing file with the rendered template.
	ExistingReplace ExistingMode = "replace"
	// ExistingSkip leaves the existing file untouched.
	ExistingSkip ExistingMode = "skip"
)

// Operation describes one planned file operation, its permissions, and content.
type Operation struct {
	Path         string      `json:"path"`
	Action       Action      `json:"action"`
	Mode         fs.FileMode `json:"mode"`
	Reason       string      `json:"reason,omitempty"`
	Content      []byte      `json:"-"`
	ExpectedHash string      `json:"-"`
}
