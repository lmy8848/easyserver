package service

import (
	"easyserver/internal/executor"
	"easyserver/internal/web"
)

// WebsiteService is a compatibility alias; implementation moved to internal/web.
type WebsiteService = web.WebsiteService

// NewWebsiteService is a forwarding stub.
func NewWebsiteService(repo web.WebsiteRepository, webServerRepo web.ServerRepository, exec executor.CommandExecutor) *WebsiteService {
	return web.NewWebsiteService(repo, webServerRepo, exec)
}
