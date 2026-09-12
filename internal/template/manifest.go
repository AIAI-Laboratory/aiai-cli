package template

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"go.yaml.in/yaml/v3"
)

type TemplateMetadata struct {
	ID          string `yaml:"id" json:"id"`
	Version     string `yaml:"version" json:"version"`
	DisplayName string `yaml:"display_name" json:"display_name"`
	Description string `yaml:"description" json:"description"`
}

type File struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Executable  bool   `yaml:"executable,omitempty"`
}

type Manifest struct {
	TemplateMetadata `yaml:",inline"`
	Variables        []string `yaml:"variables"`
	Files            []File   `yaml:"files"`
}

func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return m, fmt.Errorf("manifest must contain exactly one document")
	}
	if m.ID == "" || m.Version == "" || m.DisplayName == "" || m.Description == "" || len(m.Files) == 0 {
		return m, fmt.Errorf("manifest is missing required metadata or files")
	}
	vars := map[string]bool{}
	for _, v := range m.Variables {
		if (v != "project_name" && v != "package_name") || vars[v] {
			return m, fmt.Errorf("invalid or duplicate variable %q", v)
		}
		vars[v] = true
	}
	if !vars["project_name"] || !vars["package_name"] {
		return m, fmt.Errorf("manifest must declare project_name and package_name")
	}
	destinations := map[string]bool{}
	for _, f := range m.Files {
		if !fs.ValidPath(f.Source) || strings.ContainsAny(f.Source, `\:`) || f.Destination == "" || destinations[f.Destination] {
			return m, fmt.Errorf("invalid source or duplicate destination: %q", f.Source)
		}
		destinations[f.Destination] = true
	}
	return m, nil
}
