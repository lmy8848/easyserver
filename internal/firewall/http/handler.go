package http

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/firewall"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// ============================================================
// FirewallHandler — firewall status, control, and log API
// ============================================================

// FirewallHandler handles firewall status, control, and log API requests
type FirewallHandler struct {
	firewallService *firewall.Service
	panelPort       string
}

// NewFirewallHandler creates a new FirewallHandler
func NewFirewallHandler(firewallService *firewall.Service, panelPort int) *FirewallHandler {
	return &FirewallHandler{
		firewallService: firewallService,
		panelPort:       strconv.Itoa(panelPort),
	}
}

// GetStatus returns the firewall status
func (h *FirewallHandler) GetStatus(c *gin.Context) (any, error) {
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return status, nil
}

// EnableFirewall enables the firewall
func (h *FirewallHandler) EnableFirewall(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "启用防火墙")
	if err := h.firewallService.EnableFirewall(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "防火墙已启用"}, nil
}

// DisableFirewall disables the firewall
func (h *FirewallHandler) DisableFirewall(c *gin.Context) (any, error) {
	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		return nil, errx.BadRequest("请确认禁用防火墙，设置 {\"confirm\": true}")
	}

	middleware.AuditSummary(c, "禁用防火墙")
	if err := h.firewallService.DisableFirewall(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "防火墙已禁用"}, nil
}

// SetDefaultPolicy sets the default policy for a chain (INPUT or OUTPUT)
func (h *FirewallHandler) SetDefaultPolicy(c *gin.Context) (any, error) {
	var req firewall.SetDefaultPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "设置防火墙默认策略 "+req.Chain+" "+req.Policy)
	// Validate chain
	chain := strings.ToUpper(req.Chain)
	if chain != "INPUT" && chain != "OUTPUT" {
		return nil, errx.BadRequest("无效的链，必须是 INPUT 或 OUTPUT")
	}

	// Validate policy
	policy := strings.ToUpper(req.Policy)
	if policy != "ACCEPT" && policy != "DROP" {
		return nil, errx.BadRequest("无效的策略，必须是 ACCEPT 或 DROP")
	}

	if err := h.firewallService.SetDefaultPolicy(c.Request.Context(), chain, policy); err != nil {
		return nil, err
	}

	return gin.H{"message": fmt.Sprintf("%s 的默认策略已设置为 %s", chain, policy)}, nil
}

// GetLogs returns firewall log entries from system log files
func (h *FirewallHandler) GetLogs(c *gin.Context) (any, error) {
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

	var allEntries []firewall.FirewallLogEntry

	for _, logFile := range logFiles {
		entries, err := readFirewallLog(logFile, lines)
		if err != nil {
			continue // Skip files that can't be read
		}
		allEntries = append(allEntries, entries...)
	}

	// Sort by timestamp descending (newest first) and limit
	if len(allEntries) > lines {
		allEntries = allEntries[len(allEntries)-lines:]
	}

	// Reverse to show newest first
	for i, j := 0, len(allEntries)-1; i < j; i, j = i+1, j-1 {
		allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
	}

	return allEntries, nil
}

// ufw log pattern: [UFW BLOCK] IN=eth0 OUT= MAC=... SRC=1.2.3.4 DST=5.6.7.8 LEN=... PROTO=TCP SPT=12345 DPT=22 ...
var ufwActionRegex = regexp.MustCompile(`\[UFW\s+(\w+)\]`)

// readFirewallLog reads a log file and parses firewall entries.
// Uses a ring buffer to keep only the last maxLines matching lines in memory.
func readFirewallLog(filePath string, maxLines int) ([]firewall.FirewallLogEntry, error) {
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
		ordered := make([]string, len(lines))
		for i := range lines {
			ordered[i] = lines[(writeIdx+i)%len(lines)]
		}
		lines = ordered
	}

	// Parse lines into entries
	entries := make([]firewall.FirewallLogEntry, 0, len(lines))
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
	if strings.Contains(line, "[UFW ") {
		return true
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "iptables") || strings.Contains(lower, "nftables") {
		return true
	}
	if strings.Contains(line, "SRC=") && strings.Contains(line, "DST=") && strings.Contains(line, "PROTO=") {
		return true
	}
	return false
}

// parseFirewallLogLine parses a single firewall log line into a structured entry
func parseFirewallLogLine(line string) *firewall.FirewallLogEntry {
	entry := &firewall.FirewallLogEntry{
		Raw: line,
	}

	parts := strings.SplitN(line, " ", 4)
	if len(parts) >= 3 {
		entry.Timestamp = strings.Join(parts[:3], " ")
	}

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

	if strings.Contains(line, "PROTO=TCP") {
		entry.Protocol = "tcp"
	} else if strings.Contains(line, "PROTO=UDP") {
		entry.Protocol = "udp"
	} else if strings.Contains(line, "PROTO=ICMP") {
		entry.Protocol = "icmp"
	}

	dptRegex := regexp.MustCompile(`DPT=(\d+)`)
	if matches := dptRegex.FindStringSubmatch(line); len(matches) > 1 {
		entry.DstPort, _ = strconv.Atoi(matches[1])
	}

	sptRegex := regexp.MustCompile(`SPT=(\d+)`)
	if matches := sptRegex.FindStringSubmatch(line); len(matches) > 1 {
		entry.SrcPort, _ = strconv.Atoi(matches[1])
	}

	srcRegex := regexp.MustCompile(`SRC=(\S+)`)
	if matches := srcRegex.FindStringSubmatch(line); len(matches) > 1 {
		entry.SrcIP = matches[1]
	}

	dstRegex := regexp.MustCompile(`DST=(\S+)`)
	if matches := dstRegex.FindStringSubmatch(line); len(matches) > 1 {
		entry.DstIP = matches[1]
	}

	inRegex := regexp.MustCompile(`IN=(\S*)`)
	if matches := inRegex.FindStringSubmatch(line); len(matches) > 1 && matches[1] != "" {
		entry.Interface = matches[1]
	}

	return entry
}

// Helper validation functions

func isValidPort(port string) bool {
	if port == "all" || port == "ALL" {
		return true
	}
	if strings.Contains(port, "-") {
		parts := strings.Split(port, "-")
		if len(parts) != 2 {
			return false
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil || start < 1 || start > 65535 {
			return false
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil || end < 1 || end > 65535 || start >= end {
			return false
		}
		return true
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return p >= 1 && p <= 65535
}

func isValidCIDR(cidr string) bool {
	if cidr == "" || cidr == "0.0.0.0/0" || cidr == "::/0" {
		return true
	}
	if ip := net.ParseIP(cidr); ip != nil {
		return true
	}
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// ============================================================
// FirewallRuleHandler — firewall rules CRUD operations
// ============================================================

// FirewallRuleHandler handles firewall rules CRUD operations
type FirewallRuleHandler struct {
	firewallService *firewall.Service
}

func NewFirewallRuleHandler(firewallService *firewall.Service) *FirewallRuleHandler {
	return &FirewallRuleHandler{
		firewallService: firewallService,
	}
}

// isProtectedPort checks if a port is protected (panel port or SSH)
func (h *FirewallRuleHandler) isProtectedPort(c *gin.Context, port string) bool {
	return h.firewallService.IsProtectedPort(c.Request.Context(), port)
}

// ListRules returns all firewall rules
func (h *FirewallRuleHandler) ListRules(c *gin.Context) (any, error) {
	rules, err := h.firewallService.ListRules(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// GetRule returns a firewall rule by ID
func (h *FirewallRuleHandler) GetRule(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	return rule, nil
}

// CreateRule creates a new firewall rule
func (h *FirewallRuleHandler) CreateRule(c *gin.Context) (any, error) {
	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("获取防火墙状态失败")
	}
	if !status.Enabled {
		return nil, errx.BadRequest("防火墙已禁用，请先启用防火墙")
	}

	var req firewall.CreateFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "创建防火墙规则 "+req.Port)
	// Validate chain
	validChains := map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true}
	if !validChains[strings.ToUpper(req.Chain)] {
		return nil, errx.BadRequest("无效的链，必须是 INPUT、OUTPUT 或 FORWARD")
	}

	// Validate action
	validActions := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
	action := strings.ToUpper(req.Action)
	if !validActions[action] {
		return nil, errx.BadRequest("无效的动作，必须是 ACCEPT、DROP 或 REJECT")
	}

	// Set defaults
	protocol := req.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	validProtocols := map[string]bool{"tcp": true, "udp": true, "all": true, "icmp": true}
	if !validProtocols[strings.ToLower(protocol)] {
		return nil, errx.BadRequest("无效的协议，必须是 tcp、udp、all 或 icmp")
	}

	// Validate port format if provided
	if req.Port != "" {
		if !isValidPort(req.Port) {
			return nil, errx.BadRequest("无效的端口格式，使用单端口 (80) 或范围 (8000-9000)")
		}

		// Check if port is protected (panel port or SSH)
		if action != "ACCEPT" && h.isProtectedPort(c, req.Port) {
			return nil, errx.BadRequest(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", req.Port))
		}
	}

	// Validate source IP/CIDR if provided
	if req.Source != "" && req.Source != "0.0.0.0/0" && req.Source != "::/0" {
		if !isValidCIDR(req.Source) {
			return nil, errx.BadRequest("无效的源地址格式，使用 IP (192.168.1.1) 或 CIDR (192.168.1.0/24)")
		}
	}

	// Validate and default ip_version
	ipVersion := req.IPVersion
	if ipVersion == "" {
		ipVersion = "ipv4"
	}
	validIPVersions := map[string]bool{"ipv4": true, "ipv6": true, "both": true}
	if !validIPVersions[ipVersion] {
		return nil, errx.BadRequest("无效的 ip_version，必须是 ipv4、ipv6 或 both")
	}

	rule := &firewall.FirewallRule{
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
		return nil, err
	}
	return rule, nil
}

// UpdateRule updates an existing firewall rule
func (h *FirewallRuleHandler) UpdateRule(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}

	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}

	var req firewall.UpdateFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	summary := "更新防火墙规则 (端口: " + rule.Port + ")"
	if req.Port != nil && *req.Port != "" && *req.Port != rule.Port {
		summary = "更新防火墙规则端口 " + rule.Port + " -> " + *req.Port
	}
	middleware.AuditSummary(c, summary)
	if req.Chain != nil {
		validChains := map[string]bool{"INPUT": true, "OUTPUT": true, "FORWARD": true}
		if !validChains[strings.ToUpper(*req.Chain)] {
			return nil, errx.BadRequest("无效的链")
		}
		rule.Chain = strings.ToUpper(*req.Chain)
	}
	if req.Protocol != nil {
		validProtocols := map[string]bool{"tcp": true, "udp": true, "all": true, "icmp": true}
		if !validProtocols[strings.ToLower(*req.Protocol)] {
			return nil, errx.BadRequest("无效的协议")
		}
		rule.Protocol = strings.ToLower(*req.Protocol)
	}
	if req.Port != nil {
		if *req.Port != "" && !isValidPort(*req.Port) {
			return nil, errx.BadRequest("无效的端口格式")
		}
		rule.Port = *req.Port
	}
	if req.Action != nil {
		validActions := map[string]bool{"ACCEPT": true, "DROP": true, "REJECT": true}
		if !validActions[strings.ToUpper(*req.Action)] {
			return nil, errx.BadRequest("无效的动作")
		}
		rule.Action = strings.ToUpper(*req.Action)
	}
	if req.Source != nil {
		if *req.Source != "" && *req.Source != "0.0.0.0/0" && *req.Source != "::/0" && !isValidCIDR(*req.Source) {
			return nil, errx.BadRequest("无效的源地址格式，使用 IP (192.168.1.1) 或 CIDR (192.168.1.0/24)")
		}
		rule.Source = *req.Source
	}
	if req.IPVersion != nil {
		validIPVersions := map[string]bool{"ipv4": true, "ipv6": true, "both": true}
		if !validIPVersions[*req.IPVersion] {
			return nil, errx.BadRequest("无效的 ip_version，必须是 ipv4、ipv6 或 both")
		}
		rule.IPVersion = *req.IPVersion
	}
	if req.Remark != nil {
		rule.Remark = *req.Remark
	}

	// Check if the updated rule would block a protected port
	if rule.Action != "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		return nil, errx.BadRequest(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", rule.Port))
	}

	if err := h.firewallService.UpdateRule(c.Request.Context(), rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// DeleteRule deletes a firewall rule
func (h *FirewallRuleHandler) DeleteRule(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}
	// Check existence
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	middleware.AuditSummary(c, "删除防火墙规则 (端口: "+rule.Port+")")
	if err := h.firewallService.DeleteRule(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "规则已删除"}, nil
}

// EnableRule enables a firewall rule
func (h *FirewallRuleHandler) EnableRule(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}

	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("获取防火墙状态失败")
	}
	if !status.Enabled {
		return nil, errx.BadRequest("防火墙已禁用，请先启用防火墙")
	}

	// Check existence
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	middleware.AuditSummary(c, "启用防火墙规则 (端口: "+rule.Port+")")

	// Check if enabling this rule would block a protected port
	if rule.Action != "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		return nil, errx.BadRequest(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法启用 DROP/REJECT 规则", rule.Port))
	}

	if err := h.firewallService.EnableRule(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "规则已启用"}, nil
}

// DisableRule disables a firewall rule
func (h *FirewallRuleHandler) DisableRule(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}
	// Check existence
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	middleware.AuditSummary(c, "禁用防火墙规则 (端口: "+rule.Port+")")
	if err := h.firewallService.DisableRule(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "规则已禁用"}, nil
}

// MoveRuleUp moves a rule up in priority
func (h *FirewallRuleHandler) MoveRuleUp(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	middleware.AuditSummary(c, "上移防火墙规则 (端口: "+rule.Port+")")
	if err := h.firewallService.MoveRuleUp(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "规则已上移"}, nil
}

// MoveRuleDown moves a rule down in priority
func (h *FirewallRuleHandler) MoveRuleDown(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的规则ID")
	}
	rule, err := h.firewallService.GetRule(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("规则不存在")
	}
	middleware.AuditSummary(c, "下移防火墙规则 (端口: "+rule.Port+")")
	if err := h.firewallService.MoveRuleDown(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "规则已下移"}, nil
}

// BulkEnableRules enables multiple firewall rules
func (h *FirewallRuleHandler) BulkEnableRules(c *gin.Context) (any, error) {
	// Check if firewall is enabled
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("获取防火墙状态失败")
	}
	if !status.Enabled {
		return nil, errx.BadRequest("防火墙已禁用，请先启用防火墙")
	}

	var req firewall.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "批量启用防火墙规则 "+strconv.Itoa(len(req.IDs))+" 条")
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

	return gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	}, nil
}

// BulkDisableRules disables multiple firewall rules
func (h *FirewallRuleHandler) BulkDisableRules(c *gin.Context) (any, error) {
	var req firewall.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "批量禁用防火墙规则 "+strconv.Itoa(len(req.IDs))+" 条")
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

	return gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	}, nil
}

// BulkDeleteRules deletes multiple firewall rules
func (h *FirewallRuleHandler) BulkDeleteRules(c *gin.Context) (any, error) {
	var req firewall.BulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "批量删除防火墙规则 "+strconv.Itoa(len(req.IDs))+" 条")
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

	return gin.H{
		"succeeded": len(succeeded),
		"failed":    len(failed),
		"errors":    errors,
	}, nil
}

// GetSystemRules returns rules from the system
func (h *FirewallRuleHandler) GetSystemRules(c *gin.Context) (any, error) {
	rules, err := h.firewallService.GetSystemRules(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// DeleteSystemRule deletes a rule directly from the system (not from database)
func (h *FirewallRuleHandler) DeleteSystemRule(c *gin.Context) (any, error) {
	var rule firewall.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "删除系统防火墙规则 "+rule.Port)
	// SAFETY: Prevent deleting ACCEPT rules for protected ports (SSH, panel).
	// Removing an ACCEPT rule for a protected port could lock out the user
	// if the default policy is DROP.
	if strings.ToUpper(rule.Action) == "ACCEPT" && rule.Port != "" && h.isProtectedPort(c, rule.Port) {
		return nil, errx.BadRequest(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法删除其 ACCEPT 规则", rule.Port))
	}

	if err := h.firewallService.RemoveSystemRule(c.Request.Context(), &rule); err != nil {
		return nil, err
	}

	return nil, nil
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
func (h *FirewallRuleHandler) ExportRules(c *gin.Context) (any, error) {
	rules, err := h.firewallService.ListRules(c.Request.Context())
	if err != nil {
		return nil, err
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
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/json")
	return data, nil
}

// ImportRules imports firewall rules from a JSON file
func (h *FirewallRuleHandler) ImportRules(c *gin.Context) (any, error) {
	var data FirewallExportData
	if err := c.ShouldBindJSON(&data); err != nil {
		return nil, errx.BadRequest("无效的 JSON: %w", err)
	}

	middleware.AuditSummary(c, "导入防火墙规则 "+strconv.Itoa(len(data.Rules))+" 条")
	if data.Version != 1 {
		return nil, errx.BadRequest(fmt.Sprintf("不支持的导出版本: %d，期望版本 1", data.Version))
	}

	if len(data.Rules) == 0 {
		return nil, errx.BadRequest("没有可导入的规则")
	}

	// Limit import size to prevent abuse
	const maxImportRules = 500
	if len(data.Rules) > maxImportRules {
		return nil, errx.BadRequest(fmt.Sprintf("导入规则数量过多: %d（最大 %d）", len(data.Rules), maxImportRules))
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

		rule := &firewall.FirewallRule{
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

	return gin.H{
		"succeeded": succeeded,
		"failed":    failed,
		"errors":    errors,
	}, nil
}

// ============================================================
// FirewallTemplateHandler — firewall rule template operations
// ============================================================

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
func (h *FirewallTemplateHandler) GetTemplates(c *gin.Context) (any, error) {
	return firewallRuleTemplates, nil
}

// ApplyTemplate creates a firewall rule from a named template
func (h *FirewallTemplateHandler) ApplyTemplate(c *gin.Context) (any, error) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("请提供模板名称")
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
		return nil, errx.BadRequest("未找到模板: " + req.Name)
	}

	// Check if the template would block a protected port
	if tpl.Action != "ACCEPT" && tpl.Port != "" && h.isProtectedPort(c, tpl.Port) {
		return nil, errx.BadRequest(fmt.Sprintf("端口 %s 受保护（面板或 SSH），无法创建 DROP/REJECT 规则", tpl.Port))
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
		return nil, err
	}
	return rule, nil
}

// ============================================================
// Route registration
// ============================================================

func RegisterRoutes(protected *gin.RouterGroup, firewallService *firewall.Service, panelPort int) {
	// Set protected ports: SSH (22) + panel port from config
	firewallService.SetProtectedPorts([]string{"22", strconv.Itoa(panelPort)})

	// Create sub-handlers
	handler := NewFirewallHandler(firewallService, panelPort)
	ruleHandler := NewFirewallRuleHandler(firewallService)
	templateHandler := NewFirewallTemplateHandler(firewallService)

	// Status and control
	protected.GET("/firewall/status", httpx.H(handler.GetStatus))
	protected.POST("/firewall/enable", httpx.H(handler.EnableFirewall))
	protected.POST("/firewall/disable", httpx.H(handler.DisableFirewall))
	protected.POST("/firewall/default-policy", httpx.H(handler.SetDefaultPolicy))
	protected.GET("/firewall/logs", httpx.H(handler.GetLogs))

	// Rules CRUD
	protected.GET("/firewall/rules", httpx.H(ruleHandler.ListRules))
	protected.POST("/firewall/rules", httpx.H(ruleHandler.CreateRule))
	protected.GET("/firewall/rules/export", httpx.H(ruleHandler.ExportRules))
	protected.POST("/firewall/rules/import", httpx.H(ruleHandler.ImportRules))
	protected.POST("/firewall/rules/bulk-enable", httpx.H(ruleHandler.BulkEnableRules))
	protected.POST("/firewall/rules/bulk-disable", httpx.H(ruleHandler.BulkDisableRules))
	protected.POST("/firewall/rules/bulk-delete", httpx.H(ruleHandler.BulkDeleteRules))
	protected.GET("/firewall/rules/:id", httpx.H(ruleHandler.GetRule))
	protected.PUT("/firewall/rules/:id", httpx.H(ruleHandler.UpdateRule))
	protected.DELETE("/firewall/rules/:id", httpx.H(ruleHandler.DeleteRule))
	protected.POST("/firewall/rules/:id/enable", httpx.H(ruleHandler.EnableRule))
	protected.POST("/firewall/rules/:id/disable", httpx.H(ruleHandler.DisableRule))
	protected.POST("/firewall/rules/:id/move-up", httpx.H(ruleHandler.MoveRuleUp))
	protected.POST("/firewall/rules/:id/move-down", httpx.H(ruleHandler.MoveRuleDown))

	// System rules
	protected.GET("/firewall/system-rules", httpx.H(ruleHandler.GetSystemRules))
	protected.POST("/firewall/system-rules/delete", httpx.H(ruleHandler.DeleteSystemRule))

	// Templates
	protected.GET("/firewall/templates", httpx.H(templateHandler.GetTemplates))
	protected.POST("/firewall/templates/apply", httpx.H(templateHandler.ApplyTemplate))
}
