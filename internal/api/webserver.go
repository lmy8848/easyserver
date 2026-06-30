package api

import (
	"easyserver/internal/api/middleware"
	"easyserver/internal/web"
	"github.com/gin-gonic/gin"
)

// registerWebServerRoutes registers web server and website management routes
func registerWebServerRoutes(protected *gin.RouterGroup, webServerService *web.Service, websiteService *web.WebsiteService) {
	handler := NewWebServerHandler(webServerService, websiteService)

	// Utilities (must be before /:id to avoid conflict)
	protected.GET("/web-servers/project-types", handler.GetProjectTypes)
	protected.GET("/web-servers/browse", handler.BrowseDirs)
	protected.GET("/web-servers/validate-path", handler.ValidatePath)

	// Web server CRUD
	protected.GET("/web-servers", handler.List)
	protected.GET("/web-servers/:id", handler.Get)
	protected.POST("/web-servers", middleware.SetAction("WEBSERVERS_CREATE"), handler.Create)
	protected.DELETE("/web-servers/:id", middleware.SetAction("WEBSERVERS_DELETE"), handler.Delete)
	protected.POST("/web-servers/:id/install", middleware.SetAction("WEBSERVERS_INSTALL"), handler.Install)
	protected.POST("/web-servers/:id/uninstall", middleware.SetAction("WEBSERVERS_UNINSTALL"), handler.Uninstall)
	protected.POST("/web-servers/:id/start", middleware.SetAction("WEBSERVERS_START"), handler.Start)
	protected.POST("/web-servers/:id/stop", middleware.SetAction("WEBSERVERS_STOP"), handler.Stop)
	protected.POST("/web-servers/:id/restart", middleware.SetAction("WEBSERVERS_RESTART"), handler.Restart)
	protected.GET("/web-servers/:id/status", handler.Status)
	protected.POST("/web-servers/:id/reload", handler.Reload)
	protected.GET("/web-servers/:id/test-config", handler.TestConfig)
	protected.GET("/web-servers/:id/config", handler.GetConfig)
	protected.PUT("/web-servers/:id/config", middleware.SetAction("WEBSERVERS_UPDATE_CONFIG"), handler.SaveConfig)
	protected.GET("/web-servers/:id/logs", handler.GetServiceLogs)
	protected.POST("/web-servers/:id/auto-start", handler.SetAutoStart)
	protected.GET("/web-servers/:id/process", handler.GetProcessInfo)

	// Websites nested under web server (:id = server, :wid = website)
	protected.GET("/web-servers/:id/websites", handler.ListWebsites)
	protected.GET("/web-servers/:id/websites/:wid", handler.GetWebsite)
	protected.POST("/web-servers/:id/websites", middleware.SetAction("WEBSITES_CREATE"), handler.CreateWebsite)
	protected.PUT("/web-servers/:id/websites/:wid", middleware.SetAction("WEBSITES_UPDATE"), handler.UpdateWebsite)
	protected.DELETE("/web-servers/:id/websites/:wid", middleware.SetAction("WEBSITES_DELETE"), handler.DeleteWebsite)
	protected.POST("/web-servers/:id/websites/:wid/enable", middleware.SetAction("WEBSITES_ENABLE"), handler.EnableWebsite)
	protected.POST("/web-servers/:id/websites/:wid/disable", middleware.SetAction("WEBSITES_DISABLE"), handler.DisableWebsite)
	protected.GET("/web-servers/:id/websites/:wid/logs", handler.GetWebsiteLogs)
	protected.POST("/web-servers/:id/websites/:wid/ssl", middleware.SetAction("WEBSITES_APPLY_SSL"), handler.ApplyWebsiteSSL)
}
