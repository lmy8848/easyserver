package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/packagemanager"
	"easyserver/internal/repository"
)

// PackageManagerService is kept as a compatibility alias for the packagemanager module.
type PackageManagerService = packagemanager.Service

func NewPackageManagerService(repo repository.PackageRepository, exec executor.CommandExecutor) *PackageManagerService {
	return packagemanager.NewService(repo, exec)
}
