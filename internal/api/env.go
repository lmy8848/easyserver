package api

import (
	"easyserver/internal/envconfig"

	"github.com/gin-gonic/gin"
)

// registerEnvRoutes registers environment configuration routes
func registerEnvRoutes(protected *gin.RouterGroup, envConfigService *envconfig.Service) {
	handler := NewEnvConfigHandler(envConfigService)
	protected.GET("/env-config", handler.ListEnvConfigs)
	protected.GET("/env-config/:id", handler.GetEnvConfig)
	protected.POST("/env-config", handler.CreateEnvConfig)
	protected.PUT("/env-config/:id", handler.UpdateEnvConfig)
	protected.DELETE("/env-config/:id", handler.DeleteEnvConfig)
	protected.GET("/env-config/path", handler.ListPathEntries)
	protected.POST("/env-config/path", handler.CreatePathEntry)
	protected.DELETE("/env-config/path/:id", handler.DeletePathEntry)
	protected.GET("/env-config/script", handler.GenerateEnvScript)
}
