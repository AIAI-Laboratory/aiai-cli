package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestSkillLifecycle(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"aiai-skills-main/skills/alpha/SKILL.md":            skillMarkdown("alpha", "Alpha skill"),
		"aiai-skills-main/skills/alpha/references/guide.md": "guide",
		"aiai-skills-main/skills/beta/SKILL.md":             skillMarkdown("beta", "Beta skill"),
	})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Write(archive)
	}))
	defer server.Close()

	workDir := t.TempDir()
	service := NewService("test")
	service.ArchiveURL = server.URL
	service.WorkDir = workDir

	available, err := service.Available(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(available.Skills) != 2 || available.Skills[0].Name != "alpha" || available.Skills[1].Name != "beta" {
		t.Fatalf("unexpected available skills: %+v", available.Skills)
	}

	installed, err := service.Install(context.Background(), []string{"alpha"}, ScopeProject, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed.Skills) != 1 || installed.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected install result: %+v", installed)
	}
	guide := filepath.Join(workDir, ".agents", "skills", "alpha", "references", "guide.md")
	if body, err := os.ReadFile(guide); err != nil || string(body) != "guide" {
		t.Fatalf("installed guide = %q, %v", body, err)
	}

	requestsBeforeConflict := requests.Load()
	if _, err := service.Install(context.Background(), []string{"alpha"}, ScopeProject, false, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("second install error = %v, want ErrConflict", err)
	}
	if requests.Load() != requestsBeforeConflict {
		t.Fatalf("conflicting install made an unnecessary network request: before=%d after=%d", requestsBeforeConflict, requests.Load())
	}
	if _, err := service.Update(context.Background(), nil, ScopeProject, false); err != nil {
		t.Fatalf("update: %v", err)
	}

	listed, err := service.List(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Skills) != 1 || listed.Skills[0].Description != "Alpha skill" {
		t.Fatalf("unexpected installed skills: %+v", listed.Skills)
	}

	removed, err := service.Remove([]string{"alpha"}, ScopeProject, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Skills) != 1 {
		t.Fatalf("unexpected remove result: %+v", removed)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(guide))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("skill directory still exists: %v", err)
	}
}

func TestArchiveRejectsLinks(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "aiai-skills-main/skills/alpha/SKILL.md",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../outside",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := parseArchive(bytes.NewReader(buf.Bytes()), archiveOptions{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("parseArchive error = %v, want ErrValidation", err)
	}
}

func TestArchiveFiltersSkillsAndMetadata(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"repo/skills/alpha/SKILL.md":            skillMarkdown("alpha", "Alpha skill"),
		"repo/skills/alpha/references/guide.md": "guide",
		"repo/skills/beta/SKILL.md":             skillMarkdown("beta", "Beta skill"),
	})
	skills, err := parseArchive(bytes.NewReader(archive), archiveOptions{
		names:        map[string]struct{}{"alpha": {}},
		metadataOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].descriptor.Name != "alpha" {
		t.Fatalf("unexpected filtered skills: %+v", skills)
	}
	if len(skills[0].files) != 1 {
		t.Fatalf("metadata-only parse retained extra files: %v", skills[0].files)
	}
	if _, ok := skills[0].files["SKILL.md"]; !ok {
		t.Fatal("metadata-only parse did not retain SKILL.md")
	}
}

func TestInstallValidatesSelectionBeforeWriting(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"repo/skills/alpha/SKILL.md": skillMarkdown("alpha", "Alpha skill"),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	}))
	defer server.Close()

	workDir := t.TempDir()
	service := NewService("test")
	service.ArchiveURL = server.URL
	service.WorkDir = workDir
	_, err := service.Install(context.Background(), []string{"missing"}, ScopeProject, false, false)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Install error = %v, want ErrValidation", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install wrote before selection validation: %v", err)
	}
}

func TestRemoveAllPreservesUnmanagedSkills(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"repo/skills/alpha/SKILL.md": skillMarkdown("alpha", "Alpha skill"),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	}))
	defer server.Close()

	workDir := t.TempDir()
	service := NewService("test")
	service.ArchiveURL = server.URL
	service.WorkDir = workDir
	if _, err := service.Install(context.Background(), []string{"alpha"}, ScopeProject, false, false); err != nil {
		t.Fatal(err)
	}
	unmanagedDir := filepath.Join(workDir, ".agents", "skills", "third-party")
	if err := os.MkdirAll(unmanagedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanagedDir, "SKILL.md"), []byte(skillMarkdown("third-party", "Third-party skill")), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := service.Remove(nil, ScopeProject, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Skills) != 1 || removed.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected removed skills: %+v", removed.Skills)
	}
	if _, err := os.Stat(filepath.Join(unmanagedDir, "SKILL.md")); err != nil {
		t.Fatalf("unmanaged skill was removed: %v", err)
	}
}

func TestUpdateRestoresMissingManagedSkill(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"repo/skills/alpha/SKILL.md": skillMarkdown("alpha", "Alpha skill"),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	}))
	defer server.Close()

	workDir := t.TempDir()
	service := NewService("test")
	service.ArchiveURL = server.URL
	service.WorkDir = workDir
	if _, err := service.Install(context.Background(), []string{"alpha"}, ScopeProject, false, false); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(workDir, ".agents", "skills", "alpha")
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatal(err)
	}

	updated, err := service.Update(context.Background(), nil, ScopeProject, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Skills) != 1 || updated.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected update result: %+v", updated.Skills)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("missing managed skill was not restored: %v", err)
	}
}

func TestRemoveCleansMissingManagedSkillFromManifest(t *testing.T) {
	archive := testArchive(t, map[string]string{
		"repo/skills/alpha/SKILL.md": skillMarkdown("alpha", "Alpha skill"),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(archive)
	}))
	defer server.Close()

	workDir := t.TempDir()
	service := NewService("test")
	service.ArchiveURL = server.URL
	service.WorkDir = workDir
	if _, err := service.Install(context.Background(), []string{"alpha"}, ScopeProject, false, false); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(workDir, ".agents", "skills")
	if err := os.RemoveAll(filepath.Join(root, "alpha")); err != nil {
		t.Fatal(err)
	}

	removed, err := service.Remove(nil, ScopeProject, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Skills) != 1 || removed.Skills[0].Name != "alpha" {
		t.Fatalf("unexpected remove result: %+v", removed.Skills)
	}
	tracked, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracked.Skills) != 0 {
		t.Fatalf("stale manifest entries remain: %+v", tracked.Skills)
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func skillMarkdown(name, description string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", name, description, name)
}
