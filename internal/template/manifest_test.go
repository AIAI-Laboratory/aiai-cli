package template

import (
	"strings"
	"testing"
	"testing/fstest"
)

const validManifest = `id: test
version: "1"
display_name: Test
description: Example
variables: [project_name, package_name]
files:
  - source: file.tmpl
    destination: file
`

const validAgentManifest = `id: agent-test
version: "1"
kind: agent
default_order: 1
display_name: Agent Setup
description: Rules and skills
variables: []
next_steps:
  - Review AGENTS.md
next_steps_note: Existing skills are protected.
components:
  - id: agents-md
    name: AGENTS.md
    description: Root agent instructions
    default: true
  - id: rule:docstring
    name: Docstring Rule
    description: Google-style docstrings
    default: true
files:
  - source: AGENTS.md.tmpl
    destination: AGENTS.md
    component: agents-md
    conflict_policy: append
    append_marker: aiai:agent-test
  - source: docstring.md.tmpl
    destination: .agents/rules/docstring.md
    component: rule:docstring
    conflict_policy: protect
    protect_root: .agents/rules/docstring.md
`

func TestManifestValidation(t *testing.T) {
	if _, err := ParseManifest([]byte(validManifest)); err != nil {
		t.Fatal(err)
	}
	agentM, err := ParseManifest([]byte(validAgentManifest))
	if err != nil {
		t.Fatal(err)
	}
	if agentM.Kind != KindAgent || len(agentM.Components) != 2 || agentM.Files[1].ConflictPolicy != PolicyProtect {
		t.Fatalf("unexpected parsed agent manifest: %+v", agentM)
	}
	for _, data := range []string{
		validManifest + "typo: value\n",
		validManifest + "---\nid: other\n",
		strings.Replace(validManifest, "file.tmpl", "../file", 1),
		strings.Replace(validManifest, "package_name", "shell", 1),
		strings.Replace(validManifest, "package_name", "project_name", 1),
		validManifest + "  - source: other\n    destination: file\n",
		strings.Replace(validAgentManifest, "kind: agent", "kind: unknown", 1),
		strings.Replace(validAgentManifest, "id: agents-md", "id: all", 1),
		strings.Replace(validAgentManifest, "component: agents-md", "component: missing", 1),
		strings.Replace(validAgentManifest, "conflict_policy: protect", "conflict_policy: nuke", 1),
		strings.Replace(validAgentManifest, "protect_root: .agents/rules/docstring.md", "protect_root: ../escape", 1),
	} {
		if _, err := ParseManifest([]byte(data)); err == nil {
			t.Errorf("accepted malformed manifest:\n%s", data)
		}
	}
}

func TestRegistryRequiresEveryAsset(t *testing.T) {
	assets := fstest.MapFS{"test/manifest.yaml": &fstest.MapFile{Data: []byte(validManifest)}}
	if _, err := NewRegistry(assets, "test"); err == nil {
		t.Fatal("accepted a missing template file")
	}
	assets["test/files/file.tmpl"] = &fstest.MapFile{Data: []byte("example")}
	r, err := NewRegistry(assets, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get("missing"); err == nil {
		t.Fatal("accepted unknown ID")
	}
	if _, err := NewRegistry(assets, "test", "test"); err == nil {
		t.Fatal("accepted duplicate ID")
	}
}

func TestEmbeddedTemplatesOrderAndMetadata(t *testing.T) {
	r, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 embedded templates, got %d", len(list))
	}
	if list[0].ID != AgentKitID || list[0].Kind != KindAgent {
		t.Fatalf("expected agent-kit first, got %+v", list[0])
	}
	if len(list[0].Components) != 6 {
		t.Fatalf("expected 6 selectable agent components, got %d", len(list[0].Components))
	}
	if list[1].ID != PythonMinimalID || list[1].Kind != KindProject {
		t.Fatalf("expected python-minimal second, got %+v", list[1])
	}
}
