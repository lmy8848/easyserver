package packagemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"easyserver/internal/infra/executor"
)

type Service struct {
	repo     Repository
	executor executor.CommandExecutor
}

func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	return &Service{repo: repo, executor: exec}
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
// This is a no-op since the repository handles its own schema.
func (s *Service) InitTables(_ context.Context) error {
	return nil
}

// ListPackages returns all packages for a runtime
func (s *Service) ListPackages(ctx context.Context, runtimeID int64) ([]Package, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.List(ctx, runtimeID)
}

// ScanPackages scans installed packages for a runtime
func (s *Service) ScanPackages(ctx context.Context, runtimeID int64, runtimeName, runtimePath string) ([]Package, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var packages []Package
	var err error

	switch runtimeName {
	case "node":
		packages, err = s.scanNpmPackages(ctx, runtimeID, runtimePath)
	case "python":
		packages, err = s.scanPipPackages(ctx, runtimeID, runtimePath)
	case "java":
		packages, err = s.scanMavenPackages(ctx, runtimeID, runtimePath)
	case "php":
		packages, err = s.scanComposerPackages(ctx, runtimeID, runtimePath)
	default:
		return []Package{}, nil
	}

	if err != nil {
		return nil, err
	}

	// Save to database
	for _, pkg := range packages {
		s.repo.Upsert(ctx, pkg.RuntimeID, pkg.Name, pkg.Version, pkg.Scope, pkg.Source)
	}

	return packages, nil
}

// InstallPackage installs a package
func (s *Service) InstallPackage(ctx context.Context, req *PackageInstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.installNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.installPipPackage(ctx, req, runtimePath)
	case "java":
		return s.installMavenPackage(ctx, req, runtimePath)
	case "php":
		return s.installComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UninstallPackage uninstalls a package
func (s *Service) UninstallPackage(ctx context.Context, req *PackageUninstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.uninstallNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.uninstallPipPackage(ctx, req, runtimePath)
	case "java":
		return s.uninstallMavenPackage(ctx, req, runtimePath)
	case "php":
		return s.uninstallComposerPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UpdatePackage updates a package
func (s *Service) UpdatePackage(ctx context.Context, req *PackageUpdateRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.updateNpmPackage(ctx, req, runtimePath)
	case "python":
		return s.updatePipPackage(ctx, req, runtimePath)
	default:
		return fmt.Errorf("package update not supported for %s", runtimeName)
	}
}

// SearchPackages searches for available packages
func (s *Service) SearchPackages(ctx context.Context, runtimeName, query string) ([]PackageInfo, error) {
	switch runtimeName {
	case "node":
		return s.searchNpmPackages(ctx, query)
	case "python":
		return s.searchPipPackages(ctx, query)
	default:
		return []PackageInfo{}, nil
	}
}

// GetPackageVersions returns available versions for a package
func (s *Service) GetPackageVersions(ctx context.Context, runtimeName, packageName string) ([]string, error) {
	switch runtimeName {
	case "node":
		return s.getNpmPackageVersions(ctx, packageName)
	case "python":
		return s.getPipPackageVersions(ctx, packageName)
	default:
		return []string{}, nil
	}
}

// npm package search
func (s *Service) searchNpmPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	output, _, _, err := s.executor.Run(ctx, "npm", "search", query, "--json")
	if err != nil {
		log.Printf("package: npm search error: %v", err)
		return []PackageInfo{}, nil
	}

	var result []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []PackageInfo
	for _, pkg := range result {
		packages = append(packages, PackageInfo{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Source:      "npm",
		})
	}

	return packages, nil
}

// npm package versions
func (s *Service) getNpmPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	output, _, _, err := s.executor.Run(ctx, "npm", "view", packageName, "versions", "--json")
	if err != nil {
		log.Printf("package: npm view error: %v", err)
		return []string{}, nil
	}

	var versions []string
	if err := json.Unmarshal([]byte(output), &versions); err != nil {
		var single string
		if err2 := json.Unmarshal([]byte(output), &single); err2 == nil {
			versions = []string{single}
		} else {
			return nil, err
		}
	}

	if len(versions) > 20 {
		versions = versions[len(versions)-20:]
	}

	return versions, nil
}

// pip package search
func (s *Service) searchPipPackages(ctx context.Context, query string) ([]PackageInfo, error) {
	return []PackageInfo{}, fmt.Errorf("pip search not supported, use pip install <package>")
}

// pip package versions
func (s *Service) getPipPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	output, _, _, err := s.executor.Run(ctx, "pip", "index", "versions", packageName)
	if err != nil {
		log.Printf("package: pip index error: %v", err)
		return []string{}, nil
	}

	outputStr := output
	start := strings.Index(outputStr, "(")
	end := strings.Index(outputStr, ")")
	if start == -1 || end == -1 {
		return []string{}, nil
	}

	versionsStr := outputStr[start+1 : end]
	versions := strings.Split(versionsStr, ", ")

	if len(versions) > 20 {
		versions = versions[:20]
	}

	return versions, nil
}

// npm package management
func (s *Service) scanNpmPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	output, _, _, err := s.executor.Run(ctx, "npm", "list", "-g", "--json")
	if err != nil {
		log.Printf("package: npm list error: %v", err)
		return []Package{}, nil
	}

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []Package
	for name, dep := range result.Dependencies {
		packages = append(packages, Package{
			RuntimeID: runtimeID,
			Name:      name,
			Version:   dep.Version,
			Scope:     "global",
			Source:    "npm",
		})
	}

	return packages, nil
}

func (s *Service) installNpmPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	args := []string{"install", "-g"}
	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s@%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	output, _, err := s.executor.RunCombined(ctx, "npm", args...)
	if err != nil {
		return fmt.Errorf("npm install failed: %s", output)
	}

	log.Printf("package: installed %s via npm", req.Name)
	return nil
}

func (s *Service) uninstallNpmPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "npm", "uninstall", "-g", req.Name)
	if err != nil {
		return fmt.Errorf("npm uninstall failed: %s", output)
	}

	s.repo.Delete(ctx, req.RuntimeID, req.Name, "global")

	log.Printf("package: uninstalled %s via npm", req.Name)
	return nil
}

func (s *Service) updateNpmPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "npm", "update", "-g", req.Name)
	if err != nil {
		return fmt.Errorf("npm update failed: %s", output)
	}

	log.Printf("package: updated %s via npm", req.Name)
	return nil
}

// pip package management
func (s *Service) scanPipPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	output, _, _, err := s.executor.Run(ctx, "pip", "list", "--format=json")
	if err != nil {
		log.Printf("package: pip list error: %v", err)
		return []Package{}, nil
	}

	var result []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []Package
	for _, pkg := range result {
		packages = append(packages, Package{
			RuntimeID: runtimeID,
			Name:      pkg.Name,
			Version:   pkg.Version,
			Scope:     "global",
			Source:    "pip",
		})
	}

	return packages, nil
}

func (s *Service) installPipPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	args := []string{"install"}
	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s==%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	output, _, err := s.executor.RunCombined(ctx, "pip", args...)
	if err != nil {
		return fmt.Errorf("pip install failed: %s", output)
	}

	log.Printf("package: installed %s via pip", req.Name)
	return nil
}

func (s *Service) uninstallPipPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "pip", "uninstall", "-y", req.Name)
	if err != nil {
		return fmt.Errorf("pip uninstall failed: %s", output)
	}

	s.repo.Delete(ctx, req.RuntimeID, req.Name, "global")

	log.Printf("package: uninstalled %s via pip", req.Name)
	return nil
}

func (s *Service) updatePipPackage(ctx context.Context, req *PackageUpdateRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "pip", "install", "--upgrade", req.Name)
	if err != nil {
		return fmt.Errorf("pip update failed: %s", output)
	}

	log.Printf("package: updated %s via pip", req.Name)
	return nil
}

// maven package management (placeholder)
func (s *Service) scanMavenPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	return []Package{}, nil
}

func (s *Service) installMavenPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package installation not yet supported")
}

func (s *Service) uninstallMavenPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package uninstallation not yet supported")
}

// composer package management (placeholder)
func (s *Service) scanComposerPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]Package, error) {
	return []Package{}, nil
}

func (s *Service) installComposerPackage(ctx context.Context, req *PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package installation not yet supported")
}

func (s *Service) uninstallComposerPackage(ctx context.Context, req *PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package uninstallation not yet supported")
}
