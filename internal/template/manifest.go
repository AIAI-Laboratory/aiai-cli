package template

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Kind identifies whether a template scaffolds a full project or an agent setup kit.
type Kind string

const (
	// KindProject identifies a full language or framework project template.
	KindProject Kind = "project"
	// KindAgent identifies a reusable AI agent configuration kit (AGENTS.md, rules, skills, docs).
	KindAgent Kind = "agent"
)

// ConflictPolicy defines how the scaffold engine behaves when a destination path already exists.
type ConflictPolicy string

const (
	// PolicyBlock treats differing existing files as blocking conflicts unless --force is set.
	PolicyBlock ConflictPolicy = "block"
	// PolicyAppend appends a managed block to an existing file by default (or replaces/skips per user choice).
	PolicyAppend ConflictPolicy = "append"
	// PolicyAppendLines appends missing lines (such as .gitignore entries) to an existing file.
	PolicyAppendLines ConflictPolicy = "append_lines"
	// PolicyProtect never overwrites an existing user file or directory, even when --force is set.
	PolicyProtect ConflictPolicy = "protect"
)

// Component describes an optional selectable resource group declared by a template.
type Component struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Default     bool   `yaml:"default" json:"default"`
}

// TemplateMetadata describes a template's identity, display information, and optional components.
type TemplateMetadata struct {
	ID           string      `yaml:"id" json:"id"`
	Version      string      `yaml:"version" json:"version"`
	Kind         Kind        `yaml:"kind,omitempty" json:"kind,omitempty"`
	DefaultOrder int         `yaml:"default_order,omitempty" json:"default_order,omitempty"`
	DisplayName  string      `yaml:"display_name" json:"display_name"`
	Description  string      `yaml:"description" json:"description"`
	Components   []Component `yaml:"components,omitempty" json:"components,omitempty"`
}

// File declares one embedded template asset and its destination, component, and conflict behavior.
type File struct {
	Source         string         `yaml:"source"`
	Destination    string         `yaml:"destination"`
	Executable     bool           `yaml:"executable,omitempty"`
	Component      string         `yaml:"component,omitempty"`
	ConflictPolicy ConflictPolicy `yaml:"conflict_policy,omitempty"`
	AppendMarker   string         `yaml:"append_marker,omitempty"`
	ProtectRoot    string         `yaml:"protect_root,omitempty"`
}

// Manifest is the strictly validated YAML definition loaded from a template's manifest.yaml.
type Manifest struct {
	TemplateMetadata `yaml:",inline"`
	NextSteps        []string `yaml:"next_steps,omitempty"`
	NextStepsNote    string   `yaml:"next_steps_note,omitempty"`
	Variables        []string `yaml:"variables"`
	Files            []File   `yaml:"files"`
}

// ParseManifest decodes and strictly validates a template manifest document.
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
	if m.Kind == "" {
		m.Kind = KindProject
	}
	if m.Kind != KindProject && m.Kind != KindAgent {
		return m, fmt.Errorf("invalid template kind %q", m.Kind)
	}
	vars := map[string]bool{}
	for _, v := range m.Variables {
		if (v != "project_name" && v != "package_name") || vars[v] {
			return m, fmt.Errorf("invalid or duplicate variable %q", v)
		}
		vars[v] = true
	}
	if m.Kind == KindProject && (!vars["project_name"] || !vars["package_name"]) {
		return m, fmt.Errorf("manifest must declare project_name and package_name")
	}
	components := map[string]bool{}
	for _, c := range m.Components {
		if c.ID == "" || c.Name == "" || strings.EqualFold(c.ID, "all") || components[c.ID] {
			return m, fmt.Errorf("invalid or duplicate component %q", c.ID)
		}
		components[c.ID] = true
	}
	destinations := map[string]bool{}
	for i := range m.Files {
		f := &m.Files[i]
		if !fs.ValidPath(f.Source) || strings.ContainsAny(f.Source, `\:`) || f.Destination == "" || destinations[f.Destination] {
			return m, fmt.Errorf("invalid source or duplicate destination: %q", f.Source)
		}
		destinations[f.Destination] = true
		if f.Component != "" && !components[f.Component] {
			return m, fmt.Errorf("file %q references unknown component %q", f.Source, f.Component)
		}
		if f.ConflictPolicy == "" {
			f.ConflictPolicy = PolicyBlock
		}
		switch f.ConflictPolicy {
		case PolicyBlock, PolicyAppend, PolicyAppendLines, PolicyProtect:
		default:
			return m, fmt.Errorf("invalid conflict policy %q for %q", f.ConflictPolicy, f.Source)
		}
		if f.ProtectRoot != "" && (!fs.ValidPath(f.ProtectRoot) || strings.ContainsAny(f.ProtectRoot, `\:`)) {
			return m, fmt.Errorf("invalid protect_root %q for %q", f.ProtectRoot, f.Source)
		}
	}
	return m, nil
}
