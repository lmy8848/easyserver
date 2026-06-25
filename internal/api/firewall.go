package api

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// FirewallHandler handles firewall API requests
type FirewallHandler struct {
	firewallService *service.FirewallService
	panelPort       string
}

// NewFirewallHandler creates a new FirewallHandler
func NewFirewallHandler(firewallService *service.FirewallService, panelPort int) *FirewallHandler {
	return &FirewallHandler{
		firewallService: firewallService,
		panelPort:       strconv.Itoa(panelPort),
	}
}

// isProtectedPort checks if a port is protected (panel port or SSH)
func (h *FirewallHandler) isProtectedPort(c *gin.Context, port string) bool {
	return h.firewallService.IsProtectedPort(c.Request.Context(), port)
}

// GetStatus returns the firewall status
func (h *FirewallHandler) GetStatus(c *gin.Context) {
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, status)
}

// EnableFirewall enables the firewall
func (h *FirewallHandler) EnableFirewall(c *gin.Context) {
	if err := h.firewallService.EnableFirewall(c.Request.Context()); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "防火墙已启用"})
}

// DisableFirewall disables the firewall
func (h *FirewallHandler) DisableFirewall(c *gin.Context) {
	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		BadRequest(c, "请确认禁用防火墙，设置 {\"confirm\": true}")
		return
	}

	if err := h.firewallService.DisableFirewall(c.Request.Context()); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "防火墙已禁用"})
}

// ListRules returns all firewall rules
func (h *FirewallHandler) ListRules(c *gin.Context) {
	rules, err := h.firewallService.ListRules(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rules)
}

// GetRule returns a firewall rule by ID
func (h *FirewallHandler) GetRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		NotFound(c, "规则不存在")
		return
	}
	Success(c, rule)
}

// CreateRule creates a new firewall rule
func (h *FirewallHandler) CreateRule(c *gin.Context) {
	var req model.CreateFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Validate chain
	validChains := map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true}
	if !validChains[strings.ToUpper(req.Chain)] {
		BadRequest(c, "无效的链，必须是 INPUT、OUTPUT 或 FORWARD")
		return
	}

	// Validate action
	validActions := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
	action := strings.ToUpper(req.Action)
	if !validActions[action] {
		BadRequest(c, "无效的动作，必须是 ACCEPT、DROP 或 REJECT")
		return
	}

	// Set defaults
	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	validProtocols := map[string]bool{"tcp": true, "udp": true, "all": true, "icmp": true}
	if !validProtocols[strings.ToLower(protocol)] {
		BadRequest(c, "无效的协议，必须是 tcp、udp、all 或 icmp")
		return
	}

	// Validate port format if provided
	if req.Port != "" {
		if !isValidPort(req.Port) {
			BadRequest(c, "无效的端口格式，使用单端口 (80) 或范围 (8000-9000)")
			return
		}

		// Check if port is protected (panel port or SSH)
		if action != "ACCEPT" && h.isProtectedPort(c, req.Port) {
			BadRequest(c, fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", req.Port))
			return
		}
	}

	// Validate source IP/CIDR if provided
	if req.Source != "" && req.Source != "0.0.0.0/0" && req.Source != "::/0" {
		if !isValidCIDR(req.Source) {
			BadRequest(c, "无效的源地址格式，使用 IP (192.168.1.1) 或 CIDR (192.168.1.0/24)")
			return
		}
	}

	// Validate and default ip_version
	ipVersion := req.IPVersion
	if ipVersion == "" {
		ipVersion = "ipv4"
	}
	validIPVersions := map[string]bool{"ipv4": true, "ipv6": true, "both": true}
	if !validIPVersions[ipVersion] {
		BadRequest(c, "无效的 ip_version，必须是 ipv4、ipv6 或 both")
		return
	}

	rule := &model.FirewallRule{
		Chain:     strings.ToUpper(req.Chain),
		Protocol:  strings.ToLower(protocol),
		Port:      req.Port,
		Action:    action,
		Source:    req.Source,
		Enabled:   true,
		IPVersion: ipVersion,
		Remark:    req.Remark,
	}

	if err := h.firewallService.CreateRule(c.Request.Context(), rule); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rule)
}

// UpdateRule updates an existing firewall rule
func (h *FirewallHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}

	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		NotFound(c, "规则不存在")
		return
	}

	var req model.UpdateFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.Chain != nil {
		validChains := map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true}
		if !validChains[strings.ToUpper(*req.Chain)] {
			BadRequest(c, "无效的链")
			return
		}
		rule.Chain = strings.ToUpper(*req.Chain)
	}
	if req.Protocol != nil {
		validProtocols := map[string]bool{"tcp": true, "udp": true, "all": true, "icmp": true}
		if !validProtocols[strings.ToLower(*req.Protocol)] {
			BadRequest(c, "无效的协议")
			return
		}
		rule.Protocol = strings.ToLower(*req.Protocol)
	}
	if req.Port != nil {
		if *req.Port != "" && !isValidPort(*req.Port) {
			BadRequest(c, "无效的端口格式")
			return
		}
		rule.Port = *req.Port
	}
	if req.Action != nil {
		validActions := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
		if !validActions[strings.ToUpper(*req.Action)] {
			BadRequest(c, "无效的动作")
			return
		}
		rule.Action = strings.ToUpper(*req.Action)
	}
	if req.Source != nil {
		if *req.Source != "" && *req.Source != "0.0.0.0/0" && *req.Source != "::/0" && !isValidCIDR(*req.Source) {
			BadRequest(c, "无效的源地址格式，使用 IP (192.168.1.1) 或 CIDR (192.168.1.0/24)")
			return
		}
		rule.Source = *req.Source
	}
	if req.IPVersion != nil {
		validIPVersions := map[string]bool{"ipv4": true, "ipv6": true, "both": true}
		if !validIPVersions[*req.IPVersion] {
			BadRequest(c, "无效的 ip_version，必须是 ipv4、ipv6 或 both")
			return
		}
		rule.IPVersion = *req.IPVersion
	}
	if req.Remark != nil {
		rule.Remark = *req.Remark
	}

	// Check if the updated rule would block a protected port
	if rule.Action != "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		BadRequest(c, fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", rule.Port))
		return
	}

	if err := h.firewallService.UpdateRule(c.Request.Context(), rule); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rule)
}

// DeleteRule deletes a firewall rule
func (h *FirewallHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	// Check existence
	if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
		NotFound(c, "规则不存在")
		return
	}
	if err := h.firewallService.DeleteRule(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "规则已删除"})
}

// EnableRule enables a firewall rule
func (h *FirewallHandler) EnableRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	// Check existence
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		NotFound(c, "规则不存在")
		return
	}

	// Check if enabling this rule would block a protected port
	if rule.Action != "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		BadRequest(c, fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法启用 DROP/REJECT 规则", rule.Port))
		return
	}

	if err := h.firewallService.EnableRule(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "规则已启用"})
}

// DisableRule disables a firewall rule
func (h *FirewallHandler) DisableRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	// Check existence
	if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
		NotFound(c, "规则不存在")
		return
	}
	if err := h.firewallService.DisableRule(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "规则已禁用"})
}

// MoveRuleUp moves a rule up in priority
func (h *FirewallHandler) MoveRuleUp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
		NotFound(c, "规则不存在")
		return
	}
	if err := h.firewallService.MoveRuleUp(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "规则已上移"})
}

// MoveRuleDown moves a rule down in priority
func (h *FirewallHandler) MoveRuleDown(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}
	if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
		NotFound(c, "规则不存在")
		return
	}
	if err := h.firewallService.MoveRuleDown(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "规则已下移"})
}

// BulkEnableRules enables multiple firewall rules
func (h *FirewallHandler) BulkEnableRules(c *gin.Context) {
	var req model.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	var succeeded, failed []int64
	var errors []string
	for _, id := range req.IDs {
		rule, err := h.firewallService.GetRule(c.Request.Context(), id)
		if err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: not found", id))
			continue
		}
		if rule.Action != "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: port %s is protected", id, rule.Port))
			continue
		}
		if err := h.firewallService.EnableRule(c.Request.Context(), id); err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: %s", id, err.Error()))
			continue
		}
		succeeded = append(succeeded, id)
	}

	Success(c, gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	})
}

// BulkDisableRules disables multiple firewall rules
func (h *FirewallHandler) BulkDisableRules(c *gin.Context) {
	var req model.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	var succeeded, failed []int64
	var errors []string
	for _, id := range req.IDs {
		if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: not found", id))
			continue
		}
		if err := h.firewallService.DisableRule(c.Request.Context(), id); err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: %s", id, err.Error()))
			continue
		}
		succeeded = append(succeeded, id)
	}

	Success(c, gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	})
}

// BulkDeleteRules deletes multiple firewall rules
func (h *FirewallHandler) BulkDeleteRules(c *gin.Context) {
	var req model.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	var succeeded, failed []int64
	var errors []string
	for _, id := range req.IDs {
		if _, err := h.firewallService.GetRule(c.Request.Context(), id); err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: not found", id))
			continue
		}
		if err := h.firewallService.DeleteRule(c.Request.Context(), id); err != nil {
			failed = append(failed, id)
			errors = append(errors, fmt.Sprintf("id %d: %s", id, err.Error()))
			continue
		}
		succeeded = append(succeeded, id)
	}

	Success(c, gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	})
}

// GetSystemRules returns rules from the system
func (h *FirewallHandler) GetSystemRules(c *gin.Context) {
	rules, err := h.firewallService.GetSystemRules(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rules)
}

// DeleteSystemRule deletes a rule directly from the system (not from database)
func (h *FirewallHandler) DeleteSystemRule(c *gin.Context) {
	var rule model.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		BadRequest(c, "invalid request: "+err.Error())
		return
	}

	// SAFETY: Prevent deleting ACCEPT rules for protected ports (SSH, panel).
	// Removing an ACCEPT rule for a protected port could lock out the user
	// if the default policy is DROP.
	if strings.ToUpper(rule.Action) == "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		BadRequest(c, fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法删除其 ACCEPT 规则", rule.Port))
		return
	}

	if err := h.firewallService.RemoveSystemRule(c.Request.Context(), &rule); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

// FirewallExportData is the export file format
type FirewallExportData struct {
	Version    int                    `json:"version"`
	ExportedAt string                 `json:"exported_at"`
	Rules      []FirewallExportedRule `json:"rules"`
}

// FirewallExportedRule is a single rule in the export file
type FirewallExportedRule struct {
	Chain     string `json:"chain"`
	Protocol  string `json:"protocol"`
	Port      string `json:"port"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	IPVersion string `json:"ip_version,omitempty"`
	Remark    string `json:"remark"`
}

// ExportRules exports all firewall rules as a downloadable JSON file
func (h *FirewallHandler) ExportRules(c *gin.Context) {
	rules, err := h.firewallService.ListRules(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	exportedRules := make([]FirewallExportedRule, 0, len(rules))
	for _, r := range rules {
		exportedRules = append(exportedRules, FirewallExportedRule{
			Chain:     r.Chain,
			Protocol:  r.Protocol,
			Port:      r.Port,
			Action:    r.Action,
			Source:    r.Source,
			IPVersion: r.IPVersion,
			Remark:    r.Remark,
		})
	}

	data := FirewallExportData{
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Rules:      exportedRules,
	}

	filename := fmt.Sprintf("firewall-rules-%s.json", time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json")
	c.JSON(200, data)
}

// ImportRules imports firewall rules from a JSON file
func (h *FirewallHandler) ImportRules(c *gin.Context) {
	var data FirewallExportData
	if err := c.ShouldBindJSON(&data); err != nil {
		BadRequest(c, "无效的 JSON: "+err.Error())
		return
	}

	if data.Version != 1 {
		BadRequest(c, fmt.Sprintf("不支持的导出版本: %d，期望版本 1", data.Version))
		return
	}

	if len(data.Rules) == 0 {
		BadRequest(c, "没有可导入的规则")
		return
	}

	// Limit import size to prevent abuse
	const maxImportRules = 500
	if len(data.Rules) > maxImportRules {
		BadRequest(c, fmt.Sprintf("导入规则数量过多: %d（最大 %d）", len(data.Rules), maxImportRules))
		return
	}

	validChains := map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true}
	validActions := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
	validProtocols := map[string]bool{"tcp": true, "udp": true, "all": true, "icmp": true}

	var succeeded, failed int
	var errors []string

	for i, r := range data.Rules {
		// Validate chain
		if !validChains[strings.ToUpper(r.Chain)] {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid chain '%s'", i+1, r.Chain))
			continue
		}

		// Validate action
		action := strings.ToUpper(r.Action)
		if !validActions[action] {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid action '%s'", i+1, r.Action))
			continue
		}

		// Validate protocol
		protocol := strings.ToLower(r.Protocol)
		if protocol == "" {
			protocol = "tcp"
		}
		if !validProtocols[protocol] {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid protocol '%s'", i+1, r.Protocol))
			continue
		}

		// Validate port format if provided
		if r.Port != "" && !isValidPort(r.Port) {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid port '%s'", i+1, r.Port))
			continue
		}

		// Check protected port
		if action != "ACCEPT" && r.Port != "" && h.isProtectedPort(c, r.Port) {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: port %s is protected", i+1, r.Port))
			continue
		}

		// Validate source IP/CIDR if provided
		if r.Source != "" && r.Source != "0.0.0.0/0" && r.Source != "::/0" && !isValidCIDR(r.Source) {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid source '%s'", i+1, r.Source))
			continue
		}

		// Validate ip_version
		ipVersion := r.IPVersion
		if ipVersion == "" {
			ipVersion = "ipv4"
		}
		validIPVersions := map[string]bool{"ipv4": true, "ipv6": true, "both": true}
		if !validIPVersions[ipVersion] {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: invalid ip_version '%s'", i+1, r.IPVersion))
			continue
		}

		rule := &model.FirewallRule{
			Chain:     strings.ToUpper(r.Chain),
			Protocol:  protocol,
			Port:      r.Port,
			Action:    action,
			Source:    r.Source,
			Enabled:   true,
			IPVersion: ipVersion,
			Remark:    r.Remark,
		}

		if err := h.firewallService.CreateRule(c.Request.Context(), rule); err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("rule %d: %s", i+1, err.Error()))
			continue
		}
		succeeded++
	}

	Success(c, gin.H{
		"succeeded": succeeded,
		"failed":    failed,
		"errors":    errors,
	})
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
func (h *FirewallHandler) GetTemplates(c *gin.Context) {
	Success(c, firewallRuleTemplates)
}

// ApplyTemplate creates a firewall rule from a named template
func (h *FirewallHandler) ApplyTemplate(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "请提供模板名称")
		return
	}

	// Find the template
	var tpl *FirewallRuleTemplate
	for i := range firewallRuleTemplates {
		if firewallRuleTemplates[i].Name == req.Name {
			tpl = &firewallRuleTemplates[i]
			break
		}
	}
	if tpl == nil {
		BadRequest(c, fmt.Sprintf("未找到模板: %s", req.Name))
		return
	}

	// Check if the template would block a protected port
	if tpl.Action != "ACCEPT" && tpl.Port != "" && h.isProtectedPort(c, tpl.Port) {
		BadRequest(c, fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", tpl.Port))
		return
	}

	rule := &model.FirewallRule{
		Chain:     "INPUT",
		Protocol:  tpl.Protocol,
		Port:      tpl.Port,
		Action:    tpl.Action,
		Enabled:   true,
		IPVersion: "ipv4",
		Remark:    tpl.Remark,
	}

	if err := h.firewallService.CreateRule(c.Request.Context(), rule); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rule)
}

// SetDefaultPolicy sets the default policy for a chain (INPUT or OUTPUT)
func (h *FirewallHandler) SetDefaultPolicy(c *gin.Context) {
	var req model.SetDefaultPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Validate chain
	chain := strings.ToUpper(req.Chain)
	if chain != "INPUT" && chain != "OUTPUT" {
		BadRequest(c, "无效的链，必须是 INPUT 或 OUTPUT")
		return
	}

	// Validate policy
	policy := strings.ToUpper(req.Policy)
	if policy != "ACCEPT" && policy != "DROP" {
		BadRequest(c, "无效的策略，必须是 ACCEPT 或 DROP")
		return
	}

	if err := h.firewallService.SetDefaultPolicy(c.Request.Context(), chain, policy); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": fmt.Sprintf("%s 的默认策略已设置为 %s", chain, policy)})
}

// GetLogs returns firewall log entries from system log files
func (h *FirewallHandler) GetLogs(c *gin.Context) {
	linesParam := c.DefaultQuery("lines", "100")
	lines, err := strconv.Atoi(linesParam)
	if err != nil || lines < 1 {
		lines = 100
	}
	if lines > 1000 {
		lines = 1000
	}

	logFiles := []string{
		"/var/log/ufw.log",
		"/var/log/kern.log",
		"/var/log/messages",
	}

	var allEntries []model.FirewallLogEntry

	for _, logFile := range logFiles {
		entries, err := readFirewallLog(logFile, lines)
		if err != nil {
			continue // Skip files that can't be read
		}
		allEntries = append(allEntries, entries...)
	}

	// Sort by timestamp descending (newest first) and limit
	if len(allEntries) > lines {
		// Simple truncation - entries are already appended in file order
		// For better sorting we'd need to parse timestamps, but keep it simple
		allEntries = allEntries[len(allEntries)-lines:]
	}

	// Reverse to show newest first
	for i, j := 0, len(allEntries)-1; i < j; i, j = i+1, j-1 {
		allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
	}

	Success(c, allEntries)
}

// iptables log pattern: kernel: [12345.678] IN=eth0 OUT= MAC=... SRC=1.2.3.4 DST=5.6.7.8 LEN=... PROTO=TCP SPT=12345 DPT=22 ...
var iptablesLogRegex = regexp.MustCompile(`SRC=(\S+)\s+DST=(\S+)\s+.*?PROTO=(\S+)`)

// ufw log pattern: [UFW BLOCK] IN=eth0 OUT= MAC=... SRC=1.2.3.4 DST=5.6.7.8 LEN=... PROTO=TCP SPT=12345 DPT=22 ...
var ufwActionRegex = regexp.MustCompile(`\[UFW\s+(\w+)\]`)

// readFirewallLog reads a log file and parses firewall entries.
// Uses a ring buffer to keep only the last maxLines matching lines in memory.
func readFirewallLog(filePath string, maxLines int) ([]model.FirewallLogEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Ring buffer to keep only the last maxLines lines
	lines := make([]string, 0, maxLines)
	writeIdx := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isFirewallLogLine(line) {
			if len(lines) < maxLines {
				lines = append(lines, line)
			} else {
				lines[writeIdx] = line
			}
			writeIdx = (writeIdx + 1) % maxLines
		}
	}

	// Reorder ring buffer to chronological order
	if len(lines) == maxLines {
		// Rotate so that writeIdx points to the oldest entry
		ordered := make([]string, len(lines))
		for i := range lines {
			ordered[i] = lines[(writeIdx+i)%len(lines)]
		}
		lines = ordered
	}

	// Parse lines into entries
	entries := make([]model.FirewallLogEntry, 0, len(lines))
	for _, line := range lines {
		entry := parseFirewallLogLine(line)
		if entry != nil {
			entries = append(entries, *entry)
		}
	}

	return entries, nil
}

// isFirewallLogLine checks if a log line is related to firewall activity
func isFirewallLogLine(line string) bool {
	// Check for ufw log entries
	if strings.Contains(line, "[UFW ") {
		return true
	}
	// Check for iptables/nft log entries (common patterns)
	lower := strings.ToLower(line)
	if strings.Contains(lower, "iptables") || strings.Contains(lower, "nftables") {
		return true
	}
	// Check for kernel firewall log entries with SRC/DST pattern
	if strings.Contains(line, "SRC=") && strings.Contains(line, "DST=") && strings.Contains(line, "PROTO=") {
		return true
	}
	return false
}

// parseFirewallLogLine parses a single firewall log line into a structured entry
func parseFirewallLogLine(line string) *model.FirewallLogEntry {
	entry := &model.FirewallLogEntry{
		Raw: line,
	}

	// Parse timestamp (first field in most log formats)
	parts := strings.SplitN(line, " ", 4)
	if len(parts) >= 3 {
		// Typical format: "Jun 23 19:00:00 hostname ..."
		entry.Timestamp = strings.Join(parts[:3], " ")
	}

	// Parse action
	if ufwActionRegex.MatchString(line) {
		matches := ufwActionRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			entry.Action = strings.ToUpper(matches[1])
		}
	} else if strings.Contains(line, "DROP") {
		entry.Action = "DROP"
	} else if strings.Contains(line, "REJECT") {
		entry.Action = "REJECT"
	} else if strings.Contains(line, "ACCEPT") {
		entry.Action = "ACCEPT"
	} else {
		entry.Action = "LOG"
	}

	// Parse SRC, DST, PROTO
	if iptablesLogRegex.MatchString(line) {
		matches := iptablesLogRegex.FindStringSubmatch(line)
		if len(matches) > 3 {
			entry.SrcIP = matches[1]
			entry.DstIP = matches[2]
			entry.Protocol = strings.ToLower(matches[3])
		}
	}

	// Parse ports
	entry.DstPort = extractPort(line, "DPT=")
	entry.SrcPort = extractPort(line, "SPT=")

	// Parse interface
	if idx := strings.Index(line, "IN="); idx >= 0 {
		rest := line[idx+3:]
		if endIdx := strings.Index(rest, " "); endIdx >= 0 {
			entry.Interface = rest[:endIdx]
		}
	}

	return entry
}

// extractPort extracts a port number from a log line given a prefix like "DPT="
func extractPort(line, prefix string) int {
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return 0
	}
	rest := line[idx+len(prefix):]
	endIdx := strings.Index(rest, " ")
	var portStr string
	if endIdx >= 0 {
		portStr = rest[:endIdx]
	} else {
		portStr = rest
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// isValidPort validates port format (single port or range)
func isValidPort(port string) bool {
	// Single port: 80
	if p, err := strconv.Atoi(port); err == nil {
		return p > 0 && p <= 65535
	}
	// Port range: 8000-9000
	parts := strings.Split(port, "-")
	if len(parts) == 2 {
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && start > 0 && end > start && start <= 65535 && end <= 65535 {
			return true
		}
	}
	return false
}

// isValidCIDR validates IP or CIDR format
func isValidCIDR(s string) bool {
	// Single IP: 192.168.1.1
	if net.ParseIP(s) != nil {
		return true
	}
	// CIDR: 192.168.1.0/24
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	return false
}
