package api

import (
	"fmt"

	"easyserver/internal/api/middleware"
	"easyserver/internal/firewall"

	"github.com/gin-gonic/gin"
)

// FirewallTemplateHandler handles firewall rule template operations
type FirewallTemplateHandler struct {
	firewallService *firewall.Service
}

// NewFirewallTemplateHandler creates a new FirewallTemplateHandler
func NewFirewallTemplateHandler(firewallService *firewall.Service) *FirewallTemplateHandler {
	return &FirewallTemplateHandler{
		firewallService: firewallService,
	}
}

// isProtectedPort checks if a port is protected (panel port or SSH)
func (h *FirewallTemplateHandler) isProtectedPort(c *gin.Context, port string) bool {
	return h.firewallService.IsProtectedPort(c.Request.Context(), port)
}

// FirewallRuleTemplate represents a pre-built firewall rule template
type FirewallRuleTemplate struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Action   string `json:"action"`
	Remark   string `json:"remark"`
}

// firewallRuleTemplates is the list of pre-built rule templates
var firewallRuleTemplates = []FirewallRuleTemplate{
	{Name: "允许 SSH", Protocol: "tcp", Port: "22", Action: "ACCEPT", Remark: "SSH 远程访问"},
	{Name: "允许 HTTP", Protocol: "tcp", Port: "80", Action: "ACCEPT", Remark: "Web 服务"},
	{Name: "允许 HTTPS", Protocol: "tcp", Port: "443", Action: "ACCEPT", Remark: "安全 Web 服务"},
	{Name: "允许 MySQL", Protocol: "tcp", Port: "3306", Action: "ACCEPT", Remark: "MySQL 数据库"},
	{Name: "允许 PostgreSQL", Protocol: "tcp", Port: "5432", Action: "ACCEPT", Remark: "PostgreSQL 数据库"},
	{Name: "允许 Redis", Protocol: "tcp", Port: "6379", Action: "ACCEPT", Remark: "Redis 缓存"},
	{Name: "允许 FTP", Protocol: "tcp", Port: "21", Action: "ACCEPT", Remark: "FTP 文件传输"},
	{Name: "允许 DNS", Protocol: "udp", Port: "53", Action: "ACCEPT", Remark: "DNS 解析"},
	{Name: "允许 SMTP", Protocol: "tcp", Port: "25", Action: "ACCEPT", Remark: "邮件发送"},
	{Name: "阻止 Telnet", Protocol: "tcp", Port: "23", Action: "DROP", Remark: "阻止不安全的 Telnet"},
}

// GetTemplates returns the list of available rule templates
func (h *FirewallTemplateHandler) GetTemplates(c *gin.Context) {
	Success(c, firewallRuleTemplates)
}

// ApplyTemplate creates a firewall rule from a named template
func (h *FirewallTemplateHandler) ApplyTemplate(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("请提供模板名称"))
		return
	}

	middleware.AuditSummary(c, "应用防火墙模板 "+req.Name)
	// Find the template
	var tpl *FirewallRuleTemplate
	for i := range firewallRuleTemplates {
		if firewallRuleTemplates[i].Name == req.Name {
			tpl = &firewallRuleTemplates[i]
			break
		}
	}
	if tpl == nil {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("未找到模板: %s", req.Name)))
		return
	}

	// Check if the template would block a protected port
	if tpl.Action != "ACCEPT" && tpl.Port != "" && h.isProtectedPort(c, tpl.Port) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", tpl.Port)))
		return
	}

	rule := &firewall.FirewallRule{
		Chain:     "INPUT",
		Protocol:  tpl.Protocol,
		Port:      tpl.Port,
		Action:    tpl.Action,
		Enabled:   true,
		IPVersion: "ipv4",
		Remark:    tpl.Remark,
	}

	if err := h.firewallService.CreateRule(c.Request.Context(), rule); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, rule)
}
