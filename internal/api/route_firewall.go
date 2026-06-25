package api

import (
	"strconv"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerFirewallRoutes registers firewall management routes
func registerFirewallRoutes(protected *gin.RouterGroup, firewallService *service.FirewallService, panelPort int) {
	// Set protected ports: SSH (22) + panel port from config
	firewallService.SetProtectedPorts([]string{"22", strconv.Itoa(panelPort)})
	handler := NewFirewallHandler(firewallService, panelPort)

	protected.GET("/firewall/status", handler.GetStatus)
	protected.POST("/firewall/enable", handler.EnableFirewall)
	protected.POST("/firewall/disable", handler.DisableFirewall)
	protected.GET("/firewall/rules", handler.ListRules)
	protected.POST("/firewall/rules", handler.CreateRule)
	protected.GET("/firewall/rules/:id", handler.GetRule)
	protected.PUT("/firewall/rules/:id", handler.UpdateRule)
	protected.DELETE("/firewall/rules/:id", handler.DeleteRule)
	protected.POST("/firewall/rules/:id/enable", handler.EnableRule)
	protected.POST("/firewall/rules/:id/disable", handler.DisableRule)
	protected.POST("/firewall/rules/:id/move-up", handler.MoveRuleUp)
	protected.POST("/firewall/rules/:id/move-down", handler.MoveRuleDown)
	protected.POST("/firewall/rules/bulk-enable", handler.BulkEnableRules)
	protected.POST("/firewall/rules/bulk-disable", handler.BulkDisableRules)
	protected.POST("/firewall/rules/bulk-delete", handler.BulkDeleteRules)
	protected.GET("/firewall/rules/export", handler.ExportRules)
	protected.POST("/firewall/rules/import", handler.ImportRules)
	protected.POST("/firewall/default-policy", handler.SetDefaultPolicy)
	protected.GET("/firewall/templates", handler.GetTemplates)
	protected.POST("/firewall/templates/apply", handler.ApplyTemplate)
	protected.GET("/firewall/system-rules", handler.GetSystemRules)
	protected.POST("/firewall/system-rules/delete", handler.DeleteSystemRule)
	protected.GET("/firewall/logs", handler.GetLogs)
}
