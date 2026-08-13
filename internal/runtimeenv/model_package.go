package runtimeenv

// Package represents an installed package, sourced directly from the system
// package manager at query time (no DB caching).
type Package struct {
	RuntimeName string `json:"runtime_name"` // node / python / php
	Name        string `json:"name"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Source      string `json:"source"`
}

// PackageInstallRequest represents a request to install a package.
// runtime 通过 query 参数传递（?runtime=lang@exact，与其它包管理端点一致）。
type PackageInstallRequest struct {
	Name    string `json:"name" binding:"required"`
	Version string `json:"version"`
	Scope   string `json:"scope"`
	Manager string `json:"manager"` // npm, pnpm, etc
}

// PackageUninstallRequest represents a request to uninstall a package
type PackageUninstallRequest struct {
	Name    string `json:"name" binding:"required"`
	Manager string `json:"manager"`
}

// PackageUpdateRequest represents a request to update a package
type PackageUpdateRequest struct {
	Name    string `json:"name" binding:"required"`
	Manager string `json:"manager"`
}

// PackageInfo represents package information for search results
type PackageInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source"`
}
