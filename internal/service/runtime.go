package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/runtimeenv"
)

// RuntimeService is kept as a compatibility alias for the runtimeenv module.
type RuntimeService = runtimeenv.Service

// NewRuntimeService is a compatibility wrapper; prefer runtimeenv.NewService directly.
func NewRuntimeService(repo runtimeenv.Repository, exec executor.CommandExecutor) *RuntimeService {
	return runtimeenv.NewService(repo, exec)
}
