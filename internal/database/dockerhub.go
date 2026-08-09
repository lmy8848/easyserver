package database

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// dockerHubTagPage mirrors the paginated tag list from hub.docker.com. Only the
// fields the picker needs are kept.
type dockerHubTagPage struct {
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

// fetchDockerHubTags lists published tags for an official Docker Hub image
// (library/xxx). The front-end "更多版本" flow calls this so users can install
// any published tag, not just the curated presets. Tags that don't look like a
// version (e.g. latest, oraclelinux9) are dropped.
func fetchDockerHubTags(image string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("https://hub.docker.com/v2/repositories/library/%s/tags?page_size=100", image), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "easyserver")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("查询 Docker Hub 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Docker Hub 返回 %s", resp.Status)
	}
	var page dockerHubTagPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("解析 Docker Hub 响应失败: %w", err)
	}
	seen := make(map[string]bool, len(page.Results))
	tags := make([]string, 0, len(page.Results))
	for _, r := range page.Results {
		if r.Name == "" || seen[r.Name] || !versionLike(r.Name) {
			continue
		}
		seen[r.Name] = true
		tags = append(tags, r.Name)
	}
	return tags, nil
}

// versionLike reports whether a tag looks like a version number the picker
// should offer. Plain version tags (8.4, 8.4.11) and alpine variants
// (7-alpine, used by Redis) are kept; platform-specific tags such as
// "8.4-oraclelinux9" or "8.4-bullseye" are noise and dropped.
func versionLike(tag string) bool {
	if tag == "" {
		return false
	}
	if !unicode.IsDigit([]rune(tag)[0]) {
		return false
	}
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		return strings.HasSuffix(tag, "-alpine")
	}
	return true
}
