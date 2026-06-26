package packagemanager

import "time"

// Package represents an installed package
type Package struct {
	ID          int64     `json:"id"`
	RuntimeID   int64     `json:"runtime_id"`
	RuntimeName string    `json:"runtime_name"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Scope       string    `json:"scope"`
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
}

// PackageInstallRequest represents a request to install a package
type PackageInstallRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Version   string `json:"version"`
	Scope     string `json:"scope"`
}

// PackageUninstallRequest represents a request to uninstall a package
type PackageUninstallRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
}

// PackageUpdateRequest represents a request to update a package
type PackageUpdateRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
}

// PackageInfo represents package information for search results
type PackageInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source"`
}
