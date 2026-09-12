package template

import "github.com/AIAI-Laboratory/aiai-cli/templates"

func Embedded() (*Registry, error) {
	return NewRegistry(templates.Files, "python-minimal")
}
