package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"easyserver/internal/cloud"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/config"
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
func (h *CloudHandler) GetInstances(c *gin.Context) {
	if h.cloudService == nil {
		Success(c, gin.H{"instances": []interface{}{}, "total_count": 0})
		return
	}

	instances, err := h.cloudService.GetInstances(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
		"instances":   instances,
		"total_count": len(instances),
	})
}

// GetInstance returns a specific instance
func (h *CloudHandler) GetInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	instance, err := h.cloudService.GetInstance(c.Request.Context(), instanceID)
	if err != nil {
		c.Error(ErrNotFound.Wrap(err))
		return
	}

	Success(c, instance)
}

// StartInstance starts an instance
func (h *CloudHandler) StartInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	middleware.AuditSummary(c, "启动云主机 "+instanceID)
	if err := h.cloudService.StartInstance(c.Request.Context(), instanceID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "STARTING"})
}

// StopInstance stops an instance
func (h *CloudHandler) StopInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	// Prevent stopping the current instance
	if h.isCurrentInstance(instanceID) {
		c.Error(ErrBadRequest.WithMessage("cannot stop the instance running this panel"))
		return
	}

	middleware.AuditSummary(c, "停止云主机 "+instanceID)
	if err := h.cloudService.StopInstance(c.Request.Context(), instanceID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "STOPPING"})
}

// RestartInstance restarts an instance
func (h *CloudHandler) RestartInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	// Prevent restarting the current instance (panel will be unavailable during restart)
	if h.isCurrentInstance(instanceID) {
		c.Error(ErrBadRequest.WithMessage("cannot restart the instance running this panel, use /api/settings/restart instead"))
		return
	}

	middleware.AuditSummary(c, "重启云主机 "+instanceID)
	if err := h.cloudService.RestartInstance(c.Request.Context(), instanceID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "REBOOTING"})
}

// GetFirewallRules returns firewall rules
func (h *CloudHandler) GetFirewallRules(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		Success(c, gin.H{"rules": []interface{}{}})
		return
	}

	rules, err := h.cloudService.GetFirewallRules(c.Request.Context(), instanceID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"rules": rules})
}

// AddFirewallRule adds a firewall rule
func (h *CloudHandler) AddFirewallRule(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	var rule cloud.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate port format (e.g., "80", "443", "8000-9000", "ALL")
	if rule.Port == "" {
		c.Error(ErrBadRequest.WithMessage("port is required"))
		return
	}

	// Prevent blocking panel port on current instance. Check exact match,
	// "ALL", and port ranges (e.g. "80-443") so a broad DROP can't slip past.
	if h.isCurrentInstance(instanceID) && rule.Action != "ACCEPT" && portCoversPanel(rule.Port, h.panelPort) {
		c.Error(ErrBadRequest.WithMessage("cannot block panel port on the current instance"))
		return
	}

	middleware.AuditSummary(c, "添加防火墙规则 "+instanceID+" 端口 "+rule.Port)
	if err := h.cloudService.AddFirewallRule(c.Request.Context(), instanceID, rule); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// DeleteFirewallRule deletes a firewall rule
func (h *CloudHandler) DeleteFirewallRule(c *gin.Context) {
	instanceID := c.Param("id")
	ruleID := c.Param("ruleId")

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance ID is required"))
		return
	}

	middleware.AuditSummary(c, "删除防火墙规则 "+instanceID+" "+ruleID)
	if err := h.cloudService.DeleteFirewallRule(c.Request.Context(), instanceID, ruleID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// GetSnapshots returns snapshots
func (h *CloudHandler) GetSnapshots(c *gin.Context) {
	instanceID := c.Query("instance_id")

	if h.cloudService == nil {
		Success(c, gin.H{"snapshots": []interface{}{}})
		return
	}

	if instanceID == "" {
		c.Error(ErrBadRequest.WithMessage("instance_id query parameter is required"))
		return
	}

	snapshots, err := h.cloudService.GetSnapshots(c.Request.Context(), instanceID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"snapshots": snapshots})
}

// CreateSnapshot creates a snapshot
func (h *CloudHandler) CreateSnapshot(c *gin.Context) {
	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	var req struct {
		InstanceID string `json:"instance_id" binding:"required"`
		Name       string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建云主机快照 "+req.InstanceID+" "+req.Name)
	if err := h.cloudService.CreateSnapshot(c.Request.Context(), req.InstanceID, req.Name); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// ApplySnapshot applies a snapshot (rollback)
func (h *CloudHandler) ApplySnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	if snapshotID == "" {
		c.Error(ErrBadRequest.WithMessage("snapshot id is required"))
		return
	}

	if h.cloudService == nil {
		c.Error(ErrNotFound.WithMessage("cloud service not enabled"))
		return
	}

	middleware.AuditSummary(c, "应用云主机快照 "+snapshotID)
	if err := h.cloudService.ApplySnapshot(c.Request.Context(), snapshotID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"snapshot_id": snapshotID, "status": "APPLYING"})
}

// GetMonitorData returns monitor data
func (h *CloudHandler) GetMonitorData(c *gin.Context) {
	instanceID := c.Param("id")
	metric := c.Query("metric")
	startStr := c.Query("start")
	endStr := c.Query("end")

	if h.cloudService == nil {
		Success(c, gin.H{"metric": metric, "points": []interface{}{}})
		return
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
		c.Error(WrapError(err))
		return
	}

	Success(c, data)
}

// GetTraffic returns traffic info
func (h *CloudHandler) GetTraffic(c *gin.Context) {
	instanceID := c.Query("instance_id")
	if instanceID == "" {
		instanceID = ""
	}

	if h.cloudService == nil {
		Success(c, gin.H{})
		return
	}

	traffic, err := h.cloudService.GetTraffic(c.Request.Context(), instanceID)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, traffic)
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

func registerCloudRoutes(protected *gin.RouterGroup, cloudService *cloud.Service, cfg *config.TencentCloudConfig, panelPort int) {
	if cloudService == nil {
		return // Cloud service not enabled, skip route registration
	}
	handler := NewCloudHandler(cloudService, cfg.InstanceID, panelPort)
	protected.GET("/cloud/instances", handler.GetInstances)
	protected.GET("/cloud/instances/:id", handler.GetInstance)
	protected.GET("/cloud/monitor/:id", handler.GetMonitorData)
	protected.GET("/cloud/firewall/:id", handler.GetFirewallRules)
	protected.GET("/cloud/snapshots", handler.GetSnapshots)
	protected.GET("/cloud/traffic", handler.GetTraffic)
	protected.POST("/cloud/instances/:id/start", handler.StartInstance)
	protected.POST("/cloud/instances/:id/stop", handler.StopInstance)
	protected.POST("/cloud/instances/:id/restart", handler.RestartInstance)
	protected.POST("/cloud/firewall/:id", handler.AddFirewallRule)
	protected.DELETE("/cloud/firewall/:id/:ruleId", handler.DeleteFirewallRule)
	protected.POST("/cloud/snapshots", handler.CreateSnapshot)
	protected.POST("/cloud/snapshots/:id/apply", handler.ApplySnapshot)
}
