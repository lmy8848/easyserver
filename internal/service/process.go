package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/process"
	"easyserver/internal/repository"
)

// ProcessManager is now defined in easyserver/internal/process.Service.
// Kept as alias for backward compatibility.
type ProcessManager = process.Service

// NewProcessManager creates a new process manager.
// Delegates to the domain package implementation.
func NewProcessManager(repo repository.ProcessRepository, exec executor.CommandExecutor) *ProcessManager {
	return process.NewService(repo, exec)
}
