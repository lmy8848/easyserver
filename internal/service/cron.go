package service

import (
	"easyserver/internal/cron"
	"easyserver/internal/executor"
)

// CronService is now defined in easyserver/internal/cron.Service.
// Kept as alias for backward compatibility.
type CronService = cron.Service

// NewCronService creates a new CronService.
// Delegates to cron.NewService.
func NewCronService(repo cron.Repository, exec executor.CommandExecutor) *CronService {
	return cron.NewService(repo, exec)
}
