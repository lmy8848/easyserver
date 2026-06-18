package api

import (
	"time"

	"easyserver/internal/config"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type CloudHandler struct {
	cloudService *service.CloudService
}

func NewCloudHandler(cfg *config.TencentCloudConfig) (*CloudHandler, error) {
	if !cfg.Enabled {
		return &CloudHandler{}, nil
	}

	cloudService, err := service.NewCloudService(
		cfg.SecretID,
		cfg.SecretKey,
		cfg.Region,
		cfg.InstanceID,
	)
	if err != nil {
		return nil, err
	}

	return &CloudHandler{
		cloudService: cloudService,
	}, nil
}

// GetInstances returns all instances
func (h *CloudHandler) GetInstances(c *gin.Context) {
	if h.cloudService == nil {
		Success(c, gin.H{"instances": []interface{}{}, "total_count": 0})
		return
	}

	instances, err := h.cloudService.GetInstances()
	if err != nil {
		InternalError(c, err.Error())
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
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	instance, err := h.cloudService.GetInstance(instanceID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	Success(c, instance)
}

// StartInstance starts an instance
func (h *CloudHandler) StartInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	if err := h.cloudService.StartInstance(instanceID); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "STARTING"})
}

// StopInstance stops an instance
func (h *CloudHandler) StopInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	if err := h.cloudService.StopInstance(instanceID); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "STOPPING"})
}

// RestartInstance restarts an instance
func (h *CloudHandler) RestartInstance(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	if err := h.cloudService.RestartInstance(instanceID); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"instance_id": instanceID, "state": "REBOOTING"})
}

// GetFirewallRules returns firewall rules
func (h *CloudHandler) GetFirewallRules(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		Success(c, gin.H{"rules": []interface{}{}})
		return
	}

	rules, err := h.cloudService.GetFirewallRules(instanceID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"rules": rules})
}

// AddFirewallRule adds a firewall rule
func (h *CloudHandler) AddFirewallRule(c *gin.Context) {
	instanceID := c.Param("id")
	if instanceID == "" {
		BadRequest(c, "instance id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	var rule service.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.cloudService.AddFirewallRule(instanceID, rule); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

// DeleteFirewallRule deletes a firewall rule
func (h *CloudHandler) DeleteFirewallRule(c *gin.Context) {
	instanceID := c.Param("id")
	ruleID := c.Param("ruleId")

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	if err := h.cloudService.DeleteFirewallRule(instanceID, ruleID); err != nil {
		InternalError(c, err.Error())
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

	snapshots, err := h.cloudService.GetSnapshots(instanceID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"snapshots": snapshots})
}

// CreateSnapshot creates a snapshot
func (h *CloudHandler) CreateSnapshot(c *gin.Context) {
	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	var req struct {
		InstanceID string `json:"instance_id" binding:"required"`
		Name       string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.cloudService.CreateSnapshot(req.InstanceID, req.Name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

// ApplySnapshot applies a snapshot (rollback)
func (h *CloudHandler) ApplySnapshot(c *gin.Context) {
	snapshotID := c.Param("id")
	if snapshotID == "" {
		BadRequest(c, "snapshot id is required")
		return
	}

	if h.cloudService == nil {
		NotFound(c, "cloud service not enabled")
		return
	}

	if err := h.cloudService.ApplySnapshot(snapshotID); err != nil {
		InternalError(c, err.Error())
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

	data, err := h.cloudService.GetMonitorData(instanceID, metric, start, end)
	if err != nil {
		InternalError(c, err.Error())
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

	traffic, err := h.cloudService.GetTraffic(instanceID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, traffic)
}
