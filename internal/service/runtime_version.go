package service

import (
	"easyserver/internal/runtimeenv"
	"easyserver/internal/runtimeversion"
)

// RuntimeVersionService is kept as a compatibility alias for the runtimeversion module.
type RuntimeVersionService = runtimeversion.Service

// NewRuntimeVersionService is a compatibility wrapper; prefer runtimeversion.NewService directly.
func NewRuntimeVersionService(repo runtimeenv.Repository) *RuntimeVersionService {
	return runtimeversion.NewService(repo)
}
