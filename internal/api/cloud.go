package api

import (
	"strconv"
	"time"

	"easyserver/internal/config"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type CloudHandler struct {
	cloudService    *service.CloudService
	currentInstance string // The instance running this panel
	panelPort       int    // Panel port for self-protection
}

func NewCloudHandler(cloudService *service.CloudService, currentInstance string, panelPort int) *CloudHandler {
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

	var rule service.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate port format (e.g., "80", "443", "8000-9000", "ALL")
	if rule.Port == "" {
		c.Error(ErrBadRequest.WithMessage("port is required"))
		return
	}

	// Prevent blocking panel port on current instance
	panelPortStr := strconv.Itoa(h.panelPort)
	if h.isCurrentInstance(instanceID) && rule.Port == panelPortStr && rule.Action != "ACCEPT" {
		c.Error(ErrBadRequest.WithMessage("cannot block panel port on the current instance"))
		return
	}

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

func registerCloudRoutes(protected *gin.RouterGroup, cloudService *service.CloudService, cfg *config.TencentCloudConfig, panelPort int) {
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
