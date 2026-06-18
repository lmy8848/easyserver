package model

import "time"

// Package represents an installed package
type Package struct {
	ID          int64     `json:"id"`
	RuntimeID   int64     `json:"runtime_id"`   // Associated runtime environment ID
	RuntimeName string    `json:"runtime_name"` // java, node, python, etc.
	Name        string    `json:"name"`         // express, flask, requests, etc.
	Version     string    `json:"version"`      // 4.18.2, 3.0.0, etc.
	Scope       string    `json:"scope"`        // global, local
	Source      string    `json:"source"`       // npm, pip, maven, composer
	InstalledAt time.Time `json:"installed_at"`
}

// PackageInstallRequest represents a request to install a package
type PackageInstallRequest struct {
	RuntimeID int64  `json:"runtime_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Version   string `json:"version"` // Optional, empty = latest
	Scope     string `json:"scope"`   // global, local
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
