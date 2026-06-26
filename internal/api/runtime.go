package api

import (
	"fmt"

	"easyserver/internal/envconfig"
	"easyserver/internal/packagemanager"
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
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	// Validate runtime name
	validRuntimes := map[string]bool{
		"java": true, "node": true, "go": true, "python": true, "php": true,
	}
	if !validRuntimes[req.Name] {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: "+req.Name))
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
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	if err := h.runtimeService.Uninstall(c.Request.Context(), req.Name, req.Version); err != nil {
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
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
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

// Detect detects installed runtime environments on the system
func (h *RuntimeHandler) Detect(c *gin.Context) {
	results, err := h.runtimeService.Detect(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"detected": results,
	})
}

// ImportDetected imports detected runtime environments into the database
func (h *RuntimeHandler) ImportDetected(c *gin.Context) {
	imported, err := h.runtimeService.ImportDetected(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message":  "导入成功",
		"imported": imported,
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

// CheckDependencies checks if all required dependencies are installed
func (h *RuntimeHandler) CheckDependencies(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	// Validate runtime name
	validRuntimes := map[string]bool{
		"java": true, "node": true, "go": true, "python": true, "php": true,
	}
	if !validRuntimes[name] {
		c.Error(ErrBadRequest.WithMessage("不支持的运行时: "+name))
		return
	}

	installed, missing, optional, err := h.runtimeService.CheckDependencies(c.Request.Context(), name)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	allInstalled := len(missing) == 0

	Success(c, gin.H{
		"all_installed": allInstalled,
		"installed":     installed,
		"missing":       missing,
		"optional":      optional,
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

func registerRuntimeRoutes(protected *gin.RouterGroup, runtimeService *runtimeenv.Service, runtimeVersionService *runtimeenv.VersionService, packageService *packagemanager.Service) {
	// Runtime environment management
	runtimeHandler := NewRuntimeHandler(runtimeService)
	protected.GET("/runtime", runtimeHandler.List)
	protected.GET("/runtime/:name", runtimeHandler.ListByName)
	protected.POST("/runtime/install", runtimeHandler.Install)
	protected.POST("/runtime/uninstall", runtimeHandler.Uninstall)
	protected.POST("/runtime/set-default", runtimeHandler.SetDefault)
	protected.GET("/runtime/detect", runtimeHandler.Detect)
	protected.POST("/runtime/import-detected", runtimeHandler.ImportDetected)
	protected.GET("/runtime/progress/:id", runtimeHandler.GetProgress)
	protected.GET("/runtime/check-deps/:name", runtimeHandler.CheckDependencies)
	protected.GET("/runtime/logs/:id", runtimeHandler.GetLogs)
	protected.GET("/runtime/cleanup/:id", runtimeHandler.GetCleanupInfo)

	// Runtime version management
	runtimeVersionHandler := NewRuntimeVersionHandler(runtimeVersionService)
	protected.GET("/runtime-versions/:name", runtimeVersionHandler.List)
	protected.POST("/runtime-versions/:name/fetch", runtimeVersionHandler.Fetch)
	protected.GET("/runtime-versions/:name/resolve/:alias", runtimeVersionHandler.ResolveAlias)
	protected.GET("/runtime-versions/:name/suggestions", runtimeVersionHandler.GetAliasSuggestions)

	// Package management
	packageHandler := NewPackageManagerHandler(packageService, runtimeService)
	protected.GET("/packages", packageHandler.ListPackages)
	protected.GET("/packages/scan/:id", packageHandler.ScanPackages)
	protected.GET("/packages/search", packageHandler.SearchPackages)
	protected.GET("/packages/versions/:name", packageHandler.GetPackageVersions)
	protected.POST("/packages/install", packageHandler.InstallPackage)
	protected.POST("/packages/uninstall", packageHandler.UninstallPackage)
	protected.POST("/packages/update", packageHandler.UpdatePackage)
}
