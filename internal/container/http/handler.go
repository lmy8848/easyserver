package http

import (
	"easyserver/internal/container"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
	"strconv"
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
func (h *ContainerHandler) DetectEngine(c *gin.Context) {
	status, err := h.containerService.Detect(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, status)
}

// InstallEngine installs a engine
func (h *ContainerHandler) InstallEngine(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "安装 "+string(engine))
	if err := h.containerService.Install(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": string(engine) + " 安装成功"})
}

// StartEngine starts a engine's service
func (h *ContainerHandler) StartEngine(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "启动 "+string(engine))
	if err := h.containerService.StartEngine(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": string(engine) + " 已启动"})
}

// StopEngine stops a engine's service
func (h *ContainerHandler) StopEngine(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "停止 "+string(engine))
	if err := h.containerService.StopEngine(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": string(engine) + " 已停止"})
}

// RestartEngine restarts a engine's service
func (h *ContainerHandler) RestartEngine(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "重启 "+string(engine))
	if err := h.containerService.RestartEngine(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": string(engine) + " 已重启"})
}

// EnableSocket enables a engine's API socket unit (e.g. podman.socket).
func (h *ContainerHandler) EnableSocket(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "启用 "+string(engine)+" Socket")
	if err := h.containerService.EnableSocket(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "Socket 已启用"})
}

// DisableSocket disables a engine's API socket unit.
func (h *ContainerHandler) DisableSocket(c *gin.Context) {
	engine := h.engineName(c)
	middleware.AuditSummary(c, "禁用 "+string(engine)+" Socket")
	if err := h.containerService.DisableSocket(c.Request.Context(), engine); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "Socket 已禁用"})
}

// GetEngineInfo returns a engine's system info
func (h *ContainerHandler) GetEngineInfo(c *gin.Context) {
	info, err := h.containerService.GetInfo(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, info)
}

// ConfigureMirror configures a engine's registry mirror
func (h *ContainerHandler) ConfigureMirror(c *gin.Context) {
	var req struct {
		MirrorURL string `json:"mirror_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	engine := h.engineName(c)
	middleware.AuditSummary(c, "配置 "+string(engine)+" 镜像源 "+req.MirrorURL)
	if err := h.containerService.ConfigureMirror(c.Request.Context(), engine, req.MirrorURL); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "镜像源已配置"})
}

// GetRegistryConfig returns the engine's mirror + insecure registries.
func (h *ContainerHandler) GetRegistryConfig(c *gin.Context) {
	cfg, err := h.containerService.GetRegistryConfig(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, cfg)
}

// SetRegistryConfig persists the engine's mirror + insecure registries.
func (h *ContainerHandler) SetRegistryConfig(c *gin.Context) {
	var req container.RegistryConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "配置 "+string(engine)+" 镜像仓库 "+req.Mirror)
	if err := h.containerService.SetRegistryConfig(c.Request.Context(), engine, req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "镜像仓库配置已保存"})
}

// RegistryLogin logs into a private registry.
func (h *ContainerHandler) RegistryLogin(c *gin.Context) {
	var req container.RegistryAuth
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "登录 "+string(engine)+" 私有仓库 "+req.Server)
	if err := h.containerService.RegistryLogin(c.Request.Context(), engine, req.Server, req.Username, req.Password); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "登录成功"})
}

// RegistryLogout clears stored credentials for a registry.
func (h *ContainerHandler) RegistryLogout(c *gin.Context) {
	server := c.Query("server")
	if server == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少 server 参数"))
		return
	}
	engine := h.engineName(c)
	middleware.AuditSummary(c, "退出 "+string(engine)+" 私有仓库 "+server)
	if err := h.containerService.RegistryLogout(c.Request.Context(), engine, server); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已退出登录"})
}

// ========== Container Management ==========

// ListContainers returns all containers
func (h *ContainerHandler) ListContainers(c *gin.Context) {
	all := c.Query("all") == "true"
	containers, err := h.containerService.ListContainers(c.Request.Context(), h.engineName(c), all)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"containers": containers})
}

// GetContainer returns a specific container
func (h *ContainerHandler) GetContainer(c *gin.Context) {
	id := c.Param("id")
	container, err := h.containerService.GetContainer(c.Request.Context(), h.engineName(c), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("容器不存在"))
		return
	}
	httpx.Success(c, container)
}

// CreateContainer creates a new container
func (h *ContainerHandler) CreateContainer(c *gin.Context) {
	var req container.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "创建容器 "+req.Name)
	id, err := h.containerService.CreateContainer(c.Request.Context(), h.engineName(c), req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"id": id, "message": "容器已创建"})
}

// StartContainer starts a container
func (h *ContainerHandler) StartContainer(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "启动容器 "+id)
	if err := h.containerService.StartContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已启动"})
}

// StopContainer stops a container
func (h *ContainerHandler) StopContainer(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "停止容器 "+id)
	if err := h.containerService.StopContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已停止"})
}

// RestartContainer restarts a container
func (h *ContainerHandler) RestartContainer(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "重启容器 "+id)
	if err := h.containerService.RestartContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已重启"})
}

// PauseContainer pauses a container
func (h *ContainerHandler) PauseContainer(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "暂停容器 "+id)
	if err := h.containerService.PauseContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已暂停"})
}

// UnpauseContainer unpauses a container
func (h *ContainerHandler) UnpauseContainer(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "恢复容器 "+id)
	if err := h.containerService.UnpauseContainer(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已恢复"})
}

// RemoveContainer removes a container
func (h *ContainerHandler) RemoveContainer(c *gin.Context) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除容器 "+id)
	if err := h.containerService.RemoveContainer(c.Request.Context(), h.engineName(c), id, force); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已删除"})
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

	logs, err := h.containerService.GetContainerLogs(c.Request.Context(), h.engineName(c), id, tail)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"logs": logs})
}

// ExecInContainer executes a command in a container
func (h *ContainerHandler) ExecInContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Log exec command for audit
	middleware.AuditSummary(c, "容器内执行命令 "+id+": "+req.Command)

	output, err := h.containerService.ExecInContainer(c.Request.Context(), h.engineName(c), id, req.Command)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"output": output})
}

// GetContainerStats returns real-time container stats
func (h *ContainerHandler) GetContainerStats(c *gin.Context) {
	id := c.Param("id")
	stats, err := h.containerService.GetContainerStats(c.Request.Context(), h.engineName(c), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, stats)
}

// GetContainerTop returns processes running in a container
func (h *ContainerHandler) GetContainerTop(c *gin.Context) {
	id := c.Param("id")
	processes, err := h.containerService.GetContainerTop(c.Request.Context(), h.engineName(c), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"processes": processes})
}

// CopyToContainer copies a file to a container
func (h *ContainerHandler) CopyToContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "复制文件到容器 "+id+": "+req.SrcPath+" -> "+req.DestPath)
	if err := h.containerService.CopyToContainer(c.Request.Context(), h.engineName(c), id, req.SrcPath, req.DestPath); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "文件已复制到容器"})
}

// CopyFromContainer copies a file from a container
func (h *ContainerHandler) CopyFromContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "从容器复制文件 "+id+": "+req.SrcPath+" -> "+req.DestPath)
	if err := h.containerService.CopyFromContainer(c.Request.Context(), h.engineName(c), id, req.SrcPath, req.DestPath); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "文件已从容器复制"})
}

// RenameContainer renames a container
func (h *ContainerHandler) RenameContainer(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "重命名容器 "+id+" 为 "+req.Name)
	if err := h.containerService.RenameContainer(c.Request.Context(), h.engineName(c), id, req.Name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已重命名"})
}

// UpdateContainer updates container resources
func (h *ContainerHandler) UpdateContainer(c *gin.Context) {
	id := c.Param("id")
	var req container.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "更新容器 "+id)
	if err := h.containerService.UpdateContainer(c.Request.Context(), h.engineName(c), id, req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "容器已更新"})
}

// ========== Image Management ==========

// ListImages returns all images
func (h *ContainerHandler) ListImages(c *gin.Context) {
	images, err := h.containerService.ListImages(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"images": images})
}

// PullImage pulls an image
func (h *ContainerHandler) PullImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "拉取镜像 "+req.Image)
	if err := h.containerService.PullImage(c.Request.Context(), h.engineName(c), req.Image); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "镜像已拉取"})
}

// RemoveImage removes an image
func (h *ContainerHandler) RemoveImage(c *gin.Context) {
	id := c.Param("id")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除镜像 "+id)
	if err := h.containerService.RemoveImage(c.Request.Context(), h.engineName(c), id, force); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "镜像已删除"})
}

// ========== Compose Management ==========

// ListComposeProjects lists all compose projects
func (h *ContainerHandler) ListComposeProjects(c *gin.Context) {
	projects, err := h.containerService.ListProjects(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"projects": projects})
}

// ComposeUp runs compose up -d
func (h *ContainerHandler) ComposeUp(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "启动 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeUp(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "Compose 已启动"})
}

// ComposeDown runs compose down
func (h *ContainerHandler) ComposeDown(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "停止 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeDown(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "Compose 已停止"})
}

// ComposeRestart runs compose restart
func (h *ContainerHandler) ComposeRestart(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "重启 Compose "+req.ProjectDir)
	if err := h.containerService.ComposeRestart(c.Request.Context(), h.engineName(c), req.ProjectDir); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "Compose 已重启"})
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

	logs, err := h.containerService.ComposeGetLogs(c.Request.Context(), h.engineName(c), projectDir, tail)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"logs": logs})
}

// ComposeGetConfig reads docker-compose.yml content
func (h *ContainerHandler) ComposeGetConfig(c *gin.Context) {
	projectDir := c.Query("dir")
	content, err := h.containerService.ComposeGetConfig(c.Request.Context(), projectDir)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"content": content})
}

// ComposeSaveConfig writes docker-compose.yml content
func (h *ContainerHandler) ComposeSaveConfig(c *gin.Context) {
	var req struct {
		ProjectDir string `json:"project_dir" binding:"required"`
		Content    string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "保存 Compose 配置 "+req.ProjectDir)
	if err := h.containerService.ComposeSaveConfig(c.Request.Context(), req.ProjectDir, req.Content); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "配置已保存"})
}

// ========== Volume Management ==========

// ListVolumes returns all volumes
func (h *ContainerHandler) ListVolumes(c *gin.Context) {
	volumes, err := h.containerService.ListVolumes(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"volumes": volumes})
}

// CreateVolume creates a new volume
func (h *ContainerHandler) CreateVolume(c *gin.Context) {
	var req struct {
		Name   string            `json:"name" binding:"required"`
		Driver string            `json:"driver"`
		Labels map[string]string `json:"labels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "创建数据卷 "+req.Name)
	if err := h.containerService.CreateVolume(c.Request.Context(), h.engineName(c), req.Name, req.Driver, req.Labels); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "卷已创建"})
}

// RemoveVolume removes a volume
func (h *ContainerHandler) RemoveVolume(c *gin.Context) {
	name := c.Param("name")
	force := c.Query("force") == "true"
	middleware.AuditSummary(c, "删除数据卷 "+name)
	if err := h.containerService.RemoveVolume(c.Request.Context(), h.engineName(c), name, force); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "卷已删除"})
}

// ========== Network Management ==========

// ListNetworks returns all networks
func (h *ContainerHandler) ListNetworks(c *gin.Context) {
	networks, err := h.containerService.ListNetworks(c.Request.Context(), h.engineName(c))
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"networks": networks})
}

// CreateNetwork creates a new network
func (h *ContainerHandler) CreateNetwork(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "创建网络 "+req.Name)
	if err := h.containerService.CreateNetwork(c.Request.Context(), h.engineName(c), req.Name, req.Driver); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "网络已创建"})
}

// RemoveNetwork removes a network
func (h *ContainerHandler) RemoveNetwork(c *gin.Context) {
	id := c.Param("id")
	middleware.AuditSummary(c, "删除网络 "+id)
	if err := h.containerService.RemoveNetwork(c.Request.Context(), h.engineName(c), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "网络已删除"})
}

func RegisterRoutes(protected *gin.RouterGroup, containerService *container.Service) {
	handler := NewContainerHandler(containerService)

	// Engine management (?engine=docker|podman, default docker)
	protected.GET("/container/status", handler.DetectEngine)
	protected.POST("/container/install", handler.InstallEngine)
	protected.POST("/container/start", handler.StartEngine)
	protected.POST("/container/stop", handler.StopEngine)
	protected.POST("/container/restart", handler.RestartEngine)
	protected.GET("/container/info", handler.GetEngineInfo)
	protected.POST("/container/mirror", handler.ConfigureMirror)
	protected.GET("/container/registry", handler.GetRegistryConfig)
	protected.POST("/container/registry", handler.SetRegistryConfig)
	protected.POST("/container/registry/login", handler.RegistryLogin)
	protected.POST("/container/registry/logout", handler.RegistryLogout)
	protected.POST("/container/socket/enable", handler.EnableSocket)
	protected.POST("/container/socket/disable", handler.DisableSocket)

	// Container instances
	protected.GET("/container/instances", handler.ListContainers)
	protected.GET("/container/instances/:id", handler.GetContainer)
	protected.POST("/container/instances", handler.CreateContainer)
	protected.POST("/container/instances/:id/start", handler.StartContainer)
	protected.POST("/container/instances/:id/stop", handler.StopContainer)
	protected.POST("/container/instances/:id/restart", handler.RestartContainer)
	protected.POST("/container/instances/:id/pause", handler.PauseContainer)
	protected.POST("/container/instances/:id/unpause", handler.UnpauseContainer)
	protected.DELETE("/container/instances/:id", handler.RemoveContainer)
	protected.GET("/container/instances/:id/logs", handler.GetContainerLogs)
	protected.POST("/container/instances/:id/exec", handler.ExecInContainer)
	protected.GET("/container/instances/:id/stats", handler.GetContainerStats)
	protected.GET("/container/instances/:id/top", handler.GetContainerTop)
	protected.POST("/container/instances/:id/copy-to", handler.CopyToContainer)
	protected.POST("/container/instances/:id/copy-from", handler.CopyFromContainer)
	protected.POST("/container/instances/:id/rename", handler.RenameContainer)
	protected.PUT("/container/instances/:id/update", handler.UpdateContainer)

	// Image management
	protected.GET("/container/images", handler.ListImages)
	protected.POST("/container/images/pull", handler.PullImage)
	protected.DELETE("/container/images/:id", handler.RemoveImage)

	// Compose management
	protected.GET("/container/compose/projects", handler.ListComposeProjects)
	protected.POST("/container/compose/up", handler.ComposeUp)
	protected.POST("/container/compose/down", handler.ComposeDown)
	protected.POST("/container/compose/restart", handler.ComposeRestart)
	protected.GET("/container/compose/logs", handler.ComposeLogs)
	protected.GET("/container/compose/config", handler.ComposeGetConfig)
	protected.PUT("/container/compose/config", handler.ComposeSaveConfig)

	// Volume management
	protected.GET("/container/volumes", handler.ListVolumes)
	protected.POST("/container/volumes", handler.CreateVolume)
	protected.DELETE("/container/volumes/:name", handler.RemoveVolume)

	// Network management
	protected.GET("/container/networks", handler.ListNetworks)
	protected.POST("/container/networks", handler.CreateNetwork)
	protected.DELETE("/container/networks/:id", handler.RemoveNetwork)
}
