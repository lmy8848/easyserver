package model

import "easyserver/internal/packagemanager"

// Type aliases for backward compatibility.
// Canonical definitions live in internal/packagemanager/model.go.

type Package = packagemanager.Package
type PackageInfo = packagemanager.PackageInfo
type PackageInstallRequest = packagemanager.PackageInstallRequest
type PackageUninstallRequest = packagemanager.PackageUninstallRequest
type PackageUpdateRequest = packagemanager.PackageUpdateRequest
