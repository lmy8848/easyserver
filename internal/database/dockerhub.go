package database

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// dockerHubTagPage mirrors one paginated tag page from hub.docker.com. Only
// the fields the picker needs are kept.
type dockerHubTagPage struct {
	Next    string `json:"next"`
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

// fetchDockerHubTags lists every published version-like tag for an official
// Docker Hub image (library/xxx). It walks the API's pagination (page_size=100)
// until exhausted; tags that don't look like a version (e.g. latest,
// oraclelinux9) are dropped. The front-end "更多版本" flow calls this so users
// can install any published tag, not just the curated presets.
func fetchDockerHubTags(image string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	seen := make(map[string]bool)
	var tags []string
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/library/%s/tags?page_size=100", image)
	for url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "easyserver")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("查询 Docker Hub 失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Docker Hub 返回 %s", resp.Status)
		}
		var page dockerHubTagPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("解析 Docker Hub 响应失败: %w", err)
		}
		resp.Body.Close()
		for _, r := range page.Results {
			if r.Name == "" || seen[r.Name] || !versionLike(r.Name) {
				continue
			}
			seen[r.Name] = true
			tags = append(tags, r.Name)
		}
		url = page.Next
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
