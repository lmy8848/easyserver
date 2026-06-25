package service

import (
	"database/sql"

	"easyserver/internal/executor"
	"easyserver/internal/runtimeenv"
)

// RuntimeService is kept as a compatibility alias for the runtimeenv module.
type RuntimeService = runtimeenv.Service

func NewRuntimeService(db *sql.DB, exec executor.CommandExecutor) *RuntimeService {
	return runtimeenv.NewService(db, exec)
}
