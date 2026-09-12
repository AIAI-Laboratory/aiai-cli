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

func TestManifestValidation(t *testing.T) {
	if _, err := ParseManifest([]byte(validManifest)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		validManifest + "typo: value\n",
		validManifest + "---\nid: other\n",
		strings.Replace(validManifest, "file.tmpl", "../file", 1),
		strings.Replace(validManifest, "package_name", "shell", 1),
		strings.Replace(validManifest, "package_name", "project_name", 1),
		validManifest + "  - source: other\n    destination: file\n",
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
