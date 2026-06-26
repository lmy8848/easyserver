package service

import (
	"easyserver/internal/dbserver"
	"easyserver/internal/executor"
)

// DBServerService is kept as a compatibility alias for the dbserver module.
type DBServerService = dbserver.Service

func NewDBServerService(exec executor.CommandExecutor, repo dbserver.Repository) *DBServerService {
	return dbserver.NewService(exec, repo)
}
