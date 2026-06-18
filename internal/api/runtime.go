package api

import (
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type RuntimeHandler struct {
	runtimeService *service.RuntimeService
}

func NewRuntimeHandler(runtimeService *service.RuntimeService) *RuntimeHandler {
	return &RuntimeHandler{runtimeService: runtimeService}
}

// List returns all installed runtime environments
func (h *RuntimeHandler) List(c *gin.Context) {
	environments, err := h.runtimeService.ListAll()
	if err != nil {
		InternalError(c, err.Error())
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
		BadRequest(c, "runtime name is required")
		return
	}

	environments, err := h.runtimeService.ListByName(name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"environments": environments,
	})
}

// Install installs a runtime environment
func (h *RuntimeHandler) Install(c *gin.Context) {
	var req model.RuntimeInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// Validate runtime name
	validRuntimes := map[string]bool{
		"java": true, "node": true, "go": true, "python": true, "php": true,
	}
	if !validRuntimes[req.Name] {
		BadRequest(c, "unsupported runtime: "+req.Name)
		return
	}

	if err := h.runtimeService.Install(req.Name, req.Version); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "installation started",
	})
}

// Uninstall uninstalls a runtime environment
func (h *RuntimeHandler) Uninstall(c *gin.Context) {
	var req model.RuntimeUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if err := h.runtimeService.Uninstall(req.Name, req.Version); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "uninstalled successfully",
	})
}

// SetDefault sets a version as the default for a runtime environment
func (h *RuntimeHandler) SetDefault(c *gin.Context) {
	var req model.RuntimeSetDefaultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	if err := h.runtimeService.SetDefault(req.Name, req.Version); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message": "default version set successfully",
	})
}

// Detect detects installed runtime environments on the system
func (h *RuntimeHandler) Detect(c *gin.Context) {
	results, err := h.runtimeService.Detect()
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"detected": results,
	})
}

// ImportDetected imports detected runtime environments into the database
func (h *RuntimeHandler) ImportDetected(c *gin.Context) {
	imported, err := h.runtimeService.ImportDetected()
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message":  "imported successfully",
		"imported": imported,
	})
}

// GetProgress returns the installation progress for a runtime environment
func (h *RuntimeHandler) GetProgress(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		BadRequest(c, "id is required")
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		BadRequest(c, "invalid id")
		return
	}

	progress, step, logs, errorMessage, err := h.runtimeService.GetProgress(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"progress":       progress,
		"step":           step,
		"logs":           logs,
		"error_message":  errorMessage,
	})
}

// CheckDependencies checks if all required dependencies are installed
func (h *RuntimeHandler) CheckDependencies(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "runtime name is required")
		return
	}

	// Validate runtime name
	validRuntimes := map[string]bool{
		"java": true, "node": true, "go": true, "python": true, "php": true,
	}
	if !validRuntimes[name] {
		BadRequest(c, "unsupported runtime: "+name)
		return
	}

	installed, missing, optional, err := h.runtimeService.CheckDependencies(name)
	if err != nil {
		InternalError(c, err.Error())
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
		BadRequest(c, "id is required")
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		BadRequest(c, "invalid id")
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if env == nil {
		NotFound(c, "runtime environment not found")
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
		BadRequest(c, "id is required")
		return
	}

	// Parse id
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		BadRequest(c, "invalid id")
		return
	}

	// Get the environment info
	env, err := h.runtimeService.GetByID(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if env == nil {
		NotFound(c, "runtime environment not found")
		return
	}

	// Get related environment variables
	envConfigs, err := h.runtimeService.GetEnvConfigsByRuntimeID(id)
	if err != nil {
		envConfigs = []model.EnvConfig{}
	}

	// Get related PATH entries
	pathEntries, err := h.runtimeService.GetPathEntriesByRuntimeID(id)
	if err != nil {
		pathEntries = []model.PathEntry{}
	}

	Success(c, gin.H{
		"runtime": env,
		"env_configs": envConfigs,
		"path_entries": pathEntries,
		"will_cleanup": gin.H{
			"env_configs_count": len(envConfigs),
			"path_entries_count": len(pathEntries),
		},
	})
}
