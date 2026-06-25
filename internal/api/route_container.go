package api

import (
	"easyserver/internal/executor"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerContainerRoutes registers all container-related routes
func registerContainerRoutes(protected *gin.RouterGroup, auditService *service.AuditService) {
	exec := executor.NewOSExecutor()

	containerService := service.NewContainerService(exec)
	dockerService := service.NewDockerService(exec)
	composeService := service.NewComposeService(exec)
	volumeService := service.NewVolumeService(exec)
	networkService := service.NewNetworkService(exec)

	handler := NewContainerHandler(containerService, dockerService, composeService, volumeService, networkService, auditService)

	// Docker management
	protected.GET("/docker/status", handler.DetectDocker)
	protected.POST("/docker/install", handler.InstallDocker)
	protected.POST("/docker/start", handler.StartDocker)
	protected.POST("/docker/stop", handler.StopDocker)
	protected.POST("/docker/restart", handler.RestartDocker)
	protected.GET("/docker/info", handler.GetDockerInfo)
	protected.POST("/docker/mirror", handler.ConfigureMirror)

	// Container management
	protected.GET("/containers", handler.ListContainers)
	protected.GET("/containers/:id", handler.GetContainer)
	protected.POST("/containers", handler.CreateContainer)
	protected.POST("/containers/:id/start", handler.StartContainer)
	protected.POST("/containers/:id/stop", handler.StopContainer)
	protected.POST("/containers/:id/restart", handler.RestartContainer)
	protected.POST("/containers/:id/pause", handler.PauseContainer)
	protected.POST("/containers/:id/unpause", handler.UnpauseContainer)
	protected.DELETE("/containers/:id", handler.RemoveContainer)
	protected.GET("/containers/:id/logs", handler.GetContainerLogs)
	protected.POST("/containers/:id/exec", handler.ExecInContainer)
	protected.GET("/containers/:id/stats", handler.GetContainerStats)
	protected.GET("/containers/:id/top", handler.GetContainerTop)
	protected.POST("/containers/:id/copy-to", handler.CopyToContainer)
	protected.POST("/containers/:id/copy-from", handler.CopyFromContainer)
	protected.POST("/containers/:id/rename", handler.RenameContainer)
	protected.PUT("/containers/:id/update", handler.UpdateContainer)

	// Image management
	protected.GET("/images", handler.ListImages)
	protected.POST("/images/pull", handler.PullImage)
	protected.DELETE("/images/:id", handler.RemoveImage)

	// Compose management
	protected.GET("/compose/projects", handler.ListComposeProjects)
	protected.POST("/compose/up", handler.ComposeUp)
	protected.POST("/compose/down", handler.ComposeDown)
	protected.POST("/compose/restart", handler.ComposeRestart)
	protected.GET("/compose/logs", handler.ComposeLogs)
	protected.GET("/compose/config", handler.ComposeGetConfig)
	protected.PUT("/compose/config", handler.ComposeSaveConfig)

	// Volume management
	protected.GET("/volumes", handler.ListVolumes)
	protected.POST("/volumes", handler.CreateVolume)
	protected.DELETE("/volumes/:name", handler.RemoveVolume)

	// Network management
	protected.GET("/networks", handler.ListNetworks)
	protected.POST("/networks", handler.CreateNetwork)
	protected.DELETE("/networks/:id", handler.RemoveNetwork)
}
