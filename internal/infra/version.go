package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the build version, set via ldflags at build time.
// IMPORTANT: ldflags path is easyserver/internal/infra.Version (NOT version.Version).
// Example: go build -ldflags "-X easyserver/internal/infra.Version=v0.1.2-91-prod -X easyserver/internal/infra.BuildID=0xd3a"
var Version = "dev"

// BuildID is a short unique identifier for each build/deploy, set via ldflags.
// Example: 0xd3a. Empty if not set (dev builds).
// ldflags path: easyserver/internal/infra.BuildID
var BuildID = ""

// GoVersion is the Go toolchain version used to build the binary.
var GoVersion = runtime.Version()

// DisplayVersion returns the version string with optional build ID suffix.
// Example: "v0.1.2-83-g52565d5+0xd3a" or "v0.1.2-83-g52565d5" or "dev".
func DisplayVersion() string {
	if BuildID != "" {
		return Version + "+" + BuildID
	}
	return Version
}

// GetFullVersionString returns the formatted version information.
func GetFullVersionString() string {
	return fmt.Sprintf("EasyServer version %s (Go: %s, Platform: %s/%s)", DisplayVersion(), GoVersion, runtime.GOOS, runtime.GOARCH)
}

// UpdateInfo contains details about a remote release check.
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseTitle   string `json:"release_title"`
	HasUpdate      bool   `json:"has_update"`
	ReleaseURL     string `json:"release_url"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
}

// CheckUpdate checks GitHub for the latest release.
func CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/lmy8848/easyserver/releases/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "EasyServer-UpdateChecker")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("检查更新失败: 无法连接到更新服务器 (%w)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("检查更新失败: GitHub 返回状态码 %d", resp.StatusCode)
	}

	var ghRelease struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("解析更新数据失败: %w", err)
	}

	curVer := Version
	hasUpdate := IsNewerVersion(ghRelease.TagName, curVer)

	return &UpdateInfo{
		CurrentVersion: DisplayVersion(),
		LatestVersion:  ghRelease.TagName,
		ReleaseTitle:   ghRelease.Name,
		HasUpdate:      hasUpdate,
		ReleaseURL:     ghRelease.HTMLURL,
		ReleaseNotes:   ghRelease.Body,
		PublishedAt:    ghRelease.PublishedAt,
	}, nil
}

// IsNewerVersion compares semver tags (e.g. "v0.2.0" vs "v0.1.0").
func IsNewerVersion(latest, current string) bool {
	clean := func(v string) []int {
		v = strings.TrimPrefix(v, "v")
		if idx := strings.IndexAny(v, "+-"); idx != -1 {
			v = v[:idx]
		}
		parts := strings.Split(v, ".")
		res := make([]int, 0, len(parts))
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				break
			}
			res = append(res, n)
		}
		return res
	}

	if current == "dev" || current == "" {
		return latest != ""
	}

	latParts := clean(latest)
	curParts := clean(current)
	if len(latParts) == 0 {
		return false
	}
	if len(curParts) == 0 {
		return latest != current
	}

	maxLen := max(len(curParts), len(latParts))
	for i := range maxLen {
		latVal, curVal := 0, 0
		if i < len(latParts) {
			latVal = latParts[i]
		}
		if i < len(curParts) {
			curVal = curParts[i]
		}
		if latVal > curVal {
			return true
		}
		if latVal < curVal {
			return false
		}
	}
	return false
}
