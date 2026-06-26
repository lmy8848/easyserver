package runtimeenv

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// VersionService handles runtime version fetching, caching, and alias resolution.
type VersionService struct {
	repo Repository
}

func NewVersionService(repo Repository) *VersionService {
	return &VersionService{repo: repo}
}

// Deprecated: InitTables is kept for backward compatibility only.
func (s *VersionService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.InitRuntimeVersionsTable(ctx)
}

// List returns all cached versions for a runtime
func (s *VersionService) List(ctx context.Context, name string) ([]RuntimeVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListRuntimeVersions(ctx, name)
}

// ResolveAlias resolves a version alias to an actual version
func (s *VersionService) ResolveAlias(ctx context.Context, name, alias string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, err := s.List(ctx, name)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available for %s", name)
	}

	switch strings.ToLower(alias) {
	case "lts":
		for _, v := range versions {
			if v.LTS {
				return v.Version, nil
			}
		}
		return "", fmt.Errorf("no LTS version available for %s", name)
	case "latest", "stable":
		for _, v := range versions {
			if v.Stable {
				return v.Version, nil
			}
		}
		return versions[0].Version, nil
	}

	majorVersion := strings.Split(alias, ".")[0]
	for _, v := range versions {
		if strings.HasPrefix(v.Version, majorVersion+".") {
			return v.Version, nil
		}
	}

	return alias, nil
}

// GetAliasSuggestions returns alias suggestions for a runtime
func (s *VersionService) GetAliasSuggestions(ctx context.Context, name string) []string {
	versions, err := s.List(ctx, name)
	if err != nil || len(versions) == 0 {
		return []string{}
	}

	suggestions := []string{"latest"}
	for _, v := range versions {
		if v.LTS {
			suggestions = append(suggestions, "lts")
			break
		}
	}

	seen := make(map[string]bool)
	for _, v := range versions {
		parts := strings.Split(v.Version, ".")
		if len(parts) > 0 && !seen[parts[0]] {
			seen[parts[0]] = true
			suggestions = append(suggestions, parts[0])
		}
	}

	return suggestions
}

// ListWithInstalledStatus returns all cached versions with installed status
func (s *VersionService) ListWithInstalledStatus(ctx context.Context, name string) ([]RuntimeVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, err := s.repo.ListRuntimeVersions(ctx, name)
	if err != nil {
		return nil, err
	}

	envs, err := s.repo.ListByName(ctx, name)
	if err != nil {
		return versions, nil
	}

	for i, v := range versions {
		for _, env := range envs {
			if strings.HasPrefix(env.Version, v.Version) || env.Version == v.Version {
				versions[i].Installed = true
				versions[i].IsDefault = env.IsDefault
				break
			}
		}
	}

	return versions, nil
}

// FetchAndCache fetches versions from external sources and caches them
func (s *VersionService) FetchAndCache(ctx context.Context, name string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var versions []RuntimeVersion
	var err error

	switch name {
	case "java":
		versions, err = fetchJavaVersions()
	case "node":
		versions, err = fetchNodeVersions()
	case "go":
		versions, err = fetchGoVersions()
	case "python":
		versions, err = fetchPythonVersions()
	case "php":
		versions, err = fetchPHPVersions()
	default:
		return 0, fmt.Errorf("unsupported runtime: %s", name)
	}

	if err != nil {
		return 0, err
	}

	cached := 0
	for _, v := range versions {
		if err := s.repo.UpsertRuntimeVersion(ctx, name, v.Version, v.LTS, v.Stable); err != nil {
			log.Printf("runtime_version: failed to cache %s %s: %v", name, v.Version, err)
			continue
		}
		cached++
	}

	return cached, nil
}

func fetchJavaVersions() ([]RuntimeVersion, error) {
	resp, err := http.Get("https://api.adoptium.net/v3/info/available_releases")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		AvailableLTSReleases []int `json:"available_lts_releases"`
		AvailableReleases    []int `json:"available_releases"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	ltsSet := make(map[int]bool)
	for _, v := range result.AvailableLTSReleases {
		ltsSet[v] = true
	}

	var versions []RuntimeVersion
	for _, v := range result.AvailableReleases {
		versions = append(versions, RuntimeVersion{
			Name:    "java",
			Version: fmt.Sprintf("%d", v),
			LTS:     ltsSet[v],
			Stable:  true,
		})
	}

	return versions, nil
}

func fetchNodeVersions() ([]RuntimeVersion, error) {
	resp, err := http.Get("https://nodejs.org/dist/index.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawReleases []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(body, &rawReleases); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var versions []RuntimeVersion
	for _, r := range rawReleases {
		isLTS := false
		var ltsStr string
		if err := json.Unmarshal(r.LTS, &ltsStr); err == nil {
			isLTS = ltsStr != "" && ltsStr != "false"
		}
		v := strings.TrimPrefix(r.Version, "v")
		parts := strings.Split(v, ".")
		if len(parts) >= 2 {
			key := parts[0] + "." + parts[1]
			if seen[key] {
				continue
			}
			seen[key] = true
		}

		versions = append(versions, RuntimeVersion{
			Name:    "node",
			Version: v,
			LTS:     isLTS,
			Stable:  true,
		})
	}

	return versions, nil
}

func fetchGoVersions() ([]RuntimeVersion, error) {
	resp, err := http.Get("https://go.dev/dl/?mode=json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}

	var versions []RuntimeVersion
	for _, r := range releases {
		v := strings.TrimPrefix(r.Version, "go")
		versions = append(versions, RuntimeVersion{
			Name:    "go",
			Version: v,
			LTS:     false,
			Stable:  r.Stable,
		})
	}

	return versions, nil
}

func fetchPythonVersions() ([]RuntimeVersion, error) {
	resp, err := http.Get("https://endoflife.date/api/python.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []struct {
		Cycle   string `json:"cycle"`
		Support string `json:"support"`
		EOL     string `json:"eol"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}

	var versions []RuntimeVersion
	for _, r := range releases {
		isSupported := r.Support != "" && r.EOL != ""
		versions = append(versions, RuntimeVersion{
			Name:    "python",
			Version: r.Latest,
			LTS:     false,
			Stable:  isSupported,
		})
	}

	return versions, nil
}

func fetchPHPVersions() ([]RuntimeVersion, error) {
	resp, err := http.Get("https://endoflife.date/api/php.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []struct {
		Cycle   string `json:"cycle"`
		Support string `json:"support"`
		EOL     string `json:"eol"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}

	var versions []RuntimeVersion
	for _, r := range releases {
		isSupported := r.Support != "" && r.EOL != ""
		versions = append(versions, RuntimeVersion{
			Name:    "php",
			Version: r.Latest,
			LTS:     false,
			Stable:  isSupported,
		})
	}

	return versions, nil
}
