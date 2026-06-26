package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/packagemanager"
)

// PackageManagerService is kept as a compatibility alias for the packagemanager module.
type PackageManagerService = packagemanager.Service

func NewPackageManagerService(repo packagemanager.Repository, exec executor.CommandExecutor) *PackageManagerService {
	return packagemanager.NewService(repo, exec)
}
