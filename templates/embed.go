// Package templates contains the immutable assets shipped with the executable.
package templates

import "embed"

// Files includes embedded templates, dotfiles, and underscored Python modules.
//
//go:embed all:agent-kit all:python-minimal
var Files embed.FS
