package service

import (
	"easyserver/internal/dbserver"
	"easyserver/internal/executor"
	"easyserver/internal/repository"
)

// DBServerService is kept as a compatibility alias for the dbserver module.
type DBServerService = dbserver.Service

func NewDBServerService(exec executor.CommandExecutor, repo repository.DBServerRepository) *DBServerService {
	return dbserver.NewService(exec, repo)
}
