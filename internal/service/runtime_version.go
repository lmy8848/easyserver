package service

import (
	"easyserver/internal/runtimeenv"
)

// RuntimeVersionService is kept as a compatibility alias for the runtimeenv.VersionService.
type RuntimeVersionService = runtimeenv.VersionService

// NewRuntimeVersionService is a compatibility wrapper; prefer runtimeenv.NewVersionService directly.
func NewRuntimeVersionService(repo runtimeenv.Repository) *RuntimeVersionService {
	return runtimeenv.NewVersionService(repo)
}
