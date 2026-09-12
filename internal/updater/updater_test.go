package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.1.0", "v0.1.1", true},
		{"v0.1.0", "v0.2.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.2.0", "v1.1.0", false},
		{"v0.2.0-rc1", "v0.2.0", true},
		{"dev", "v0.1.0", false}, // dev builds don't auto-upgrade
		{"", "v0.1.0", false},
		{"v0.1.0", "invalid", false},
	}

	for _, tt := range tests {
		got := IsNewerVersion(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v; want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestStateStore(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	store, err := NewFileStateStore(stateFile)
	if err != nil {
		t.Fatalf("NewFileStateStore: %v", err)
	}

	// 1. Initial state should be empty
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load empty state: %v", err)
	}
	if state.LatestVersion != "" {
		t.Errorf("expected empty LatestVersion, got %s", state.LatestVersion)
	}

	// 2. Save state
	now := time.Now().Truncate(time.Second)
	remind := now.Add(24 * time.Hour)
	err = store.Save(State{
		LastCheckedAt:  now,
		LatestVersion:  "0.2.0",
		SkippedVersion: "0.1.5",
		RemindAfter:    remind,
	})
	if err != nil {
		t.Fatalf("Save state: %v", err)
	}

	// 3. Reload state
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load saved state: %v", err)
	}
	if loaded.LatestVersion != "0.2.0" {
		t.Errorf("LatestVersion = %q, want 0.2.0", loaded.LatestVersion)
	}
	if loaded.SkippedVersion != "0.1.5" {
		t.Errorf("SkippedVersion = %q, want 0.1.5", loaded.SkippedVersion)
	}
	if !loaded.RemindAfter.Equal(remind) {
		t.Errorf("RemindAfter = %v, want %v", loaded.RemindAfter, remind)
	}
}

func TestGitHubClientFetchLatestRelease(t *testing.T) {
	targetExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		targetExt = ".zip"
	}
	assetName := fmt.Sprintf("aiai_0.2.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, targetExt)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}

		responseJSON := fmt.Sprintf(`{
			"tag_name": "v0.2.0",
			"name": "Release v0.2.0",
			"body": "Awesome release notes",
			"published_at": "2026-09-12T12:00:00Z",
			"html_url": "https://github.com/test/repo/releases/tag/v0.2.0",
			"assets": [
				{
					"name": "%s",
					"browser_download_url": "http://download.server/%s"
				},
				{
					"name": "checksums.txt",
					"browser_download_url": "http://download.server/checksums.txt"
				}
			]
		}`, assetName, assetName)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	client := NewGitHubClient("test/repo", "dummy-token", "test-agent")
	client.BaseURL = server.URL

	release, err := client.FetchLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("FetchLatestRelease failed: %v", err)
	}

	if release.TagName != "v0.2.0" {
		t.Errorf("TagName = %q, want v0.2.0", release.TagName)
	}
	if release.Version != "0.2.0" {
		t.Errorf("Version = %q, want 0.2.0", release.Version)
	}
	if release.AssetName != assetName {
		t.Errorf("AssetName = %q, want %s", release.AssetName, assetName)
	}
	if release.ChecksumURL != "http://download.server/checksums.txt" {
		t.Errorf("ChecksumURL = %q", release.ChecksumURL)
	}
}

func TestArchiveExtractionTarGz(t *testing.T) {
	binaryName := "aiai"
	expectedContent := []byte("binary executable content")

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name: binaryName,
		Mode: 0o755,
		Size: int64(len(expectedContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(expectedContent); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gzw.Close()

	archiveBytes := buf.Bytes()
	h := sha256.Sum256(archiveBytes)
	hashHex := hex.EncodeToString(h[:])
	checksumData := []byte(fmt.Sprintf("%s  aiai_0.2.0_test.tar.gz\n", hashHex))

	// Verify checksum
	if err := verifyChecksum(archiveBytes, "aiai_0.2.0_test.tar.gz", checksumData); err != nil {
		t.Fatalf("verifyChecksum failed: %v", err)
	}

	// Extract
	extracted, err := extractFromTarGz(archiveBytes, binaryName)
	if err != nil {
		t.Fatalf("extractFromTarGz failed: %v", err)
	}

	if !bytes.Equal(extracted, expectedContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", string(extracted), string(expectedContent))
	}
}

func TestArchiveExtractionZip(t *testing.T) {
	binaryName := "aiai.exe"
	expectedContent := []byte("binary executable content for windows")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create(binaryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(expectedContent); err != nil {
		t.Fatal(err)
	}
	zw.Close()

	archiveBytes := buf.Bytes()
	extracted, err := extractFromZip(archiveBytes, binaryName)
	if err != nil {
		t.Fatalf("extractFromZip failed: %v", err)
	}

	if !bytes.Equal(extracted, expectedContent) {
		t.Errorf("extracted content mismatch: got %q, want %q", string(extracted), string(expectedContent))
	}
}

func TestDefaultServiceCheck(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	targetExt := ".tar.gz"
	if runtime.GOOS == "windows" {
		targetExt = ".zip"
	}
	assetName := fmt.Sprintf("aiai_0.2.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, targetExt)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJSON := fmt.Sprintf(`{
			"tag_name": "v0.2.0",
			"name": "v0.2.0",
			"body": "Release description",
			"assets": [{"name": "%s", "browser_download_url": "http://test/%s"}]
		}`, assetName, assetName)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	service, err := NewService("test/repo", "", "test", stateFile)
	if err != nil {
		t.Fatal(err)
	}
	service.GitHub.BaseURL = server.URL

	// 1. Current version v0.1.0 -> should have update
	res, err := service.CheckForUpdate(context.Background(), "v0.1.0", false)
	if err != nil {
		t.Fatalf("CheckForUpdate error: %v", err)
	}
	if !res.HasUpdate {
		t.Errorf("expected HasUpdate=true for v0.1.0 vs v0.2.0")
	}

	// 2. Postpone by skipping version
	err = service.Postpone(*res.LatestRelease, true)
	if err != nil {
		t.Fatalf("Postpone error: %v", err)
	}

	// 3. Check again without force -> should not have update because it was skipped
	res2, err := service.CheckForUpdate(context.Background(), "v0.1.0", false)
	if err != nil {
		t.Fatalf("CheckForUpdate error: %v", err)
	}
	if res2.HasUpdate {
		t.Errorf("expected HasUpdate=false for skipped version")
	}

	// 4. Check again WITH force -> should have update
	res3, err := service.CheckForUpdate(context.Background(), "v0.1.0", true)
	if err != nil {
		t.Fatalf("CheckForUpdate with force error: %v", err)
	}
	if !res3.HasUpdate {
		t.Errorf("expected HasUpdate=true with force=true")
	}
}
