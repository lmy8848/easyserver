package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"easyserver/internal/model"
)

type PackageManagerService struct {
	db *sql.DB
}

func NewPackageManagerService(db *sql.DB) *PackageManagerService {
	return &PackageManagerService{db: db}
}

// InitTables creates package tables if they don't exist
func (s *PackageManagerService) InitTables() error {
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
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

// ListPackages returns all packages for a runtime
func (s *PackageManagerService) ListPackages(runtimeID int64) ([]model.Package, error) {
	rows, err := s.db.Query(`
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
func (s *PackageManagerService) ScanPackages(runtimeID int64, runtimeName, runtimePath string) ([]model.Package, error) {
	var packages []model.Package
	var err error

	switch runtimeName {
	case "node":
		packages, err = s.scanNpmPackages(runtimeID, runtimePath)
	case "python":
		packages, err = s.scanPipPackages(runtimeID, runtimePath)
	case "java":
		packages, err = s.scanMavenPackages(runtimeID, runtimePath)
	case "php":
		packages, err = s.scanComposerPackages(runtimeID, runtimePath)
	default:
		return []model.Package{}, nil
	}

	if err != nil {
		return nil, err
	}

	// Save to database
	for _, pkg := range packages {
		s.db.Exec(
			"INSERT OR REPLACE INTO packages (runtime_id, name, version, scope, source) VALUES (?, ?, ?, ?, ?)",
			pkg.RuntimeID, pkg.Name, pkg.Version, pkg.Scope, pkg.Source,
		)
	}

	return packages, nil
}

// InstallPackage installs a package
func (s *PackageManagerService) InstallPackage(req *model.PackageInstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.installNpmPackage(req, runtimePath)
	case "python":
		return s.installPipPackage(req, runtimePath)
	case "java":
		return s.installMavenPackage(req, runtimePath)
	case "php":
		return s.installComposerPackage(req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UninstallPackage uninstalls a package
func (s *PackageManagerService) UninstallPackage(req *model.PackageUninstallRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.uninstallNpmPackage(req, runtimePath)
	case "python":
		return s.uninstallPipPackage(req, runtimePath)
	case "java":
		return s.uninstallMavenPackage(req, runtimePath)
	case "php":
		return s.uninstallComposerPackage(req, runtimePath)
	default:
		return fmt.Errorf("package management not supported for %s", runtimeName)
	}
}

// UpdatePackage updates a package
func (s *PackageManagerService) UpdatePackage(req *model.PackageUpdateRequest, runtimeName, runtimePath string) error {
	switch runtimeName {
	case "node":
		return s.updateNpmPackage(req, runtimePath)
	case "python":
		return s.updatePipPackage(req, runtimePath)
	default:
		return fmt.Errorf("package update not supported for %s", runtimeName)
	}
}

// SearchPackages searches for available packages
func (s *PackageManagerService) SearchPackages(runtimeName, query string) ([]model.PackageInfo, error) {
	switch runtimeName {
	case "node":
		return s.searchNpmPackages(query)
	case "python":
		return s.searchPipPackages(query)
	default:
		return []model.PackageInfo{}, nil
	}
}

// GetPackageVersions returns available versions for a package
func (s *PackageManagerService) GetPackageVersions(runtimeName, packageName string) ([]string, error) {
	switch runtimeName {
	case "node":
		return s.getNpmPackageVersions(packageName)
	case "python":
		return s.getPipPackageVersions(packageName)
	default:
		return []string{}, nil
	}
}

// npm package search
func (s *PackageManagerService) searchNpmPackages(query string) ([]model.PackageInfo, error) {
	// npm search --json
	cmd := exec.Command("npm", "search", query, "--json")
	output, err := cmd.Output()
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
	if err := json.Unmarshal(output, &result); err != nil {
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
func (s *PackageManagerService) getNpmPackageVersions(packageName string) ([]string, error) {
	// npm view <package> versions --json
	cmd := exec.Command("npm", "view", packageName, "versions", "--json")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("package: npm view error: %v", err)
		return []string{}, nil
	}

	// Parse JSON output (array of versions)
	var versions []string
	if err := json.Unmarshal(output, &versions); err != nil {
		// Try single version
		var single string
		if err2 := json.Unmarshal(output, &single); err2 == nil {
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
func (s *PackageManagerService) searchPipPackages(query string) ([]model.PackageInfo, error) {
	// pip search is deprecated, use pip index versions
	// For now, return a placeholder
	return []model.PackageInfo{}, fmt.Errorf("pip search not supported, use pip install <package>")
}

// pip package versions
func (s *PackageManagerService) getPipPackageVersions(packageName string) ([]string, error) {
	// pip index versions <package>
	cmd := exec.Command("pip", "index", "versions", packageName)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("package: pip index error: %v", err)
		return []string{}, nil
	}

	// Parse output: "package_name (X.Y.Z, X.Y.Z, ...)"
	outputStr := string(output)
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
func (s *PackageManagerService) scanNpmPackages(runtimeID int64, runtimePath string) ([]model.Package, error) {
	// npm list -g --json
	cmd := exec.Command("npm", "list", "-g", "--json")
	output, err := cmd.Output()
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
	if err := json.Unmarshal(output, &result); err != nil {
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

func (s *PackageManagerService) installNpmPackage(req *model.PackageInstallRequest, runtimePath string) error {
	args := []string{"install", "-g"}
	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s@%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	cmd := exec.Command("npm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm install failed: %s", string(output))
	}

	log.Printf("package: installed %s via npm", req.Name)
	return nil
}

func (s *PackageManagerService) uninstallNpmPackage(req *model.PackageUninstallRequest, runtimePath string) error {
	cmd := exec.Command("npm", "uninstall", "-g", req.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm uninstall failed: %s", string(output))
	}

	// Remove from database
	s.db.Exec("DELETE FROM packages WHERE runtime_id = ? AND name = ? AND scope = 'global'", req.RuntimeID, req.Name)

	log.Printf("package: uninstalled %s via npm", req.Name)
	return nil
}

func (s *PackageManagerService) updateNpmPackage(req *model.PackageUpdateRequest, runtimePath string) error {
	cmd := exec.Command("npm", "update", "-g", req.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm update failed: %s", string(output))
	}

	log.Printf("package: updated %s via npm", req.Name)
	return nil
}

// pip package management
func (s *PackageManagerService) scanPipPackages(runtimeID int64, runtimePath string) ([]model.Package, error) {
	// pip list --format=json
	cmd := exec.Command("pip", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("package: pip list error: %v", err)
		return []model.Package{}, nil
	}

	// Parse JSON output
	var result []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
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

func (s *PackageManagerService) installPipPackage(req *model.PackageInstallRequest, runtimePath string) error {
	args := []string{"install"}
	if req.Version != "" {
		args = append(args, fmt.Sprintf("%s==%s", req.Name, req.Version))
	} else {
		args = append(args, req.Name)
	}

	cmd := exec.Command("pip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install failed: %s", string(output))
	}

	log.Printf("package: installed %s via pip", req.Name)
	return nil
}

func (s *PackageManagerService) uninstallPipPackage(req *model.PackageUninstallRequest, runtimePath string) error {
	cmd := exec.Command("pip", "uninstall", "-y", req.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip uninstall failed: %s", string(output))
	}

	// Remove from database
	s.db.Exec("DELETE FROM packages WHERE runtime_id = ? AND name = ? AND scope = 'global'", req.RuntimeID, req.Name)

	log.Printf("package: uninstalled %s via pip", req.Name)
	return nil
}

func (s *PackageManagerService) updatePipPackage(req *model.PackageUpdateRequest, runtimePath string) error {
	cmd := exec.Command("pip", "install", "--upgrade", req.Name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip update failed: %s", string(output))
	}

	log.Printf("package: updated %s via pip", req.Name)
	return nil
}

// maven package management (placeholder)
func (s *PackageManagerService) scanMavenPackages(runtimeID int64, runtimePath string) ([]model.Package, error) {
	// Maven doesn't have a simple list command like npm/pip
	// Would need to parse pom.xml or ~/.m2/repository
	return []model.Package{}, nil
}

func (s *PackageManagerService) installMavenPackage(req *model.PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package installation not yet supported")
}

func (s *PackageManagerService) uninstallMavenPackage(req *model.PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("maven package uninstallation not yet supported")
}

// composer package management (placeholder)
func (s *PackageManagerService) scanComposerPackages(runtimeID int64, runtimePath string) ([]model.Package, error) {
	// composer show -i --format=json
	return []model.Package{}, nil
}

func (s *PackageManagerService) installComposerPackage(req *model.PackageInstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package installation not yet supported")
}

func (s *PackageManagerService) uninstallComposerPackage(req *model.PackageUninstallRequest, runtimePath string) error {
	return fmt.Errorf("composer package uninstallation not yet supported")
}
