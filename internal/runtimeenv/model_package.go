package runtimeenv

// Package represents an installed package, sourced directly from the system
// package manager at query time (no DB caching).
type Package struct {
	RuntimeID   int64  `json:"runtime_id"`
	RuntimeName string `json:"runtime_name"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Source      string `json:"source"`
}

// PackageInstallRequest represents a request to install a package
type PackageInstallRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	Manager   string `json:"manager"` // npm, pnpm, etc
}

// PackageUninstallRequest represents a request to uninstall a package
type PackageUninstallRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Manager   string `json:"manager"`
}

// PackageUpdateRequest represents a request to update a package
type PackageUpdateRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Manager   string `json:"manager"`
}

// PackageInfo represents package information for search results
type PackageInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source"`
}
