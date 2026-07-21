package api

import (
	"fmt"
	"strconv"

	"easyserver/internal/envconfig"
	"easyserver/internal/httpx/middleware"

	"github.com/gin-gonic/gin"
)

type EnvConfigHandler struct {
	envConfigService *envconfig.Service
}

func NewEnvConfigHandler(envConfigService *envconfig.Service) *EnvConfigHandler {
	return &EnvConfigHandler{envConfigService: envConfigService}
}

// ListEnvConfigs returns all environment configurations
func (h *EnvConfigHandler) ListEnvConfigs(c *gin.Context) {
	configs, err := h.envConfigService.ListEnvConfigs(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"configs": configs,
	})
}

// GetEnvConfig returns a specific environment configuration
func (h *EnvConfigHandler) GetEnvConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	config, err := h.envConfigService.GetEnvConfig(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if config == nil {
		c.Error(ErrNotFound.WithMessage("配置不存在"))
		return
	}

	Success(c, config)
}

// CreateEnvConfig creates a new environment configuration
func (h *EnvConfigHandler) CreateEnvConfig(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Value   string `json:"value" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	middleware.AuditSummary(c, "创建环境变量 "+req.Name)

	config := &envconfig.EnvConfig{
		Name:    req.Name,
		Value:   req.Value,
		Enabled: req.Enabled,
	}

	if err := h.envConfigService.CreateEnvConfig(c.Request.Context(), config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, config)
}

// UpdateEnvConfig updates an environment configuration
func (h *EnvConfigHandler) UpdateEnvConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Name    string `json:"name" binding:"required"`
		Value   string `json:"value" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	middleware.AuditSummary(c, "更新环境变量 "+req.Name)

	config, err := h.envConfigService.GetEnvConfig(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if config == nil {
		c.Error(ErrNotFound.WithMessage("配置不存在"))
		return
	}

	config.Name = req.Name
	config.Value = req.Value
	config.Enabled = req.Enabled

	if err := h.envConfigService.UpdateEnvConfig(c.Request.Context(), config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, config)
}

// DeleteEnvConfig deletes an environment configuration
func (h *EnvConfigHandler) DeleteEnvConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	middleware.AuditSummary(c, "删除环境变量 #"+strconv.FormatInt(id, 10))

	if err := h.envConfigService.DeleteEnvConfig(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "删除成功"})
}

// ListPathEntries returns all PATH entries
func (h *EnvConfigHandler) ListPathEntries(c *gin.Context) {
	entries, err := h.envConfigService.ListPathEntries(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"entries": entries,
	})
}

// CreatePathEntry creates a new PATH entry
func (h *EnvConfigHandler) CreatePathEntry(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	middleware.AuditSummary(c, "添加 PATH 条目 "+req.Path)

	entry := &envconfig.PathEntry{
		Path:    req.Path,
		Enabled: req.Enabled,
	}

	if err := h.envConfigService.CreatePathEntry(c.Request.Context(), entry); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, entry)
}

// UpdatePathEntry updates an existing PATH entry
func (h *EnvConfigHandler) UpdatePathEntry(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Path    string `json:"path" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// For order, we might need to get it first if it's missing, but here let's assume
	// we just keep order as is if we don't pass it, actually repo.UpdatePathEntry needs Order
	// Let's retrieve existing to keep order intact.
	existingPaths, err := h.envConfigService.ListPathEntries(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	var existing *envconfig.PathEntry
	for _, p := range existingPaths {
		if p.ID == id {
			existing = &p
			break
		}
	}
	if existing == nil {
		c.Error(ErrNotFound.WithMessage("PATH 条目不存在"))
		return
	}

	existing.Path = req.Path
	existing.Enabled = req.Enabled

	if err := h.envConfigService.UpdatePathEntry(c.Request.Context(), existing); err != nil {
		c.Error(err)
		return
	}
	Success(c, nil)
}

// DeletePathEntry deletes a PATH entry
func (h *EnvConfigHandler) DeletePathEntry(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	middleware.AuditSummary(c, "删除 PATH 条目 #"+strconv.FormatInt(id, 10))

	if err := h.envConfigService.DeletePathEntry(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "删除成功"})
}

// GenerateEnvScript generates a shell script to set environment variables
func (h *EnvConfigHandler) GenerateEnvScript(c *gin.Context) {
	script, err := h.envConfigService.GenerateEnvScript(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"script": script,
	})
}
