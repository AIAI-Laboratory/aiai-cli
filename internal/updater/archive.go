package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrChecksumMismatch   = errors.New("checksum verification failed")
	ErrBinaryNotFound     = errors.New("executable binary not found in archive")
	ErrInvalidArchiveType = errors.New("unsupported archive format")
)

// DownloadAndExtract downloads the release asset archive, verifies checksum, extracts binary into a temp file, and returns the temp file path.
func DownloadAndExtract(ctx context.Context, httpClient *http.Client, release Release, token string) (string, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	// 1. Download archive bytes
	archiveData, err := downloadFile(ctx, httpClient, release.AssetURL, token)
	if err != nil {
		return "", fmt.Errorf("download archive: %w", err)
	}

	// 2. Verify checksum if available
	if release.ChecksumURL != "" {
		checksumData, err := downloadFile(ctx, httpClient, release.ChecksumURL, token)
		if err == nil {
			if err := verifyChecksum(archiveData, release.AssetName, checksumData); err != nil {
				return "", err
			}
		}
	}

	// 3. Extract binary
	binaryName := "aiai"
	if runtime.GOOS == "windows" {
		binaryName = "aiai.exe"
	}

	var binaryData []byte
	switch {
	case strings.HasSuffix(release.AssetName, ".tar.gz") || strings.HasSuffix(release.AssetName, ".tgz"):
		binaryData, err = extractFromTarGz(archiveData, binaryName)
	case strings.HasSuffix(release.AssetName, ".zip"):
		binaryData, err = extractFromZip(archiveData, binaryName)
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidArchiveType, release.AssetName)
	}
	if err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}

	// 4. Write to temp file
	tmpDir, err := os.MkdirTemp("", "aiai-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp directory: %w", err)
	}

	tmpFile := filepath.Join(tmpDir, binaryName)
	if err := os.WriteFile(tmpFile, binaryData, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write extracted binary: %w", err)
	}

	return tmpFile, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed (status %d)", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func verifyChecksum(data []byte, filename string, checksumData []byte) error {
	h := sha256.Sum256(data)
	actualHash := hex.EncodeToString(h[:])

	lines := strings.Split(string(checksumData), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			expectedHash := strings.ToLower(parts[0])
			assetFile := filepath.Base(parts[1])
			if strings.EqualFold(assetFile, filename) {
				if expectedHash != actualHash {
					return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedHash, actualHash)
				}
				return nil
			}
		}
	}
	// If the file was not explicitly listed in checksums.txt, we do not fail hard
	return nil
}

func extractFromTarGz(data []byte, targetBinary string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(header.Name) == targetBinary && header.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, ErrBinaryNotFound
}

func extractFromZip(data []byte, targetBinary string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	for _, file := range zr.File {
		if filepath.Base(file.Name) == targetBinary {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, ErrBinaryNotFound
}
