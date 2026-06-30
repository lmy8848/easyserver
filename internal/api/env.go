package api

import (
	"context"
	"easyserver/internal/api/middleware"

	"easyserver/internal/envconfig"

	"github.com/gin-gonic/gin"
)

// registerEnvRoutes registers environment configuration routes
func registerEnvRoutes(protected *gin.RouterGroup, envConfigService *envconfig.Service) {
	// Initialize default global configs (tables managed by migration system)
	envConfigService.InitDefaultGlobalConfigs(context.Background())
	handler := NewEnvConfigHandler(envConfigService)
	protected.GET("/env-config", handler.ListEnvConfigs)
	protected.GET("/env-config/:id", handler.GetEnvConfig)
	protected.POST("/env-config", middleware.SetAction("ENV_CREATE_CONFIG"), handler.CreateEnvConfig)
	protected.PUT("/env-config/:id", middleware.SetAction("ENV_UPDATE_CONFIG"), handler.UpdateEnvConfig)
	protected.DELETE("/env-config/:id", middleware.SetAction("ENV_DELETE_CONFIG"), handler.DeleteEnvConfig)
	protected.GET("/env-config/path", handler.ListPathEntries)
	protected.POST("/env-config/path", middleware.SetAction("ENV_CREATE_PATH"), handler.CreatePathEntry)
	protected.DELETE("/env-config/path/:id", middleware.SetAction("ENV_DELETE_PATH"), handler.DeletePathEntry)
	protected.GET("/env-config/script", handler.GenerateEnvScript)

	// Global configuration
	protected.GET("/global-config", handler.ListGlobalConfigs)
	protected.GET("/global-config/:id", handler.GetGlobalConfig)
	protected.POST("/global-config", middleware.SetAction("ENV_CREATE_GLOBAL"), handler.CreateGlobalConfig)
	protected.PUT("/global-config/:id", middleware.SetAction("ENV_UPDATE_GLOBAL"), handler.UpdateGlobalConfig)
	protected.DELETE("/global-config/:id", middleware.SetAction("ENV_DELETE_GLOBAL"), handler.DeleteGlobalConfig)
}
