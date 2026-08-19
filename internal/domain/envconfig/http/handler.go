package http

import (
	"strconv"

	"easyserver/internal/domain/envconfig"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type EnvConfigHandler struct {
	envConfigService *envconfig.Service
}

func NewEnvConfigHandler(envConfigService *envconfig.Service) *EnvConfigHandler {
	return &EnvConfigHandler{envConfigService: envConfigService}
}

// ListEnvConfigs returns all environment configurations
func (h *EnvConfigHandler) ListEnvConfigs(c *gin.Context) (any, error) {
	configs, err := h.envConfigService.ListEnvConfigs(c.Request.Context())
	if err != nil {
		return nil, err
	}

	return gin.H{
		"configs": configs,
	}, nil
}

// GetEnvConfig returns a specific environment configuration
func (h *EnvConfigHandler) GetEnvConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	config, err := h.envConfigService.GetEnvConfig(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errx.NotFound("配置不存在")
	}

	return config, nil
}

// CreateEnvConfig creates a new environment configuration
func (h *EnvConfigHandler) CreateEnvConfig(c *gin.Context) (any, error) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Value   string `json:"value" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("无效的请求: %w", err)
	}
	middleware.AuditSummary(c, "创建环境变量 "+req.Name)

	config := &envconfig.EnvConfig{
		Name:    req.Name,
		Value:   req.Value,
		Enabled: req.Enabled,
	}

	if err := h.envConfigService.CreateEnvConfig(c.Request.Context(), config); err != nil {
		return nil, err
	}

	return config, nil
}

// UpdateEnvConfig updates an environment configuration
func (h *EnvConfigHandler) UpdateEnvConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		Name    string `json:"name" binding:"required"`
		Value   string `json:"value" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("无效的请求: %w", err)
	}
	middleware.AuditSummary(c, "更新环境变量 "+req.Name)

	config, err := h.envConfigService.GetEnvConfig(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errx.NotFound("配置不存在")
	}

	config.Name = req.Name
	config.Value = req.Value
	config.Enabled = req.Enabled

	if err := h.envConfigService.UpdateEnvConfig(c.Request.Context(), config); err != nil {
		return nil, err
	}

	return config, nil
}

// DeleteEnvConfig deletes an environment configuration
func (h *EnvConfigHandler) DeleteEnvConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "删除环境变量 #"+strconv.FormatInt(id, 10))

	if err := h.envConfigService.DeleteEnvConfig(c.Request.Context(), id); err != nil {
		return nil, err
	}

	return gin.H{"message": "删除成功"}, nil
}

// ListPathEntries returns all PATH entries
func (h *EnvConfigHandler) ListPathEntries(c *gin.Context) (any, error) {
	entries, err := h.envConfigService.ListPathEntries(c.Request.Context())
	if err != nil {
		return nil, err
	}

	return gin.H{
		"entries": entries,
	}, nil
}

// CreatePathEntry creates a new PATH entry
func (h *EnvConfigHandler) CreatePathEntry(c *gin.Context) (any, error) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("无效的请求: %w", err)
	}
	middleware.AuditSummary(c, "添加 PATH 条目 "+req.Path)

	entry := &envconfig.PathEntry{
		Path:    req.Path,
		Enabled: req.Enabled,
	}

	if err := h.envConfigService.CreatePathEntry(c.Request.Context(), entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// UpdatePathEntry updates an existing PATH entry
func (h *EnvConfigHandler) UpdatePathEntry(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		Path    string `json:"path" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("无效的请求: %w", err)
	}

	existingPaths, err := h.envConfigService.ListPathEntries(c.Request.Context())
	if err != nil {
		return nil, err
	}
	var existing *envconfig.PathEntry
	for _, p := range existingPaths {
		if p.ID == id {
			existing = &p
			break
		}
	}
	if existing == nil {
		return nil, errx.NotFound("PATH 条目不存在")
	}

	existing.Path = req.Path
	existing.Enabled = req.Enabled

	if err := h.envConfigService.UpdatePathEntry(c.Request.Context(), existing); err != nil {
		return nil, err
	}
	return nil, nil
}

// DeletePathEntry deletes a PATH entry
func (h *EnvConfigHandler) DeletePathEntry(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "删除 PATH 条目 #"+strconv.FormatInt(id, 10))

	if err := h.envConfigService.DeletePathEntry(c.Request.Context(), id); err != nil {
		return nil, err
	}

	return gin.H{"message": "删除成功"}, nil
}

// GenerateEnvScript generates a shell script to set environment variables
func (h *EnvConfigHandler) GenerateEnvScript(c *gin.Context) (any, error) {
	script, err := h.envConfigService.GenerateEnvScript(c.Request.Context())
	if err != nil {
		return nil, err
	}

	return gin.H{
		"script": script,
	}, nil
}

// RegisterRoutes registers environment configuration routes
func RegisterRoutes(protected *gin.RouterGroup, envConfigService *envconfig.Service) {
	handler := NewEnvConfigHandler(envConfigService)
	protected.GET("/env-config", httpx.H(handler.ListEnvConfigs))
	protected.GET("/env-config/:id", httpx.H(handler.GetEnvConfig))
	protected.POST("/env-config", httpx.H(handler.CreateEnvConfig))
	protected.PUT("/env-config/:id", httpx.H(handler.UpdateEnvConfig))
	protected.DELETE("/env-config/:id", httpx.H(handler.DeleteEnvConfig))
	protected.GET("/env-config/path", httpx.H(handler.ListPathEntries))
	protected.POST("/env-config/path", httpx.H(handler.CreatePathEntry))
	protected.PUT("/env-config/path/:id", httpx.H(handler.UpdatePathEntry))
	protected.DELETE("/env-config/path/:id", httpx.H(handler.DeletePathEntry))
	protected.GET("/env-config/script", httpx.H(handler.GenerateEnvScript))
}
