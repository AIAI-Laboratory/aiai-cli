// Package templates contains the immutable assets shipped with the executable.
package templates

import "embed"

// Files includes dotfiles and underscored Python modules.
//
//go:embed all:python-minimal
var Files embed.FS
