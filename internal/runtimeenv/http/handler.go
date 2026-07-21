package http

import (
	"fmt"
	"strings"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
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

// ============================================================
// RuntimeHandler — runtime environment management
// ============================================================

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
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"environments": environments,
	})
}

// ListByName returns all versions of a specific runtime environment
func (h *RuntimeHandler) ListByName(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	environments, err := h.runtimeService.ListByName(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"environments": environments,
	})
}

// Install installs a runtime environment
func (h *RuntimeHandler) Install(c *gin.Context) {
	var req runtimeenv.RuntimeInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	middleware.AuditSummary(c, "安装运行环境 "+req.Name+" "+req.Version)
	if err := h.runtimeService.Install(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "安装已启动",
	})
}

// Uninstall uninstalls a runtime environment
func (h *RuntimeHandler) Uninstall(c *gin.Context) {
	var req runtimeenv.RuntimeUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	middleware.AuditSummary(c, "卸载运行环境 "+req.Name+" "+req.Version)
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
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "卸载成功",
	})
}

// SetDefault sets a version as the default for a runtime environment
func (h *RuntimeHandler) SetDefault(c *gin.Context) {
	var req runtimeenv.RuntimeSetDefaultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	middleware.AuditSummary(c, "设置默认运行环境 "+req.Name+" "+req.Version)
	if err := h.runtimeService.SetDefault(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "默认版本设置成功",
	})
}

// GetProgress returns the installation progress for a runtime environment
func (h *RuntimeHandler) GetProgress(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	progress, step, logs, errorMessage, err := h.runtimeService.GetProgress(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
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
		c.Error(apperror.ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if env == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时环境不存在"))
		return
	}

	httpx.Success(c, gin.H{
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
		c.Error(apperror.ErrBadRequest.WithMessage("ID 不能为空"))
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if env == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时环境不存在"))
		return
	}

	httpx.Success(c, gin.H{
		"runtime": env,
	})
}

// GetRemoteVersions gets available remote versions
func (h *RuntimeHandler) GetRemoteVersions(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	if !runtimeenv.IsSupported(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	versions, err := h.runtimeService.GetRemoteVersions(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"versions": versions})
}

// GetCatalog returns the catalog of supported runtimes
func (h *RuntimeHandler) GetCatalog(c *gin.Context) {
	httpx.Success(c, gin.H{
		"catalog": runtimeenv.GetCatalog(),
	})
}

// ============================================================
// PackageManagerHandler — package management for runtimes
// ============================================================

type PackageManagerHandler struct {
	packageService *runtimeenv.PackageService
	runtimeService *runtimeenv.Service
}

func NewPackageManagerHandler(packageService *runtimeenv.PackageService, runtimeService *runtimeenv.Service) *PackageManagerHandler {
	return &PackageManagerHandler{
		packageService: packageService,
		runtimeService: runtimeService,
	}
}

// ListPackages returns all packages for a runtime by scanning the system
// package manager directly. There is no DB cache, so each call reflects the
// current state of the runtime's package manager.
func (h *PackageManagerHandler) ListPackages(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	packages, err := h.packageService.ListPackages(c.Request.Context(), runtimeID, runtime.Name, runtime.Path)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"packages": packages,
	})
}

// InstallPackage installs a package
func (h *PackageManagerHandler) InstallPackage(c *gin.Context) {
	var req runtimeenv.PackageInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	summary := "安装包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 安装 " + req.Name
	}
	if req.Version != "" {
		summary += " (版本: " + req.Version + ")"
	}
	middleware.AuditSummary(c, summary)
	if err := h.packageService.InstallPackage(c.Request.Context(), &req, runtime.Name, runtime.Path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "包安装已启动",
	})
}

// UninstallPackage uninstalls a package
func (h *PackageManagerHandler) UninstallPackage(c *gin.Context) {
	var req runtimeenv.PackageUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	summary := "卸载包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 卸载 " + req.Name
	}
	middleware.AuditSummary(c, summary)
	if err := h.packageService.UninstallPackage(c.Request.Context(), &req, runtime.Name, runtime.Path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "包卸载成功",
	})
}

// UpdatePackage updates a package
func (h *PackageManagerHandler) UpdatePackage(c *gin.Context) {
	var req runtimeenv.PackageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	// Try to get old version before update
	var oldVersion string
	pkgs, err := h.packageService.ListPackages(c.Request.Context(), req.RuntimeID, runtime.Name, runtime.Path)
	if err == nil {
		for _, p := range pkgs {
			if p.Name == req.Name {
				oldVersion = p.Version
				break
			}
		}
	}

	if err := h.packageService.UpdatePackage(c.Request.Context(), &req, runtime.Name, runtime.Path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	// Try to get new version after update
	var newVersion string
	pkgsAfter, err := h.packageService.ListPackages(c.Request.Context(), req.RuntimeID, runtime.Name, runtime.Path)
	if err == nil {
		for _, p := range pkgsAfter {
			if p.Name == req.Name {
				newVersion = p.Version
				break
			}
		}
	}

	summary := "更新包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 更新 " + req.Name
	}
	if oldVersion != "" && newVersion != "" && oldVersion != newVersion {
		summary += " (" + oldVersion + " -> " + newVersion + ")"
	} else if newVersion != "" {
		summary += " (版本: " + newVersion + ")"
	}
	middleware.AuditSummary(c, summary)

	httpx.Success(c, gin.H{
		"message": "包更新成功",
	})
}

// SearchPackages searches for available packages
func (h *PackageManagerHandler) SearchPackages(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	query := c.Query("q")

	if runtimeIDStr == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	packages, err := h.packageService.SearchPackages(c.Request.Context(), runtime.Name, query)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"packages": packages,
	})
}

// GetPackageVersions returns available versions for a package
func (h *PackageManagerHandler) GetPackageVersions(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	packageName := strings.TrimPrefix(c.Param("name"), "/")

	if runtimeIDStr == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	versions, err := h.packageService.GetPackageVersions(c.Request.Context(), runtime.Name, packageName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"versions": versions,
	})
}

// GetRegistry gets the package manager registry
func (h *PackageManagerHandler) GetRegistry(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	manager := c.Query("manager")

	if runtimeIDStr == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	registry, err := h.packageService.GetRegistry(c.Request.Context(), runtime.Name, manager)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"registry": registry,
	})
}

// SetRegistry sets the package manager registry
func (h *PackageManagerHandler) SetRegistry(c *gin.Context) {
	var req struct {
		RuntimeID int64  `json:"runtime_id"`
		Manager   string `json:"manager"`
		Registry  string `json:"registry"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	middleware.AuditSummary(c, "配置包管理器镜像源 "+req.Manager)
	if err := h.packageService.SetRegistry(c.Request.Context(), runtime.Name, req.Manager, req.Registry); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "配置保存成功",
	})
}

// ============================================================
// Route registration
// ============================================================

func RegisterRoutes(protected *gin.RouterGroup, runtimeService *runtimeenv.Service, packageService *runtimeenv.PackageService) {
	// Runtime environment management
	runtimeHandler := NewRuntimeHandler(runtimeService)
	protected.GET("/runtime", runtimeHandler.List)
	protected.GET("/runtime/:name", runtimeHandler.ListByName)
	protected.GET("/runtime/:name/remote-versions", runtimeHandler.GetRemoteVersions)
	protected.POST("/runtime/install", runtimeHandler.Install)
	protected.POST("/runtime/uninstall", runtimeHandler.Uninstall)
	protected.POST("/runtime/set-default", runtimeHandler.SetDefault)
	protected.GET("/runtime/progress/:id", runtimeHandler.GetProgress)
	protected.GET("/runtime/logs/:id", runtimeHandler.GetLogs)
	protected.GET("/runtime/cleanup/:id", runtimeHandler.GetCleanupInfo)
	protected.GET("/runtime/catalog", runtimeHandler.GetCatalog)

	// Package management
	packageHandler := NewPackageManagerHandler(packageService, runtimeService)
	protected.GET("/packages", packageHandler.ListPackages)
	protected.GET("/packages/search", packageHandler.SearchPackages)
	protected.GET("/packages/versions/*name", packageHandler.GetPackageVersions)
	protected.POST("/packages/install", packageHandler.InstallPackage)
	protected.POST("/packages/uninstall", packageHandler.UninstallPackage)
	protected.POST("/packages/update", packageHandler.UpdatePackage)
	protected.GET("/packages/registry", packageHandler.GetRegistry)
	protected.POST("/packages/registry", packageHandler.SetRegistry)
}
