package http

import (
	"strconv"
	"strings"
	"time"

	"easyserver/internal/cloud"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type CloudHandler struct {
	cloudService    *cloud.Service
	currentInstance string // The instance running this panel
	panelPort       int    // Panel port for self-protection
}

func NewCloudHandler(cloudService *cloud.Service, currentInstance string, panelPort int) *CloudHandler {
	return &CloudHandler{
		cloudService:    cloudService,
		currentInstance: currentInstance,
		panelPort:       panelPort,
	}
}

// isCurrentInstance checks if the instance is the one running this panel
func (h *CloudHandler) isCurrentInstance(instanceID string) bool {
	return h.currentInstance != "" && h.currentInstance == instanceID
}

// GetInstances returns all instances
func (h *CloudHandler) GetInstances(c *gin.Context) (any, error) {
	if h.cloudService == nil {
		return gin.H{"instances": []any{}, "total_count": 0}, nil
	}

	instances, err := h.cloudService.GetInstances(c.Request.Context())
	if err != nil {
		return nil, err
	}

	return gin.H{
		"instances":   instances,
		"total_count": len(instances),
	}, nil
}

// GetInstance returns a specific instance
func (h *CloudHandler) GetInstance(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	instance, err := h.cloudService.GetInstance(c.Request.Context(), instanceID)
	if err != nil {
		return nil, err
	}

	return instance, nil
}

// StartInstance starts an instance
func (h *CloudHandler) StartInstance(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	middleware.AuditSummary(c, "启动云主机 "+instanceID)
	if err := h.cloudService.StartInstance(c.Request.Context(), instanceID); err != nil {
		return nil, err
	}

	return gin.H{"instance_id": instanceID, "state": "STARTING"}, nil
}

// StopInstance stops an instance
func (h *CloudHandler) StopInstance(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	// Prevent stopping the current instance
	if h.isCurrentInstance(instanceID) {
		return nil, errx.BadRequest("cannot stop the instance running this panel")
	}

	middleware.AuditSummary(c, "停止云主机 "+instanceID)
	if err := h.cloudService.StopInstance(c.Request.Context(), instanceID); err != nil {
		return nil, err
	}

	return gin.H{"instance_id": instanceID, "state": "STOPPING"}, nil
}

// RestartInstance restarts an instance
func (h *CloudHandler) RestartInstance(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	// Prevent restarting the current instance (panel will be unavailable during restart)
	if h.isCurrentInstance(instanceID) {
		return nil, errx.BadRequest("cannot restart the instance running this panel, use /api/settings/restart instead")
	}

	middleware.AuditSummary(c, "重启云主机 "+instanceID)
	if err := h.cloudService.RestartInstance(c.Request.Context(), instanceID); err != nil {
		return nil, err
	}

	return gin.H{"instance_id": instanceID, "state": "REBOOTING"}, nil
}

// GetFirewallRules returns firewall rules
func (h *CloudHandler) GetFirewallRules(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return gin.H{"rules": []any{}}, nil
	}

	rules, err := h.cloudService.GetFirewallRules(c.Request.Context(), instanceID)
	if err != nil {
		return nil, err
	}

	return gin.H{"rules": rules}, nil
}

// AddFirewallRule adds a firewall rule
func (h *CloudHandler) AddFirewallRule(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	if instanceID == "" {
		return nil, errx.BadRequest("instance id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	var rule cloud.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Validate port format (e.g., "80", "443", "8000-9000", "ALL")
	if rule.Port == "" {
		return nil, errx.BadRequest("port is required")
	}

	// Prevent blocking panel port on current instance. Check exact match,
	// "ALL", and port ranges (e.g. "80-443") so a broad DROP can't slip past.
	if h.isCurrentInstance(instanceID) && rule.Action != "ACCEPT" && portCoversPanel(rule.Port, h.panelPort) {
		return nil, errx.BadRequest("cannot block panel port on the current instance")
	}

	middleware.AuditSummary(c, "添加防火墙规则 "+instanceID+" 端口 "+rule.Port)
	if err := h.cloudService.AddFirewallRule(c.Request.Context(), instanceID, rule); err != nil {
		return nil, err
	}

	return nil, nil
}

// DeleteFirewallRule deletes a firewall rule
func (h *CloudHandler) DeleteFirewallRule(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	ruleID := c.Param("ruleId")

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	if instanceID == "" {
		return nil, errx.BadRequest("instance ID is required")
	}

	middleware.AuditSummary(c, "删除防火墙规则 "+instanceID+" "+ruleID)
	if err := h.cloudService.DeleteFirewallRule(c.Request.Context(), instanceID, ruleID); err != nil {
		return nil, err
	}

	return nil, nil
}

// GetSnapshots returns snapshots
func (h *CloudHandler) GetSnapshots(c *gin.Context) (any, error) {
	instanceID := c.Query("instance_id")

	if h.cloudService == nil {
		return gin.H{"snapshots": []any{}}, nil
	}

	if instanceID == "" {
		return nil, errx.BadRequest("instance_id query parameter is required")
	}

	snapshots, err := h.cloudService.GetSnapshots(c.Request.Context(), instanceID)
	if err != nil {
		return nil, err
	}

	return gin.H{"snapshots": snapshots}, nil
}

// CreateSnapshot creates a snapshot
func (h *CloudHandler) CreateSnapshot(c *gin.Context) (any, error) {
	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	var req struct {
		InstanceID string `json:"instance_id" binding:"required"`
		Name       string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "创建云主机快照 "+req.InstanceID+" "+req.Name)
	if err := h.cloudService.CreateSnapshot(c.Request.Context(), req.InstanceID, req.Name); err != nil {
		return nil, err
	}

	return nil, nil
}

// ApplySnapshot applies a snapshot (rollback)
func (h *CloudHandler) ApplySnapshot(c *gin.Context) (any, error) {
	snapshotID := c.Param("id")
	if snapshotID == "" {
		return nil, errx.BadRequest("snapshot id is required")
	}

	if h.cloudService == nil {
		return nil, errx.NotFound("cloud service not enabled")
	}

	middleware.AuditSummary(c, "应用云主机快照 "+snapshotID)
	if err := h.cloudService.ApplySnapshot(c.Request.Context(), snapshotID); err != nil {
		return nil, err
	}

	return gin.H{"snapshot_id": snapshotID, "status": "APPLYING"}, nil
}

// GetMonitorData returns monitor data
func (h *CloudHandler) GetMonitorData(c *gin.Context) (any, error) {
	instanceID := c.Param("id")
	metric := c.Query("metric")
	startStr := c.Query("start")
	endStr := c.Query("end")

	if h.cloudService == nil {
		return gin.H{"metric": metric, "points": []any{}}, nil
	}

	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()

	if startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}
	if endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}

	data, err := h.cloudService.GetMonitorData(c.Request.Context(), instanceID, metric, start, end)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// GetTraffic returns traffic info
func (h *CloudHandler) GetTraffic(c *gin.Context) (any, error) {
	instanceID := c.Query("instance_id")

	if h.cloudService == nil {
		return gin.H{}, nil
	}

	traffic, err := h.cloudService.GetTraffic(c.Request.Context(), instanceID)
	if err != nil {
		return nil, err
	}

	return traffic, nil
}

// portCoversPanel reports whether a firewall rule's port spec (exact, "ALL",
// or "min-max" range) includes panelPort.
func portCoversPanel(portSpec string, panelPort int) bool {
	if portSpec == "" {
		return false
	}
	if portSpec == "ALL" || portSpec == strconv.Itoa(panelPort) {
		return true
	}
	if i := strings.IndexByte(portSpec, '-'); i > 0 {
		lo, err1 := strconv.Atoi(strings.TrimSpace(portSpec[:i]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(portSpec[i+1:]))
		if err1 == nil && err2 == nil {
			return panelPort >= lo && panelPort <= hi
		}
	}
	return false
}

func RegisterRoutes(protected *gin.RouterGroup, cloudService *cloud.Service, cfg *config.TencentCloudConfig, panelPort int) {
	handler := NewCloudHandler(cloudService, cfg.InstanceID, panelPort)
	protected.GET("/cloud/instances", httpx.H(handler.GetInstances))
	protected.GET("/cloud/instances/:id", httpx.H(handler.GetInstance))
	protected.GET("/cloud/monitor/:id", httpx.H(handler.GetMonitorData))
	protected.GET("/cloud/firewall/:id", httpx.H(handler.GetFirewallRules))
	protected.GET("/cloud/snapshots", httpx.H(handler.GetSnapshots))
	protected.GET("/cloud/traffic", httpx.H(handler.GetTraffic))
	protected.POST("/cloud/instances/:id/start", httpx.H(handler.StartInstance))
	protected.POST("/cloud/instances/:id/stop", httpx.H(handler.StopInstance))
	protected.POST("/cloud/instances/:id/restart", httpx.H(handler.RestartInstance))
	protected.POST("/cloud/firewall/:id", httpx.H(handler.AddFirewallRule))
	protected.DELETE("/cloud/firewall/:id/:ruleId", httpx.H(handler.DeleteFirewallRule))
	protected.POST("/cloud/snapshots", httpx.H(handler.CreateSnapshot))
	protected.POST("/cloud/snapshots/:id/apply", httpx.H(handler.ApplySnapshot))
}
