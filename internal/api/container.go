package api

import (
	"fmt"
	"strconv"
	
	"easyserver/internal/container"
	"easyserver/internal/audit"
	"github.com/gin-gonic/gin"
)

// ContainerHandler handles all container-related requests
type ContainerHandler struct {
	containerService *container.Service
	auditService     *audit.Service
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(
	containerService *container.Service,
	auditService *audit.Service,
) *ContainerHandler {
	return &ContainerHandler{
		containerService: containerService,
		auditService:     auditService,
	}
}

// ========== Docker Management ==========

// DetectDocker checks Docker installation status
func (h *ContainerHandler) DetectDocker(c *gin.Context) {
	status, err := h.containerService.DetectDocker(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, status)
}

// InstallDocker installs Docker using official script
func (h *ContainerHandler) InstallDocker(c *gin.Context) {
	if err := h.containerService.InstallDocker(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Docker 安装成功"})
}

// StartDocker starts the Docker service
func (h *ContainerHandler) StartDocker(c *gin.Context) {
	if err := h.containerService.StartDocker(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Docker 已启动"})
}

// StopDocker stops the Docker service
func (h *ContainerHandler) StopDocker(c *gin.Context) {
	if err := h.containerService.StopDocker(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Docker 已停止"})
}

// RestartDocker restarts the Docker service
func (h *ContainerHandler) RestartDocker(c *gin.Context) {
	if err := h.containerService.RestartDocker(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Docker 已重启"})
}

// GetDockerInfo returns Docker system info
func (h *ContainerHandler) GetDockerInfo(c *gin.Context) {
	info, err := h.containerService.GetDockerInfo(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, info)
}

// ConfigureMirror configures Docker registry mirror
func (h *ContainerHandler) ConfigureMirror(c *gin.Context) {
	var req struct {
		MirrorURL string `json:"mirror_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.ConfigureMirror(c.Request.Context(), req.MirrorURL); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "镜像源已配置"})
}

// ========== Container Management ==========

// ListContainers returns all containers
func (h *ContainerHandler) ListContainers(c *gin.Context) {
	all := c.Query("all") == "true"
	containers, err := h.containerService.ListContainers(c.Request.Context(), all)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"containers": containers})
}

// GetContainer returns a specific container
func (h *ContainerHandler) GetContainer(c *gin.Context) {
	id := c.Param("id")
	container, err := h.containerService.GetContainer(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("容器不存在"))
		return
	}
	Success(c, container)
}

// CreateContainer creates a new container
func (h *ContainerHandler) CreateContainer(c *gin.Context) {
	var req container.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	id, err := h.containerService.CreateContainer(c.Request.Context(), req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"id": id, "message": "容器已创建"})
}

// StartContainer starts a container
func (h *ContainerHandler) StartContainer(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.StartContainer(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已启动"})
}

// StopContainer stops a container
func (h *ContainerHandler) StopContainer(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.StopContainer(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已停止"})
}

// RestartContainer restarts a container
func (h *ContainerHandler) RestartContainer(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.RestartContainer(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已重启"})
}

// PauseContainer pauses a container
func (h *ContainerHandler) PauseContainer(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.PauseContainer(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已暂停"})
}

// UnpauseContainer unpauses a container
func (h *ContainerHandler) UnpauseContainer(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.UnpauseContainer(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已恢复"})
}

// RemoveContainer removes a container
func (h *ContainerHandler) RemoveContainer(c *gin.Context) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	if err := h.containerService.RemoveContainer(c.Request.Context(), id, force); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已删除"})
}

// GetContainerLogs returns container logs
func (h *ContainerHandler) GetContainerLogs(c *gin.Context) {
	id := c.Param("id")
	tail := container.DefaultLogTail
	if t, err := strconv.Atoi(c.Query("tail")); err == nil && t > 0 {
		if t > container.MaxLogTail {
			t = container.MaxLogTail
		}
		tail = t
	}

	logs, err := h.containerService.GetContainerLogs(c.Request.Context(), id, tail)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"logs": logs})
}

// ExecInContainer executes a command in a container
func (h *ContainerHandler) ExecInContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	// Log exec command for audit
	_ = c.GetInt64("user_id")
	username, _ := c.Get("username")
	if h.auditService != nil {
		if unameStr, ok := username.(string); ok {
			h.auditService.LogSecurityEvent(c.Request.Context(), unameStr,
				"CONTAINER_EXEC",
				fmt.Sprintf("Container: %s, Command: %s", id, req.Command),
				c.ClientIP(), c.Request.UserAgent())
		}
	}

	output, err := h.containerService.ExecInContainer(c.Request.Context(), id, req.Command)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"output": output})
}

// GetContainerStats returns real-time container stats
func (h *ContainerHandler) GetContainerStats(c *gin.Context) {
	id := c.Param("id")
	stats, err := h.containerService.GetContainerStats(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, stats)
}

// GetContainerTop returns processes running in a container
func (h *ContainerHandler) GetContainerTop(c *gin.Context) {
	id := c.Param("id")
	processes, err := h.containerService.GetContainerTop(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"processes": processes})
}

// CopyToContainer copies a file to a container
func (h *ContainerHandler) CopyToContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.CopyToContainer(c.Request.Context(), id, req.SrcPath, req.DestPath); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "文件已复制到容器"})
}

// CopyFromContainer copies a file from a container
func (h *ContainerHandler) CopyFromContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.CopyFromContainer(c.Request.Context(), id, req.SrcPath, req.DestPath); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "文件已从容器复制"})
}

// RenameContainer renames a container
func (h *ContainerHandler) RenameContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.RenameContainer(c.Request.Context(), id, req.Name); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已重命名"})
}

// UpdateContainer updates container resources
func (h *ContainerHandler) UpdateContainer(c *gin.Context) {
	id := c.Param("id")
	var req container.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.UpdateContainer(c.Request.Context(), id, req); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "容器已更新"})
}

// ========== Image Management ==========

// ListImages returns all Docker images
func (h *ContainerHandler) ListImages(c *gin.Context) {
	images, err := h.containerService.ListImages(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"images": images})
}

// PullImage pulls a Docker image
func (h *ContainerHandler) PullImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.PullImage(c.Request.Context(), req.Image); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "镜像已拉取"})
}

// RemoveImage removes a Docker image
func (h *ContainerHandler) RemoveImage(c *gin.Context) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	if err := h.containerService.RemoveImage(c.Request.Context(), id, force); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "镜像已删除"})
}

// ========== Compose Management ==========

// ListComposeProjects lists all compose projects
func (h *ContainerHandler) ListComposeProjects(c *gin.Context) {
	projects, err := h.containerService.ListProjects(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"projects": projects})
}

// ComposeUp runs docker compose up -d
func (h *ContainerHandler) ComposeUp(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.ComposeUp(c.Request.Context(), req.ProjectDir); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Compose 已启动"})
}

// ComposeDown runs docker compose down
func (h *ContainerHandler) ComposeDown(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.ComposeDown(c.Request.Context(), req.ProjectDir); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Compose 已停止"})
}

// ComposeRestart runs docker compose restart
func (h *ContainerHandler) ComposeRestart(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.ComposeRestart(c.Request.Context(), req.ProjectDir); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "Compose 已重启"})
}

// ComposeLogs returns compose project logs
func (h *ContainerHandler) ComposeLogs(c *gin.Context) {
	projectDir := c.Query("dir")
	tail := container.DefaultLogTail
	if t, err := strconv.Atoi(c.Query("tail")); err == nil && t > 0 {
		if t > container.MaxLogTail {
			t = container.MaxLogTail
		}
		tail = t
	}

	logs, err := h.containerService.ComposeGetLogs(c.Request.Context(), projectDir, tail)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"logs": logs})
}

// ComposeGetConfig reads docker-compose.yml content
func (h *ContainerHandler) ComposeGetConfig(c *gin.Context) {
	projectDir := c.Query("dir")
	content, err := h.containerService.ComposeGetConfig(c.Request.Context(), projectDir)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"content": content})
}

// ComposeSaveConfig writes docker-compose.yml content
func (h *ContainerHandler) ComposeSaveConfig(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.ComposeSaveConfig(c.Request.Context(), req.ProjectDir, req.Content); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "配置已保存"})
}

// ========== Volume Management ==========

// ListVolumes returns all Docker volumes
func (h *ContainerHandler) ListVolumes(c *gin.Context) {
	volumes, err := h.containerService.ListVolumes(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"volumes": volumes})
}

// CreateVolume creates a new Docker volume
func (h *ContainerHandler) CreateVolume(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.CreateVolume(c.Request.Context(), req.Name, req.Driver); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "卷已创建"})
}

// RemoveVolume removes a Docker volume
func (h *ContainerHandler) RemoveVolume(c *gin.Context) {
	name := c.Param("name")
	force := c.Query("force") == "true"
	if err := h.containerService.RemoveVolume(c.Request.Context(), name, force); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "卷已删除"})
}

// ========== Network Management ==========

// ListNetworks returns all Docker networks
func (h *ContainerHandler) ListNetworks(c *gin.Context) {
	networks, err := h.containerService.ListNetworks(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"networks": networks})
}

// CreateNetwork creates a new Docker network
func (h *ContainerHandler) CreateNetwork(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.containerService.CreateNetwork(c.Request.Context(), req.Name, req.Driver); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "网络已创建"})
}

// RemoveNetwork removes a Docker network
func (h *ContainerHandler) RemoveNetwork(c *gin.Context) {
	id := c.Param("id")
	if err := h.containerService.RemoveNetwork(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "网络已删除"})
}

func registerContainerRoutes(protected *gin.RouterGroup, containerService *container.Service, auditService *audit.Service) {
	handler := NewContainerHandler(containerService, auditService)

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
