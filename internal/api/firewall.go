package api

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"easyserver/internal/api/middleware"
	"easyserver/internal/firewall"

	"github.com/gin-gonic/gin"
)

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
func (h *FirewallHandler) GetStatus(c *gin.Context) {
	status, err := h.firewallService.GetStatus(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, status)
}

// EnableFirewall enables the firewall
func (h *FirewallHandler) EnableFirewall(c *gin.Context) {
	middleware.AuditSummary(c, "启用防火墙")
	if err := h.firewallService.EnableFirewall(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
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
		c.Error(ErrBadRequest.WithMessage("请确认禁用防火墙，设置 {\"confirm\": true}"))
		return
	}

	middleware.AuditSummary(c, "禁用防火墙")
	if err := h.firewallService.DisableFirewall(c.Request.Context()); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "防火墙已禁用"})
}

// SetDefaultPolicy sets the default policy for a chain (INPUT or OUTPUT)
func (h *FirewallHandler) SetDefaultPolicy(c *gin.Context) {
	var req firewall.SetDefaultPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "设置防火墙默认策略 "+req.Chain+" "+req.Policy)
	// Validate chain
	chain := strings.ToUpper(req.Chain)
	if chain != "INPUT" && chain != "OUTPUT" {
		c.Error(ErrBadRequest.WithMessage("无效的链，必须是 INPUT 或 OUTPUT"))
		return
	}

	// Validate policy
	policy := strings.ToUpper(req.Policy)
	if policy != "ACCEPT" && policy != "DROP" {
		c.Error(ErrBadRequest.WithMessage("无效的策略，必须是 ACCEPT 或 DROP"))
		return
	}

	if err := h.firewallService.SetDefaultPolicy(c.Request.Context(), chain, policy); err != nil {
		c.Error(WrapError(err))
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
		// Rotate so that writeIdx points to the oldest entry
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
func parseFirewallLogLine(line string) *firewall.FirewallLogEntry {
	entry := &firewall.FirewallLogEntry{
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

func registerFirewallRoutes(protected *gin.RouterGroup, firewallService *firewall.Service, panelPort int) {
	// Set protected ports: SSH (22) + panel port from config
	firewallService.SetProtectedPorts([]string{"22", strconv.Itoa(panelPort)})

	// Create sub-handlers
	handler := NewFirewallHandler(firewallService, panelPort)
	ruleHandler := NewFirewallRuleHandler(firewallService)
	templateHandler := NewFirewallTemplateHandler(firewallService)

	// Status and control
	protected.GET("/firewall/status", handler.GetStatus)
	protected.POST("/firewall/enable", handler.EnableFirewall)
	protected.POST("/firewall/disable", handler.DisableFirewall)
	protected.POST("/firewall/default-policy", handler.SetDefaultPolicy)
	protected.GET("/firewall/logs", handler.GetLogs)

	// Rules CRUD
	protected.GET("/firewall/rules", ruleHandler.ListRules)
	protected.POST("/firewall/rules", ruleHandler.CreateRule)
	protected.GET("/firewall/rules/export", ruleHandler.ExportRules)
	protected.POST("/firewall/rules/import", ruleHandler.ImportRules)
	protected.POST("/firewall/rules/bulk-enable", ruleHandler.BulkEnableRules)
	protected.POST("/firewall/rules/bulk-disable", ruleHandler.BulkDisableRules)
	protected.POST("/firewall/rules/bulk-delete", ruleHandler.BulkDeleteRules)
	protected.GET("/firewall/rules/:id", ruleHandler.GetRule)
	protected.PUT("/firewall/rules/:id", ruleHandler.UpdateRule)
	protected.DELETE("/firewall/rules/:id", ruleHandler.DeleteRule)
	protected.POST("/firewall/rules/:id/enable", ruleHandler.EnableRule)
	protected.POST("/firewall/rules/:id/disable", ruleHandler.DisableRule)
	protected.POST("/firewall/rules/:id/move-up", ruleHandler.MoveRuleUp)
	protected.POST("/firewall/rules/:id/move-down", ruleHandler.MoveRuleDown)

	// System rules
	protected.GET("/firewall/system-rules", ruleHandler.GetSystemRules)
	protected.POST("/firewall/system-rules/delete", ruleHandler.DeleteSystemRule)

	// Templates
	protected.GET("/firewall/templates", templateHandler.GetTemplates)
	protected.POST("/firewall/templates/apply", templateHandler.ApplyTemplate)
}
