package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"beacon/internal/version"
)

const (
	githubRepo = "Bajusz15/beacon"
)

var releaseURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ReleaseInfo struct {
	Tag        string
	CurrentVer string
	AssetName  string
	IsNewer    bool
}

func binaryAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	if os == "linux" && arch == "arm" {
		return "beacon-linux_arm"
	}
	return fmt.Sprintf("beacon-%s_%s", os, arch)
}

func CheckLatest(ctx context.Context) (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found for %s", githubRepo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	current := version.GetVersion()
	tag := strings.TrimPrefix(rel.TagName, "v")
	currentClean := strings.TrimPrefix(current, "v")

	return &ReleaseInfo{
		Tag:        rel.TagName,
		CurrentVer: current,
		AssetName:  binaryAssetName(),
		IsNewer:    current != "dev" && isNewerVersion(currentClean, tag),
	}, nil
}

func DownloadAndReplace(ctx context.Context) error {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return err
	}

	assetName := binaryAssetName()
	var binaryURL, checksumURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			binaryURL = a.BrowserDownloadURL
		}
		if a.Name == "SHA256SUMS.txt" {
			checksumURL = a.BrowserDownloadURL
		}
	}
	if binaryURL == "" {
		available := make([]string, 0, len(rel.Assets))
		for _, a := range rel.Assets {
			available = append(available, a.Name)
		}
		return fmt.Errorf("no asset %q in release %s (available: %s)", assetName, rel.TagName, strings.Join(available, ", "))
	}

	expectedHash := ""
	if checksumURL != "" {
		h, err := fetchExpectedHash(ctx, checksumURL, assetName)
		if err != nil {
			return fmt.Errorf("fetch checksums: %w", err)
		}
		expectedHash = h
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine binary path: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".beacon-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	dlClient := &http.Client{Timeout: 5 * time.Minute}
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, binaryURL, nil)
	if err != nil {
		_ = tmpFile.Close()
		return err
	}
	dlResp, err := dlClient.Do(dlReq)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download binary: %w", err)
	}
	defer func() { _ = dlResp.Body.Close() }()
	if dlResp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download returned HTTP %d", dlResp.StatusCode)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hasher), dlResp.Body)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if written == 0 {
		return fmt.Errorf("downloaded file is empty")
	}

	gotHash := hex.EncodeToString(hasher.Sum(nil))
	if expectedHash != "" && gotHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, gotHash)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Atomic replace: rename over the existing binary. This works on Linux and
	// macOS even while the binary is running (the kernel keeps the old inode
	// open until the process exits).
	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("replace binary: %w (try running with sudo)", err)
	}
	return nil
}

func fetchExpectedHash(ctx context.Context, url, assetName string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %q in SHA256SUMS.txt", assetName)
}

// isNewerVersion returns true if remote is a higher semver than current.
// Pre-release suffixes (e.g. "-beta") are stripped for numeric comparison,
// then a release (no suffix) is considered newer than a pre-release of the
// same numeric version (0.3.1 > 0.3.1-beta).
func isNewerVersion(current, remote string) bool {
	curBase, curPre := splitPrerelease(current)
	remBase, remPre := splitPrerelease(remote)

	curParts := parseVersionParts(curBase)
	remParts := parseVersionParts(remBase)

	for i := 0; i < len(curParts) || i < len(remParts); i++ {
		c, r := 0, 0
		if i < len(curParts) {
			c = curParts[i]
		}
		if i < len(remParts) {
			r = remParts[i]
		}
		if r > c {
			return true
		}
		if r < c {
			return false
		}
	}
	// Same numeric version: release beats pre-release.
	return curPre != "" && remPre == ""
}

func splitPrerelease(v string) (base, pre string) {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func parseVersionParts(v string) []int {
	var parts []int
	for _, s := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(s)
		parts = append(parts, n)
	}
	return parts
}
