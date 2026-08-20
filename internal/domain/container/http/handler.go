package http

import (
	"easyserver/internal/domain/container"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// ContainerHandler handles all container-related requests
type ContainerHandler struct {
	containerService *container.Service
}

// NewContainerHandler creates a new ContainerHandler
func NewContainerHandler(
	containerService *container.Service,
) *ContainerHandler {
	return &ContainerHandler{
		containerService: containerService,
	}
}

// engineName returns the engine selected by the `?engine=` query param,
// defaulting to docker.
func (h *ContainerHandler) engineName(c *gin.Context) container.Engine {
	if r := c.Query("engine"); r == string(container.EnginePodman) {
		return container.EnginePodman
	}
	return container.EngineDocker
}

// ========== Runtime Management ==========

// DetectEngine checks a engine's installation status
func (h *ContainerHandler) DetectEngine(c *gin.Context) (any, error) {
	status, err := h.containerService.Detect(c.Request.Context(), h.engineName(c))
	if err != nil {
		return nil, err
	}
	return status, nil
}

// InstallEngine installs a engine
func (h *ContainerHandler) InstallEngine(c *gin.Context) (any, error) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "安装 "+string(engine))
	if err := h.containerService.Install(c.Request.Context(), engine); err != nil {
		return nil, err
	}
	return gin.H{"message": string(engine) + " 安装成功"}, nil
}

// StartEngine starts a engine's service
func (h *ContainerHandler) StartEngine(c *gin.Context) (any, error) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "启动 "+string(engine))
	if err := h.containerService.StartEngine(c.Request.Context(), engine); err != nil {
		return nil, err
	}
	return gin.H{"message": string(engine) + " 已启动"}, nil
}

// StopEngine stops a engine's service
func (h *ContainerHandler) StopEngine(c *gin.Context) (any, error) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "停止 "+string(engine))
	if err := h.containerService.StopEngine(c.Request.Context(), engine); err != nil {
		return nil, err
	}
	return gin.H{"message": string(engine) + " 已停止"}, nil
}

// RestartEngine restarts a engine's service
func (h *ContainerHandler) RestartEngine(c *gin.Context) (any, error) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "重启 "+string(engine))
	if err := h.containerService.RestartEngine(c.Request.Context(), engine); err != nil {
		return nil, err
	}
	return gin.H{"message": string(engine) + " 已重启"}, nil
}

// EnableSocket enables a engine's API socket unit (e.g. podman.socket).
func (h *ContainerHandler) EnableSocket(c *gin.Context) (any, error) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "启用 "+string(engine)+" Socket")
	if err := h.containerService.EnableSocket(c.Request.Context(), engine); err != nil {
		return nil, err
	}
	return gin.H{"message": "Socket 已启用"}, nil
}

// GetEngineInfo returns a engine's system info
func (h *ContainerHandler) GetEngineInfo(c *gin.Context) (any, error) {
	info, err := h.containerService.GetInfo(c.Request.Context(), h.engineName(c))
	if err != nil {
		return nil, err
	}
	return info, nil
}

// ConfigureMirror configures a engine's registry mirror
func (h *ContainerHandler) ConfigureMirror(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		MirrorURL string `json:"mirror_url"`
	}](c)
	if err != nil {
		return nil, err
	}

	engine := h.engineName(c)
	middleware.AuditSummary(c, "配置 "+string(engine)+" 镜像源 "+req.MirrorURL)
	if err := h.containerService.ConfigureMirror(c.Request.Context(), engine, req.MirrorURL); err != nil {
		return nil, err
	}
	return gin.H{"message": "镜像源已配置"}, nil
}

// GetRegistryConfig returns the engine's mirror + insecure registries.
func (h *ContainerHandler) GetRegistryConfig(c *gin.Context) (any, error) {
	cfg := h.containerService.GetRegistryConfig(c.Request.Context(), h.engineName(c))
	return cfg, nil
}

// SetRegistryConfig persists the engine's mirror + insecure registries.
func (h *ContainerHandler) SetRegistryConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[container.RegistryConfig](c)
	if err != nil {
		return nil, err
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "配置 "+string(engine)+" 镜像仓库")
	if err := h.containerService.SetRegistryConfig(c.Request.Context(), engine, req); err != nil {
		return nil, err
	}
	return gin.H{"message": "镜像仓库配置已保存"}, nil
}

// RegistryLogin logs into a private registry.
func (h *ContainerHandler) RegistryLogin(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[container.RegistryAuth](c)
	if err != nil {
		return nil, err
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "登录 "+string(engine)+" 私有仓库 "+req.Server)
	if err := h.containerService.RegistryLogin(c.Request.Context(), engine, req.Server, req.Username, req.Password); err != nil {
		return nil, err
	}
	return gin.H{"message": "登录成功"}, nil
}

// GetLoggedInRegistries lists the registries the engine is logged into.
func (h *ContainerHandler) GetLoggedInRegistries(c *gin.Context) (any, error) {
	regs := h.containerService.GetLoggedInRegistries(c.Request.Context(), h.engineName(c))
	return httpx.Paginate(regs, httpx.ParsePagination(c, 50, 200)), nil
}

// RegistryLogout clears stored credentials for a registry.
func (h *ContainerHandler) RegistryLogout(c *gin.Context) (any, error) {
	server := c.Query("server")
	if server == "" {
		return nil, errx.BadRequest("缺少 server 参数")
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "退出 "+string(engine)+" 私有仓库 "+server)
	if err := h.containerService.RegistryLogout(c.Request.Context(), engine, server); err != nil {
		return nil, err
	}
	return gin.H{"message": "已退出登录"}, nil
}

// ========== Container Management ==========

// ListContainers returns all containers
func (h *ContainerHandler) ListContainers(c *gin.Context) (any, error) {
	all := c.Query("all") == "true"
	containers, err := h.containerService.ListContainers(c.Request.Context(), h.engineName(c), all)
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(containers, httpx.ParsePagination(c, 50, 200)), nil
}

// GetContainer returns a specific container
func (h *ContainerHandler) GetContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	cnt, err := h.containerService.GetContainer(c.Request.Context(), h.engineName(c), id)
	if err != nil {
		return nil, errx.NotFound("容器不存在")
	}
	return cnt, nil
}

// CreateContainer creates a new container
func (h *ContainerHandler) CreateContainer(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[container.CreateRequest](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "创建容器 "+req.Name)
	id, err := h.containerService.CreateContainer(c.Request.Context(), h.engineName(c), req)
	if err != nil {
		return nil, err
	}
	return gin.H{"id": id, "message": "容器已创建"}, nil
}

// StartContainer starts a container
func (h *ContainerHandler) StartContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "启动容器 "+id)
	if err := h.containerService.StartContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已启动"}, nil
}

// StopContainer stops a container
func (h *ContainerHandler) StopContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "停止容器 "+id)
	if err := h.containerService.StopContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已停止"}, nil
}

// RestartContainer restarts a container
func (h *ContainerHandler) RestartContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "重启容器 "+id)
	if err := h.containerService.RestartContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已重启"}, nil
}

// PauseContainer pauses a container
func (h *ContainerHandler) PauseContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "暂停容器 "+id)
	if err := h.containerService.PauseContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已暂停"}, nil
}

// UnpauseContainer unpauses a container
func (h *ContainerHandler) UnpauseContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "恢复容器 "+id)
	if err := h.containerService.UnpauseContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已恢复"}, nil
}

// RemoveContainer removes a container
func (h *ContainerHandler) RemoveContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除容器 "+id)
	if err := h.containerService.RemoveContainer(c.Request.Context(), h.engineName(c), id, force); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已删除"}, nil
}

// GetContainerLogs returns container logs
func (h *ContainerHandler) GetContainerLogs(c *gin.Context) (any, error) {
	id := c.Param("id")
	tail := container.DefaultLogTail
	if t := httpx.QueryInt(c, "tail", 0); t > 0 {
		if t > container.MaxLogTail {
			t = container.MaxLogTail
		}
		tail = t
	}

	logs, err := h.containerService.GetContainerLogs(c.Request.Context(), h.engineName(c), id, tail)
	if err != nil {
		return nil, err
	}
	return gin.H{"logs": logs}, nil
}

// ExecInContainer executes a command in a container
func (h *ContainerHandler) ExecInContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	req, err := httpx.BindJSON[struct {
		Command string `json:"command" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	// Log exec command for audit
	middleware.AuditSummary(c, "容器内执行命令 "+id+": "+req.Command)

	output, err := h.containerService.ExecInContainer(c.Request.Context(), h.engineName(c), id, req.Command)
	if err != nil {
		return nil, err
	}
	return gin.H{"output": output}, nil
}

// GetContainerStats returns real-time container stats
func (h *ContainerHandler) GetContainerStats(c *gin.Context) (any, error) {
	id := c.Param("id")
	stats, err := h.containerService.GetContainerStats(c.Request.Context(), h.engineName(c), id)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// RenameContainer renames a container
func (h *ContainerHandler) RenameContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	req, err := httpx.BindJSON[struct {
		Name string `json:"name" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "重命名容器 "+id+" 为 "+req.Name)
	if err := h.containerService.RenameContainer(c.Request.Context(), h.engineName(c), id, req.Name); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已重命名"}, nil
}

// UpdateContainer updates container resources
func (h *ContainerHandler) UpdateContainer(c *gin.Context) (any, error) {
	id := c.Param("id")
	req, err := httpx.BindJSON[container.UpdateRequest](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新容器 "+id)
	if err := h.containerService.UpdateContainer(c.Request.Context(), h.engineName(c), id, req); err != nil {
		return nil, err
	}
	return gin.H{"message": "容器已更新"}, nil
}

// ========== Image Management ==========

// ListImages returns all images
func (h *ContainerHandler) ListImages(c *gin.Context) (any, error) {
	images, err := h.containerService.ListImages(c.Request.Context(), h.engineName(c))
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(images, httpx.ParsePagination(c, 50, 200)), nil
}

// PullImage pulls an image
func (h *ContainerHandler) PullImage(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Image string `json:"image" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "拉取镜像 "+req.Image)
	if err := h.containerService.PullImage(c.Request.Context(), h.engineName(c), req.Image); err != nil {
		return nil, err
	}
	return gin.H{"message": "镜像已拉取"}, nil
}

// RemoveImage removes an image
func (h *ContainerHandler) RemoveImage(c *gin.Context) (any, error) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除镜像 "+id)
	if err := h.containerService.RemoveImage(c.Request.Context(), h.engineName(c), id, force); err != nil {
		return nil, err
	}
	return gin.H{"message": "镜像已删除"}, nil
}

// ========== Compose Management ==========

// ListComposeProjects lists all compose projects
func (h *ContainerHandler) ListComposeProjects(c *gin.Context) (any, error) {
	projects := h.containerService.ListProjects(c.Request.Context(), h.engineName(c))
	return httpx.Paginate(projects, httpx.ParsePagination(c, 50, 200)), nil
}

// ComposeUp runs compose up -d
func (h *ContainerHandler) ComposeUp(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "启动 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeUp(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		return nil, err
	}
	return gin.H{"message": "Compose 已启动"}, nil
}

// ComposeDown runs compose down
func (h *ContainerHandler) ComposeDown(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "停止 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeDown(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		return nil, err
	}
	return gin.H{"message": "Compose 已停止"}, nil
}

// ComposeRestart runs compose restart
func (h *ContainerHandler) ComposeRestart(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "重启 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeRestart(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		return nil, err
	}
	return gin.H{"message": "Compose 已重启"}, nil
}

// ComposeLogs returns compose project logs
func (h *ContainerHandler) ComposeLogs(c *gin.Context) (any, error) {
	projectDir := c.Query("dir")
	tail := container.DefaultLogTail
	if t := httpx.QueryInt(c, "tail", 0); t > 0 {
		if t > container.MaxLogTail {
			t = container.MaxLogTail
		}
		tail = t
	}

	logs, err := h.containerService.ComposeGetLogs(c.Request.Context(), h.engineName(c), projectDir, tail)
	if err != nil {
		return nil, err
	}
	return gin.H{"logs": logs}, nil
}

// ComposeGetConfig reads docker-compose.yml content
func (h *ContainerHandler) ComposeGetConfig(c *gin.Context) (any, error) {
	projectDir := c.Query("dir")
	content, err := h.containerService.ComposeGetConfig(c.Request.Context(), projectDir)
	if err != nil {
		return nil, err
	}
	return gin.H{"content": content}, nil
}

// ComposeSaveConfig writes docker-compose.yml content
func (h *ContainerHandler) ComposeSaveConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		ProjectDir string `json:"project_dir" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "保存 Compose 配置 "+req.ProjectDir)
	if err := h.containerService.ComposeSaveConfig(c.Request.Context(), req.ProjectDir, req.Content); err != nil {
		return nil, err
	}
	return gin.H{"message": "配置已保存"}, nil
}

// ========== Volume Management ==========

// ListVolumes returns all volumes
func (h *ContainerHandler) ListVolumes(c *gin.Context) (any, error) {
	volumes, err := h.containerService.ListVolumes(c.Request.Context(), h.engineName(c))
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(volumes, httpx.ParsePagination(c, 50, 200)), nil
}

// CreateVolume creates a new volume
func (h *ContainerHandler) CreateVolume(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Name   string            `json:"name" binding:"required"`
		Driver string            `json:"driver"`
		Labels map[string]string `json:"labels"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "创建数据卷 "+req.Name)
	if err := h.containerService.CreateVolume(c.Request.Context(), h.engineName(c), req.Name, req.Driver, req.Labels); err != nil {
		return nil, err
	}
	return gin.H{"message": "卷已创建"}, nil
}

// RemoveVolume removes a volume
func (h *ContainerHandler) RemoveVolume(c *gin.Context) (any, error) {
	name := c.Param("name")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除数据卷 "+name)
	if err := h.containerService.RemoveVolume(c.Request.Context(), h.engineName(c), name, force); err != nil {
		return nil, err
	}
	return gin.H{"message": "卷已删除"}, nil
}

// ========== Network Management ==========

// ListNetworks returns all networks
func (h *ContainerHandler) ListNetworks(c *gin.Context) (any, error) {
	networks, err := h.containerService.ListNetworks(c.Request.Context(), h.engineName(c))
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(networks, httpx.ParsePagination(c, 50, 200)), nil
}

// CreateNetwork creates a new network
func (h *ContainerHandler) CreateNetwork(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "创建网络 "+req.Name)
	if err := h.containerService.CreateNetwork(c.Request.Context(), h.engineName(c), req.Name, req.Driver); err != nil {
		return nil, err
	}
	return gin.H{"message": "网络已创建"}, nil
}

// RemoveNetwork removes a network
func (h *ContainerHandler) RemoveNetwork(c *gin.Context) (any, error) {
	id := c.Param("id")
	middleware.AuditSummary(c, "删除网络 "+id)
	if err := h.containerService.RemoveNetwork(c.Request.Context(), h.engineName(c), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "网络已删除"}, nil
}

func RegisterRoutes(protected *gin.RouterGroup, containerService *container.Service) {
	handler := NewContainerHandler(containerService)

	// Engine management (?engine=docker|podman, default docker)
	protected.GET("/container/status", httpx.H(handler.DetectEngine))
	protected.POST("/container/install", httpx.H(handler.InstallEngine))
	protected.POST("/container/start", httpx.H(handler.StartEngine))
	protected.POST("/container/stop", httpx.H(handler.StopEngine))
	protected.POST("/container/restart", httpx.H(handler.RestartEngine))
	protected.GET("/container/info", httpx.H(handler.GetEngineInfo))
	protected.POST("/container/mirror", httpx.H(handler.ConfigureMirror))
	protected.GET("/container/registry", httpx.H(handler.GetRegistryConfig))
	protected.POST("/container/registry", httpx.H(handler.SetRegistryConfig))
	protected.GET("/container/registry/auth", httpx.H(handler.GetLoggedInRegistries))
	protected.POST("/container/registry/login", httpx.H(handler.RegistryLogin))
	protected.POST("/container/registry/logout", httpx.H(handler.RegistryLogout))
	protected.POST("/container/socket/enable", httpx.H(handler.EnableSocket))

	// Container instances
	protected.GET("/container/instances", httpx.H(handler.ListContainers))
	protected.GET("/container/instances/:id", httpx.H(handler.GetContainer))
	protected.POST("/container/instances", httpx.H(handler.CreateContainer))
	protected.POST("/container/instances/:id/start", httpx.H(handler.StartContainer))
	protected.POST("/container/instances/:id/stop", httpx.H(handler.StopContainer))
	protected.POST("/container/instances/:id/restart", httpx.H(handler.RestartContainer))
	protected.POST("/container/instances/:id/pause", httpx.H(handler.PauseContainer))
	protected.POST("/container/instances/:id/unpause", httpx.H(handler.UnpauseContainer))
	protected.DELETE("/container/instances/:id", httpx.H(handler.RemoveContainer))
	protected.GET("/container/instances/:id/logs", httpx.H(handler.GetContainerLogs))
	protected.POST("/container/instances/:id/exec", httpx.H(handler.ExecInContainer))
	protected.GET("/container/instances/:id/stats", httpx.H(handler.GetContainerStats))
	protected.POST("/container/instances/:id/rename", httpx.H(handler.RenameContainer))
	protected.PUT("/container/instances/:id/update", httpx.H(handler.UpdateContainer))

	// Image management
	protected.GET("/container/images", httpx.H(handler.ListImages))
	protected.POST("/container/images/pull", httpx.H(handler.PullImage))
	protected.DELETE("/container/images/:id", httpx.H(handler.RemoveImage))

	// Compose management
	protected.GET("/container/compose/projects", httpx.H(handler.ListComposeProjects))
	protected.POST("/container/compose/up", httpx.H(handler.ComposeUp))
	protected.POST("/container/compose/down", httpx.H(handler.ComposeDown))
	protected.POST("/container/compose/restart", httpx.H(handler.ComposeRestart))
	protected.GET("/container/compose/logs", httpx.H(handler.ComposeLogs))
	protected.GET("/container/compose/config", httpx.H(handler.ComposeGetConfig))
	protected.PUT("/container/compose/config", httpx.H(handler.ComposeSaveConfig))

	// Volume management
	protected.GET("/container/volumes", httpx.H(handler.ListVolumes))
	protected.POST("/container/volumes", httpx.H(handler.CreateVolume))
	protected.DELETE("/container/volumes/:name", httpx.H(handler.RemoveVolume))

	// Network management
	protected.GET("/container/networks", httpx.H(handler.ListNetworks))
	protected.POST("/container/networks", httpx.H(handler.CreateNetwork))
	protected.DELETE("/container/networks/:id", httpx.H(handler.RemoveNetwork))
}
