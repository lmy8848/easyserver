package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type RuntimeVersionService struct {
	repo repository.RuntimeRepository
}

func NewRuntimeVersionService(repo repository.RuntimeRepository) *RuntimeVersionService {
	return &RuntimeVersionService{repo: repo}
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
func (s *RuntimeVersionService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.InitRuntimeVersionsTable(ctx)
}

// List returns all cached versions for a runtime
func (s *RuntimeVersionService) List(ctx context.Context, name string) ([]model.RuntimeVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListRuntimeVersions(ctx, name)
}

// ResolveAlias resolves a version alias to an actual version
// Supported aliases:
//   - "lts" -> latest LTS version
//   - "latest" -> latest stable version
//   - "stable" -> latest stable version
//   - "17" -> latest 17.x.x version
//   - "17.0" -> latest 17.0.x version
func (s *RuntimeVersionService) ResolveAlias(ctx context.Context, name, alias string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get all versions
	versions, err := s.List(ctx, name)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available for %s", name)
	}

	// Handle special aliases
	switch strings.ToLower(alias) {
	case "lts":
		// Find latest LTS version
		for _, v := range versions {
			if v.LTS {
				return v.Version, nil
			}
		}
		return "", fmt.Errorf("no LTS version available for %s", name)

	case "latest", "stable":
		// Find latest stable version
		for _, v := range versions {
			if v.Stable {
				return v.Version, nil
			}
		}
		// If no stable version, return latest
		return versions[0].Version, nil
	}

	// Handle major version (e.g., "17" -> "17.x.x")
	majorVersion := strings.Split(alias, ".")[0]
	for _, v := range versions {
		if strings.HasPrefix(v.Version, majorVersion+".") {
			return v.Version, nil
		}
	}

	// If no match found, return the alias as-is (might be a full version)
	return alias, nil
}

// GetAliasSuggestions returns alias suggestions for a runtime
func (s *RuntimeVersionService) GetAliasSuggestions(ctx context.Context, name string) []string {
	versions, err := s.List(ctx, name)
	if err != nil || len(versions) == 0 {
		return []string{}
	}

	suggestions := []string{"latest"}

	// Add LTS if available
	for _, v := range versions {
		if v.LTS {
			suggestions = append(suggestions, "lts")
			break
		}
	}

	// Add major versions
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
func (s *RuntimeVersionService) ListWithInstalledStatus(ctx context.Context, name string) ([]model.RuntimeVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, err := s.repo.ListRuntimeVersions(ctx, name)
	if err != nil {
		return nil, err
	}

	// Get installed environments for this runtime
	envs, err := s.repo.ListByName(ctx, name)
	if err != nil {
		// If error, just return versions without installed status
		return versions, nil
	}

	// Match installed environments with versions
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
func (s *RuntimeVersionService) FetchAndCache(ctx context.Context, name string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var versions []model.RuntimeVersion
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

	// Cache versions
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

// fetchJavaVersions fetches Java versions from Adoptium API
func fetchJavaVersions() ([]model.RuntimeVersion, error) {
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

	var versions []model.RuntimeVersion
	for _, v := range result.AvailableReleases {
		versions = append(versions, model.RuntimeVersion{
			Name:    "java",
			Version: fmt.Sprintf("%d", v),
			LTS:     ltsSet[v],
			Stable:  true,
		})
	}

	return versions, nil
}

// fetchNodeVersions fetches Node.js versions from official API
func fetchNodeVersions() ([]model.RuntimeVersion, error) {
	resp, err := http.Get("https://nodejs.org/dist/index.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Use json.RawMessage to handle mixed types for lts field
	var rawReleases []struct {
		Version string          `json:"version"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(body, &rawReleases); err != nil {
		return nil, err
	}

	// Deduplicate major.minor versions
	seen := make(map[string]bool)
	var versions []model.RuntimeVersion
	for _, r := range rawReleases {
		// Parse lts field - can be string or bool
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

		versions = append(versions, model.RuntimeVersion{
			Name:    "node",
			Version: v,
			LTS:     isLTS,
			Stable:  true,
		})
	}

	return versions, nil
}

// fetchGoVersions fetches Go versions from official API
func fetchGoVersions() ([]model.RuntimeVersion, error) {
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

	var versions []model.RuntimeVersion
	for _, r := range releases {
		v := strings.TrimPrefix(r.Version, "go")
		versions = append(versions, model.RuntimeVersion{
			Name:    "go",
			Version: v,
			LTS:     false,
			Stable:  r.Stable,
		})
	}

	return versions, nil
}

// fetchPythonVersions fetches Python versions from endoflife.date API
func fetchPythonVersions() ([]model.RuntimeVersion, error) {
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

	var versions []model.RuntimeVersion
	for _, r := range releases {
		// Check if still in support
		isSupported := r.Support != "" && r.EOL != ""
		versions = append(versions, model.RuntimeVersion{
			Name:    "python",
			Version: r.Latest,
			LTS:     false,
			Stable:  isSupported,
		})
	}

	return versions, nil
}

// fetchPHPVersions fetches PHP versions from endoflife.date API
func fetchPHPVersions() ([]model.RuntimeVersion, error) {
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

	var versions []model.RuntimeVersion
	for _, r := range releases {
		isSupported := r.Support != "" && r.EOL != ""
		versions = append(versions, model.RuntimeVersion{
			Name:    "php",
			Version: r.Latest,
			LTS:     false,
			Stable:  isSupported,
		})
	}

	return versions, nil
}
