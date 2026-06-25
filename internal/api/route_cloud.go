package api

import (
	"log"

	"easyserver/internal/config"

	"github.com/gin-gonic/gin"
)

// registerCloudRoutes registers cloud management routes
func registerCloudRoutes(protected *gin.RouterGroup, cfg *config.TencentCloudConfig, panelPort int) {
	handler, err := NewCloudHandler(cfg, panelPort)
	if err != nil {
		log.Printf("Warning: failed to init cloud handler: %v", err)
		handler = &CloudHandler{}
	}
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
