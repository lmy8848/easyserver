package api

import (
	"fmt"
	"strings"

	"easyserver/internal/api/middleware"
	"easyserver/internal/envconfig"
	"easyserver/internal/runtimeenv"

	"github.com/gin-gonic/gin"
)

// getProgressStatus derives a status string from progress state
func getProgressStatus(progress int, step, errorMessage string) string {
	if errorMessage != "" {
		return "failed"
	}
	if progress >= 100 {
		return "completed"
	}
	if step != "" {
		return "running"
	}
	return "pending"
}

type RuntimeHandler struct {
	runtimeService *runtimeenv.Service
}

func NewRuntimeHandler(runtimeService *runtimeenv.Service) *RuntimeHandler {
	return &RuntimeHandler{runtimeService: runtimeService}
}

// List returns all installed runtime environments
func (h *RuntimeHandler) List(c *gin.Context) {
	environments, err := h.runtimeService.ListAll(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"environments": environments,
	})
}

// ListByName returns all versions of a specific runtime environment
func (h *RuntimeHandler) ListByName(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(name) {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	environments, err := h.runtimeService.ListByName(c.Request.Context(), name)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"environments": environments,
	})
}

// Install installs a runtime environment
func (h *RuntimeHandler) Install(c *gin.Context) {
	var req runtimeenv.RuntimeInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	if err := h.runtimeService.Install(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "安装已启动",
	})
}

// Uninstall uninstalls a runtime environment
func (h *RuntimeHandler) Uninstall(c *gin.Context) {
	var req runtimeenv.RuntimeUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	if err := h.runtimeService.Uninstall(c.Request.Context(), req.Name, req.Version); err != nil {
		if strings.Contains(err.Error(), "conflict:") {
			conflictsStr := strings.TrimPrefix(err.Error(), "conflict: ")
			conflicts := strings.Split(conflictsStr, ", ")
			// Use http.StatusConflict (409) manually via c.JSON because ErrorWrapper might be generic
			c.JSON(409, gin.H{
				"code":    409,
				"message": "资源被占用，无法卸载",
				"details": conflicts,
			})
			return
		}
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "卸载成功",
	})
}

// SetDefault sets a version as the default for a runtime environment
func (h *RuntimeHandler) SetDefault(c *gin.Context) {
	var req runtimeenv.RuntimeSetDefaultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	if err := h.runtimeService.SetDefault(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "默认版本设置成功",
	})
}

// GetProgress returns the installation progress for a runtime environment
func (h *RuntimeHandler) GetProgress(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.Error(ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	progress, step, logs, errorMessage, err := h.runtimeService.GetProgress(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"progress":      progress,
		"step":          step,
		"status":        getProgressStatus(progress, step, errorMessage),
		"logs":          logs,
		"error_message": errorMessage,
	})
}

// GetLogs returns the installation logs for a runtime environment
func (h *RuntimeHandler) GetLogs(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.Error(ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if env == nil {
		c.Error(ErrNotFound.WithMessage("运行时环境不存在"))
		return
	}

	Success(c, gin.H{
		"id":            env.ID,
		"name":          env.Name,
		"version":       env.Version,
		"status":        env.Status,
		"progress":      env.Progress,
		"progress_step": env.ProgressStep,
		"logs":          env.Logs,
		"error_message": env.ErrorMessage,
	})
}

// GetCleanupInfo returns what will be cleaned up when uninstalling
func (h *RuntimeHandler) GetCleanupInfo(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.Error(ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if env == nil {
		c.Error(ErrNotFound.WithMessage("运行时环境不存在"))
		return
	}

	// Get related environment variables
	envConfigs, err := h.runtimeService.GetEnvConfigsByRuntimeID(c.Request.Context(), id)
	if err != nil {
		envConfigs = []envconfig.EnvConfig{}
	}

	// Get related PATH entries
	pathEntries, err := h.runtimeService.GetPathEntriesByRuntimeID(c.Request.Context(), id)
	if err != nil {
		pathEntries = []envconfig.PathEntry{}
	}

	Success(c, gin.H{
		"runtime":      env,
		"env_configs":  envConfigs,
		"path_entries": pathEntries,
		"will_cleanup": gin.H{
			"env_configs_count":  len(envConfigs),
			"path_entries_count": len(pathEntries),
		},
	})
}

// ListMirrors
func (h *RuntimeHandler) ListMirrors(c *gin.Context) {
	mirrors, err := h.runtimeService.ListMirrors(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"mirrors": mirrors})
}

// UpdateMirror
func (h *RuntimeHandler) UpdateMirror(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	var req runtimeenv.RuntimeMirrorUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求"))
		return
	}

	if err := h.runtimeService.UpdateMirror(c.Request.Context(), &req, id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "修改成功"})
}

// CreateMirror
func (h *RuntimeHandler) CreateMirror(c *gin.Context) {
	var req runtimeenv.RuntimeMirrorCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	id, err := h.runtimeService.CreateMirror(c.Request.Context(), &req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"id": id, "message": "创建成功"})
}

// DeleteMirror
func (h *RuntimeHandler) DeleteMirror(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	if err := h.runtimeService.DeleteMirror(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "删除成功"})
}

// GetRemoteVersions gets available remote versions
func (h *RuntimeHandler) GetRemoteVersions(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	if !runtimeenv.IsSupported(name) {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	versions, err := h.runtimeService.GetRemoteVersions(c.Request.Context(), name)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"versions": versions})
}

func registerRuntimeRoutes(protected *gin.RouterGroup, runtimeService *runtimeenv.Service, packageService *runtimeenv.PackageService) {
	// Runtime environment management
	runtimeHandler := NewRuntimeHandler(runtimeService)
	protected.GET("/runtime", runtimeHandler.List)
	protected.GET("/runtime/:name", runtimeHandler.ListByName)
	protected.GET("/runtime/:name/remote-versions", runtimeHandler.GetRemoteVersions)
	protected.POST("/runtime/install", middleware.SetAction("RUNTIME_INSTALL"), runtimeHandler.Install)
	protected.POST("/runtime/uninstall", middleware.SetAction("RUNTIME_UNINSTALL"), runtimeHandler.Uninstall)
	protected.POST("/runtime/set-default", middleware.SetAction("RUNTIME_SET_DEFAULT"), runtimeHandler.SetDefault)
	protected.GET("/runtime/progress/:id", runtimeHandler.GetProgress)
	protected.GET("/runtime/logs/:id", runtimeHandler.GetLogs)
	protected.GET("/runtime/cleanup/:id", runtimeHandler.GetCleanupInfo)
	protected.GET("/runtime/catalog", runtimeHandler.GetCatalog)
	protected.GET("/runtime/mirrors", runtimeHandler.ListMirrors)
	protected.PUT("/runtime/mirrors/:id", middleware.SetAction("RUNTIME_UPDATE_MIRROR"), runtimeHandler.UpdateMirror)
	protected.POST("/runtime/mirrors", middleware.SetAction("RUNTIME_CREATE_MIRROR"), runtimeHandler.CreateMirror)
	protected.DELETE("/runtime/mirrors/:id", middleware.SetAction("RUNTIME_DELETE_MIRROR"), runtimeHandler.DeleteMirror)

	// Package management
	packageHandler := NewPackageManagerHandler(packageService, runtimeService)
	protected.GET("/packages", packageHandler.ListPackages)
	protected.GET("/packages/search", packageHandler.SearchPackages)
	protected.GET("/packages/versions/*name", packageHandler.GetPackageVersions)
	protected.POST("/packages/install", middleware.SetAction("PACKAGE_INSTALL"), packageHandler.InstallPackage)
	protected.POST("/packages/uninstall", middleware.SetAction("PACKAGE_UNINSTALL"), packageHandler.UninstallPackage)
	protected.POST("/packages/update", middleware.SetAction("PACKAGE_UPDATE"), packageHandler.UpdatePackage)
	protected.GET("/packages/registry", packageHandler.GetRegistry)
	protected.POST("/packages/registry", middleware.SetAction("PACKAGE_SET_REGISTRY"), packageHandler.SetRegistry)
}

// GetCatalog returns the catalog of supported runtimes
func (h *RuntimeHandler) GetCatalog(c *gin.Context) {
	Success(c, gin.H{
		"catalog": runtimeenv.GetCatalog(),
	})
}
