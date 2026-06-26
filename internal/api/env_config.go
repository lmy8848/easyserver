package api

import (
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type EnvConfigHandler struct {
	envConfigService *service.EnvConfigService
}

func NewEnvConfigHandler(envConfigService *service.EnvConfigService) *EnvConfigHandler {
	return &EnvConfigHandler{envConfigService: envConfigService}
}

// ListEnvConfigs returns all environment configurations
func (h *EnvConfigHandler) ListEnvConfigs(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	var runtimeID int64
	if runtimeIDStr != "" {
		fmt.Sscanf(runtimeIDStr, "%d", &runtimeID)
	}

	configs, err := h.envConfigService.ListEnvConfigs(c.Request.Context(), runtimeID)
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
		Name      string `json:"name" binding:"required"`
		Value     string `json:"value" binding:"required"`
		RuntimeID int64  `json:"runtime_id"`
		IsGlobal  bool   `json:"is_global"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	config := &model.EnvConfig{
		Name:      req.Name,
		Value:     req.Value,
		RuntimeID: req.RuntimeID,
		IsGlobal:  req.IsGlobal,
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
		Name  string `json:"name" binding:"required"`
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
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

	config.Name = req.Name
	config.Value = req.Value

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

	if err := h.envConfigService.DeleteEnvConfig(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "删除成功"})
}

// ListPathEntries returns all PATH entries
func (h *EnvConfigHandler) ListPathEntries(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	var runtimeID int64
	if runtimeIDStr != "" {
		fmt.Sscanf(runtimeIDStr, "%d", &runtimeID)
	}

	entries, err := h.envConfigService.ListPathEntries(c.Request.Context(), runtimeID)
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
		Path      string `json:"path" binding:"required"`
		RuntimeID int64  `json:"runtime_id"`
		IsGlobal  bool   `json:"is_global"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	entry := &model.PathEntry{
		Path:      req.Path,
		RuntimeID: req.RuntimeID,
		IsGlobal:  req.IsGlobal,
	}

	if err := h.envConfigService.CreatePathEntry(c.Request.Context(), entry); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, entry)
}

// DeletePathEntry deletes a PATH entry
func (h *EnvConfigHandler) DeletePathEntry(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.envConfigService.DeletePathEntry(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "删除成功"})
}

// GenerateEnvScript generates a shell script to set environment variables
func (h *EnvConfigHandler) GenerateEnvScript(c *gin.Context) {
	runtimeIDStr := c.Query("runtime_id")
	var runtimeID int64
	if runtimeIDStr != "" {
		fmt.Sscanf(runtimeIDStr, "%d", &runtimeID)
	}

	script, err := h.envConfigService.GenerateEnvScript(c.Request.Context(), runtimeID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"script": script,
	})
}

// ListGlobalConfigs returns all global configurations
func (h *EnvConfigHandler) ListGlobalConfigs(c *gin.Context) {
	category := c.Query("category")

	configs, err := h.envConfigService.ListGlobalConfigs(c.Request.Context(), category)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"configs": configs,
	})
}

// GetGlobalConfig returns a specific global configuration
func (h *EnvConfigHandler) GetGlobalConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	config, err := h.envConfigService.GetGlobalConfig(c.Request.Context(), id)
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

// CreateGlobalConfig creates a new global configuration
func (h *EnvConfigHandler) CreateGlobalConfig(c *gin.Context) {
	var req struct {
		Category    string `json:"category" binding:"required"`
		Key         string `json:"key" binding:"required"`
		Value       string `json:"value" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	config := &model.GlobalConfig{
		Category:    req.Category,
		Key:         req.Key,
		Value:       req.Value,
		Description: req.Description,
	}

	if err := h.envConfigService.CreateGlobalConfig(c.Request.Context(), config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, config)
}

// UpdateGlobalConfig updates a global configuration
func (h *EnvConfigHandler) UpdateGlobalConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Value       string `json:"value" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	config, err := h.envConfigService.GetGlobalConfig(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if config == nil {
		c.Error(ErrNotFound.WithMessage("配置不存在"))
		return
	}

	config.Value = req.Value
	config.Description = req.Description

	if err := h.envConfigService.UpdateGlobalConfig(c.Request.Context(), config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, config)
}

// DeleteGlobalConfig deletes a global configuration
func (h *EnvConfigHandler) DeleteGlobalConfig(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.envConfigService.DeleteGlobalConfig(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "删除成功"})
}
