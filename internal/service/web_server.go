package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/web"
)

// WebServerService is a compatibility alias; implementation moved to internal/web.
type WebServerService = web.Service

// NewWebServerService is a forwarding stub.
func NewWebServerService(repo web.ServerRepository, exec executor.CommandExecutor) *WebServerService {
	return web.NewService(repo, exec)
}
