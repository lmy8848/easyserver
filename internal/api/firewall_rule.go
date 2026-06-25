package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// FirewallRuleHandler handles firewall rule CRUD operations
type FirewallRuleHandler struct {
	firewallService *service.FirewallService
}

// NewFirewallRuleHandler creates a new FirewallRuleHandler
func NewFirewallRuleHandler(firewallService *service.FirewallService) *FirewallRuleHandler {
	return &FirewallRuleHandler{
		firewallService: firewallService,
	}
}

// isProtectedPort checks if a port is protected (panel port or SSH)
func (h *FirewallRuleHandler) isProtectedPort(c *gin.Context, port string) bool {
	return h.firewallService.IsProtectedPort(c.Request.Context(), port)
}

// ListRules returns all firewall rules
func (h *FirewallRuleHandler) ListRules(c *gin.Context) {
	rules, err := h.firewallService.ListRules(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rules)
}

// GetRule returns a firewall rule by ID
func (h *FirewallRuleHandler) GetRule(c *gin.Context) {
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
func (h *FirewallRuleHandler) CreateRule(c *gin.Context) {
	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		InternalError(c, "获取防火墙状态失败")
		return
	}
	if !status.Enabled {
		BadRequest(c, "防火墙已禁用，请先启用防火墙")
		return
	}

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
func (h *FirewallRuleHandler) UpdateRule(c *gin.Context) {
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
func (h *FirewallRuleHandler) DeleteRule(c *gin.Context) {
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
func (h *FirewallRuleHandler) EnableRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的规则ID")
		return
	}

	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		InternalError(c, "获取防火墙状态失败")
		return
	}
	if !status.Enabled {
		BadRequest(c, "防火墙已禁用，请先启用防火墙")
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
func (h *FirewallRuleHandler) DisableRule(c *gin.Context) {
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
func (h *FirewallRuleHandler) MoveRuleUp(c *gin.Context) {
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
func (h *FirewallRuleHandler) MoveRuleDown(c *gin.Context) {
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
func (h *FirewallRuleHandler) BulkEnableRules(c *gin.Context) {
	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		InternalError(c, "获取防火墙状态失败")
		return
	}
	if !status.Enabled {
		BadRequest(c, "防火墙已禁用，请先启用防火墙")
		return
	}

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
func (h *FirewallRuleHandler) BulkDisableRules(c *gin.Context) {
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
func (h *FirewallRuleHandler) BulkDeleteRules(c *gin.Context) {
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
func (h *FirewallRuleHandler) GetSystemRules(c *gin.Context) {
	rules, err := h.firewallService.GetSystemRules(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, rules)
}

// DeleteSystemRule deletes a rule directly from the system (not from database)
func (h *FirewallRuleHandler) DeleteSystemRule(c *gin.Context) {
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
func (h *FirewallRuleHandler) ExportRules(c *gin.Context) {
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
func (h *FirewallRuleHandler) ImportRules(c *gin.Context) {
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
