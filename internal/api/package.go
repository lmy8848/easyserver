package api

import (
	"fmt"
	"strings"

	"easyserver/internal/api/middleware"
	"easyserver/internal/runtimeenv"

	"github.com/gin-gonic/gin"
)

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
		c.Error(ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	packages, err := h.packageService.ListPackages(c.Request.Context(), runtimeID, runtime.Name, runtime.Path)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"packages": packages,
	})
}

// InstallPackage installs a package
func (h *PackageManagerHandler) InstallPackage(c *gin.Context) {
	var req runtimeenv.PackageInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
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
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "包安装已启动",
	})
}

// UninstallPackage uninstalls a package
func (h *PackageManagerHandler) UninstallPackage(c *gin.Context) {
	var req runtimeenv.PackageUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	summary := "卸载包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 卸载 " + req.Name
	}
	middleware.AuditSummary(c, summary)
	if err := h.packageService.UninstallPackage(c.Request.Context(), &req, runtime.Name, runtime.Path); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "包卸载成功",
	})
}

// UpdatePackage updates a package
func (h *PackageManagerHandler) UpdatePackage(c *gin.Context) {
	var req runtimeenv.PackageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
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
		c.Error(WrapError(err))
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

	Success(c, gin.H{
		"message": "包更新成功",
	})
}

// SearchPackages searches for available packages
func (h *PackageManagerHandler) SearchPackages(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	query := c.Query("q")

	if runtimeIDStr == "" {
		c.Error(ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	packages, err := h.packageService.SearchPackages(c.Request.Context(), runtime.Name, query)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"packages": packages,
	})
}

// GetPackageVersions returns available versions for a package
func (h *PackageManagerHandler) GetPackageVersions(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	packageName := strings.TrimPrefix(c.Param("name"), "/")

	if runtimeIDStr == "" {
		c.Error(ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	if !runtimeenv.SupportsGlobalPkgsFor(runtime.Name) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", runtime.Name)))
		return
	}

	versions, err := h.packageService.GetPackageVersions(c.Request.Context(), runtime.Name, packageName)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"versions": versions,
	})
}

// GetRegistry gets the package manager registry
func (h *PackageManagerHandler) GetRegistry(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	manager := c.Query("manager")

	if runtimeIDStr == "" {
		c.Error(ErrBadRequest.WithMessage("runtime_id 不能为空"))
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 runtime_id"))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	registry, err := h.packageService.GetRegistry(c.Request.Context(), runtime.Name, manager)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
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
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(c.Request.Context(), req.RuntimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if runtime == nil {
		c.Error(ErrNotFound.WithMessage("运行时不存在"))
		return
	}

	middleware.AuditSummary(c, "配置包管理器镜像源 "+req.Manager)
	if err := h.packageService.SetRegistry(c.Request.Context(), runtime.Name, req.Manager, req.Registry); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"message": "配置保存成功",
	})
}
