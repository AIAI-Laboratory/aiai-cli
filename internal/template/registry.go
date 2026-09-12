package template

import (
	"fmt"
	"io/fs"
	"sort"
)

type Template struct {
	Manifest Manifest
	Files    fs.FS
}

type TemplateRegistry interface {
	Get(id string) (Template, error)
	List() []TemplateMetadata
}

type Registry struct{ templates map[string]Template }

func NewRegistry(assets fs.FS, ids ...string) (*Registry, error) {
	r := &Registry{templates: make(map[string]Template)}
	for _, id := range ids {
		data, err := fs.ReadFile(assets, id+"/manifest.yaml")
		if err != nil {
			return nil, err
		}
		m, err := ParseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", id, err)
		}
		if m.ID != id {
			return nil, fmt.Errorf("template ID %q does not match directory %q", m.ID, id)
		}
		if _, found := r.templates[id]; found {
			return nil, fmt.Errorf("duplicate template %s", id)
		}
		files, err := fs.Sub(assets, id+"/files")
		if err != nil {
			return nil, err
		}
		for _, f := range m.Files {
			if _, err := fs.ReadFile(files, f.Source); err != nil {
				return nil, fmt.Errorf("template %s: %w", id, err)
			}
		}
		r.templates[id] = Template{Manifest: m, Files: files}
	}
	return r, nil
}

func (r *Registry) Get(id string) (Template, error) {
	t, ok := r.templates[id]
	if !ok {
		return Template{}, fmt.Errorf("unknown template %q", id)
	}
	return t, nil
}

func (r *Registry) List() []TemplateMetadata {
	items := make([]TemplateMetadata, 0, len(r.templates))
	for _, t := range r.templates {
		items = append(items, t.Manifest.TemplateMetadata)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
