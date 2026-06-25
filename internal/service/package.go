package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

type PackageManagerService struct {
	db       *sql.DB
	executor executor.CommandExecutor
}

func NewPackageManagerService(db *sql.DB, exec executor.CommandExecutor) *PackageManagerService {
	return &PackageManagerService{db: db, executor: exec}
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
func (s *PackageManagerService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	queries := []string{
		`CREATE TABLE IF NOT EXISTS packages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			runtime_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			scope TEXT DEFAULT 'global',
			source TEXT NOT NULL,
			installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(runtime_id, name, scope)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_packages_runtime ON packages(runtime_id)`,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	return nil
}

// ListPackages returns all packages for a runtime
func (s *PackageManagerService) ListPackages(ctx context.Context, runtimeID int64) ([]model.Package, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.runtime_id, r.name, p.name, p.version, p.scope, p.source, p.installed_at
		FROM packages p
		LEFT JOIN runtime_environments r ON p.runtime_id = r.id
		WHERE p.runtime_id = ?
		ORDER BY p.name
	`, runtimeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var packages []model.Package
	for rows.Next() {
		var pkg model.Package
		err := rows.Scan(&pkg.ID, &pkg.RuntimeID, &pkg.RuntimeName, &pkg.Name, &pkg.Version, &pkg.Scope, &pkg.Source, &pkg.InstalledAt)
		if err != nil {
			continue
		}
		packages = append(packages, pkg)
	}

	return packages, nil
}

// ScanPackages scans installed packages for a runtime
func (s *PackageManagerService) ScanPackages(ctx context.Context, runtimeID int64, runtimeName, runtimePath string) ([]model.Package, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var packages []model.Package
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
		return []model.Package{}, nil
	}

	if err != nil {
		return nil, err
	}

	// Save to database
	for _, pkg := range packages {
		s.db.ExecContext(ctx,
			"INSERT OR REPLACE INTO packages (runtime_id, name, version, scope, source) VALUES (?, ?, ?, ?, ?)",
			pkg.RuntimeID, pkg.Name, pkg.Version, pkg.Scope, pkg.Source,
		)
	}

	return packages, nil
}

// InstallPackage installs a package
func (s *PackageManagerService) InstallPackage(ctx context.Context, req *model.PackageInstallRequest, runtimeName, runtimePath string) error {
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
func (s *PackageManagerService) UninstallPackage(ctx context.Context, req *model.PackageUninstallRequest, runtimeName, runtimePath string) error {
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
func (s *PackageManagerService) UpdatePackage(ctx context.Context, req *model.PackageUpdateRequest, runtimeName, runtimePath string) error {
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
func (s *PackageManagerService) SearchPackages(ctx context.Context, runtimeName, query string) ([]model.PackageInfo, error) {
	switch runtimeName {
	case "node":
		return s.searchNpmPackages(ctx, query)
	case "python":
		return s.searchPipPackages(ctx, query)
	default:
		return []model.PackageInfo{}, nil
	}
}

// GetPackageVersions returns available versions for a package
func (s *PackageManagerService) GetPackageVersions(ctx context.Context, runtimeName, packageName string) ([]string, error) {
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
func (s *PackageManagerService) searchNpmPackages(ctx context.Context, query string) ([]model.PackageInfo, error) {
	// npm search --json
	output, _, _, err := s.executor.Run(ctx, "npm", "search", query, "--json")
	if err != nil {
		log.Printf("package: npm search error: %v", err)
		return []model.PackageInfo{}, nil
	}

	// Parse JSON output
	var result []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []model.PackageInfo
	for _, pkg := range result {
		packages = append(packages, model.PackageInfo{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Source:      "npm",
		})
	}

	return packages, nil
}

// npm package versions
func (s *PackageManagerService) getNpmPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	// npm view <package> versions --json
	output, _, _, err := s.executor.Run(ctx, "npm", "view", packageName, "versions", "--json")
	if err != nil {
		log.Printf("package: npm view error: %v", err)
		return []string{}, nil
	}

	// Parse JSON output (array of versions)
	var versions []string
	if err := json.Unmarshal([]byte(output), &versions); err != nil {
		// Try single version
		var single string
		if err2 := json.Unmarshal([]byte(output), &single); err2 == nil {
			versions = []string{single}
		} else {
			return nil, err
		}
	}

	// Return last 20 versions (most recent)
	if len(versions) > 20 {
		versions = versions[len(versions)-20:]
	}

	return versions, nil
}

// pip package search
func (s *PackageManagerService) searchPipPackages(ctx context.Context, query string) ([]model.PackageInfo, error) {
	// pip search is deprecated, use pip index versions
	// For now, return a placeholder
	return []model.PackageInfo{}, fmt.Errorf("pip search not supported, use pip install <package>")
}

// pip package versions
func (s *PackageManagerService) getPipPackageVersions(ctx context.Context, packageName string) ([]string, error) {
	// pip index versions <package>
	output, _, _, err := s.executor.Run(ctx, "pip", "index", "versions", packageName)
	if err != nil {
		log.Printf("package: pip index error: %v", err)
		return []string{}, nil
	}

	// Parse output: "package_name (X.Y.Z, X.Y.Z, ...)"
	outputStr := output
	start := strings.Index(outputStr, "(")
	end := strings.Index(outputStr, ")")
	if start == -1 || end == -1 {
		return []string{}, nil
	}

	versionsStr := outputStr[start+1 : end]
	versions := strings.Split(versionsStr, ", ")

	// Return last 20 versions
	if len(versions) > 20 {
		versions = versions[:20]
	}

	return versions, nil
}

// npm package management
func (s *PackageManagerService) scanNpmPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]model.Package, error) {
	// npm list -g --json
	output, _, _, err := s.executor.Run(ctx, "npm", "list", "-g", "--json")
	if err != nil {
		log.Printf("package: npm list error: %v", err)
		return []model.Package{}, nil
	}

	// Parse JSON output
	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []model.Package
	for name, dep := range result.Dependencies {
		packages = append(packages, model.Package{
			RuntimeID: runtimeID,
			Name:      name,
			Version:   dep.Version,
			Scope:     "global",
			Source:    "npm",
		})
	}

	return packages, nil
}

func (s *PackageManagerService) installNpmPackage(ctx context.Context, req *model.PackageInstallRequest, runtimePath string) error {
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

func (s *PackageManagerService) uninstallNpmPackage(ctx context.Context, req *model.PackageUninstallRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "npm", "uninstall", "-g", req.Name)
	if err != nil {
		return fmt.Errorf("npm uninstall failed: %s", output)
	}

	// Remove from database
	s.db.Exec("DELETE FROM packages WHERE runtime_id = ? AND name = ? AND scope = 'global'", req.RuntimeID, req.Name)

	log.Printf("package: uninstalled %s via npm", req.Name)
	return nil
}

func (s *PackageManagerService) updateNpmPackage(ctx context.Context, req *model.PackageUpdateRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "npm", "update", "-g", req.Name)
	if err != nil {
		return fmt.Errorf("npm update failed: %s", output)
	}

	log.Printf("package: updated %s via npm", req.Name)
	return nil
}

// pip package management
func (s *PackageManagerService) scanPipPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]model.Package, error) {
	// pip list --format=json
	output, _, _, err := s.executor.Run(ctx, "pip", "list", "--format=json")
	if err != nil {
		log.Printf("package: pip list error: %v", err)
		return []model.Package{}, nil
	}

	// Parse JSON output
	var result []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	var packages []model.Package
	for _, pkg := range result {
		packages = append(packages, model.Package{
			RuntimeID: runtimeID,
			Name:      pkg.Name,
			Version:   pkg.Version,
			Scope:     "global",
			Source:    "pip",
		})
	}

	return packages, nil
}

func (s *PackageManagerService) installPipPackage(ctx context.Context, req *model.PackageInstallRequest, runtimePath string) error {
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

func (s *PackageManagerService) uninstallPipPackage(ctx context.Context, req *model.PackageUninstallRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "pip", "uninstall", "-y", req.Name)
	if err != nil {
		return fmt.Errorf("pip uninstall failed: %s", output)
	}

	// Remove from database
	s.db.Exec("DELETE FROM packages WHERE runtime_id = ? AND name = ? AND scope = 'global'", req.RuntimeID, req.Name)

	log.Printf("package: uninstalled %s via pip", req.Name)
	return nil
}

func (s *PackageManagerService) updatePipPackage(ctx context.Context, req *model.PackageUpdateRequest, runtimePath string) error {
	output, _, err := s.executor.RunCombined(ctx, "pip", "install", "--upgrade", req.Name)
	if err != nil {
		return fmt.Errorf("pip update failed: %s", output)
	}

	log.Printf("package: updated %s via pip", req.Name)
	return nil
}

// maven package management (placeholder)
func (s *PackageManagerService) scanMavenPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]model.Package, error) {
	// Maven doesn't have a simple list command like npm/pip
	// Would need to parse pom.xml or ~/.m2/repository
	return []model.Package{}, nil
}

func (s *PackageManagerService) installMavenPackage(ctx context.Context, req *model.PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package installation not yet supported")
}

func (s *PackageManagerService) uninstallMavenPackage(ctx context.Context, req *model.PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package uninstallation not yet supported")
}

// composer package management (placeholder)
func (s *PackageManagerService) scanComposerPackages(ctx context.Context, runtimeID int64, runtimePath string) ([]model.Package, error) {
	// composer show -i --format=json
	return []model.Package{}, nil
}

func (s *PackageManagerService) installComposerPackage(ctx context.Context, req *model.PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package installation not yet supported")
}

func (s *PackageManagerService) uninstallComposerPackage(ctx context.Context, req *model.PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package uninstallation not yet supported")
}
