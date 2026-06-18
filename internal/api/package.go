package api

import (
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type PackageManagerHandler struct {
	packageService *service.PackageManagerService
	runtimeService *service.RuntimeService
}

func NewPackageManagerHandler(packageService *service.PackageManagerService, runtimeService *service.RuntimeService) *PackageManagerHandler {
	return &PackageManagerHandler{
		packageService: packageService,
		runtimeService: runtimeService,
	}
}

// ListPackages returns all packages for a runtime
func (h *PackageManagerHandler) ListPackages(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		BadRequest(c, "invalid runtime_id")
		return
	}

	packages, err := h.packageService.ListPackages(runtimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"packages": packages,
	})
}

// ScanPackages scans installed packages for a runtime
func (h *PackageManagerHandler) ScanPackages(c *gin.Context) {
	runtimeIDStr := c.Param("id")
	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		BadRequest(c, "invalid runtime id")
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(runtimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	packages, err := h.packageService.ScanPackages(runtimeID, runtime.Name, runtime.Path)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"packages": packages,
	})
}

// InstallPackage installs a package
func (h *PackageManagerHandler) InstallPackage(c *gin.Context) {
	var req model.PackageInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(req.RuntimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	if err := h.packageService.InstallPackage(&req, runtime.Name, runtime.Path); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "package installation started",
	})
}

// UninstallPackage uninstalls a package
func (h *PackageManagerHandler) UninstallPackage(c *gin.Context) {
	var req model.PackageUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(req.RuntimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	if err := h.packageService.UninstallPackage(&req, runtime.Name, runtime.Path); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "package uninstalled successfully",
	})
}

// UpdatePackage updates a package
func (h *PackageManagerHandler) UpdatePackage(c *gin.Context) {
	var req model.PackageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(req.RuntimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	if err := h.packageService.UpdatePackage(&req, runtime.Name, runtime.Path); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "package updated successfully",
	})
}

// SearchPackages searches for available packages
func (h *PackageManagerHandler) SearchPackages(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	query := c.Query("q")

	if runtimeIDStr == "" {
		BadRequest(c, "runtime_id is required")
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		BadRequest(c, "invalid runtime_id")
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(runtimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	packages, err := h.packageService.SearchPackages(runtime.Name, query)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"packages": packages,
	})
}

// GetPackageVersions returns available versions for a package
func (h *PackageManagerHandler) GetPackageVersions(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	packageName := c.Param("name")

	if runtimeIDStr == "" {
		BadRequest(c, "runtime_id is required")
		return
	}

	var runtimeID int64
	if _, err := fmt.Sscanf(runtimeIDStr, "%d", &runtimeID); err != nil {
		BadRequest(c, "invalid runtime_id")
		return
	}

	// Get runtime info
	runtime, err := h.runtimeService.GetByID(runtimeID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if runtime == nil {
		NotFound(c, "runtime not found")
		return
	}

	versions, err := h.packageService.GetPackageVersions(runtime.Name, packageName)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"versions": versions,
	})
}
