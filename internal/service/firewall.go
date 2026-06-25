package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// FirewallService manages local firewall rules
type FirewallService struct {
	db             *sql.DB
	mu             sync.Mutex
	protectedPorts []string // Ports that cannot be blocked (panel port, SSH, etc.)
	executor       executor.CommandExecutor
	detectedTool   string // Cached firewall tool detection
}

// NewFirewallService creates a new FirewallService
func NewFirewallService(db *sql.DB, exec executor.CommandExecutor) *FirewallService {
	return &FirewallService{
		db:             db,
		protectedPorts: []string{"22"}, // SSH port is always protected
		executor:       exec,
	}
}

// SetProtectedPorts sets the list of protected ports
func (s *FirewallService) SetProtectedPorts(ports []string) {
	s.protectedPorts = ports
}

// IsProtectedPort checks if a port is protected
func (s *FirewallService) IsProtectedPort(ctx context.Context, port string) bool {
	if port == "" {
		return false
	}

	// Handle port ranges
	if strings.Contains(port, "-") {
		parts := strings.Split(port, "-")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				for _, protected := range s.protectedPorts {
					p, err := strconv.Atoi(protected)
					if err == nil && p >= start && p <= end {
						return true
					}
				}
			}
		}
		return false
	}

	// Single port
	for _, protected := range s.protectedPorts {
		if port == protected {
			return true
		}
	}
	return false
}

// DetectTool detects which firewall tool is available
func (s *FirewallService) DetectTool(ctx context.Context) string {
	// Cache the result (thread-safe)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.detectedTool != "" {
		return s.detectedTool
	}

	// Priority: ufw > nft > iptables
	tools := []string{"ufw", "nft", "iptables"}
	for _, tool := range tools {
		_, exitCode, err := s.executor.RunCombined(ctx, "which", tool)
		if err == nil && exitCode == 0 {
			s.detectedTool = tool
			return tool
		}
	}

	// Fallback: check if iptables is available via alternative paths
	for _, path := range []string{"/sbin/iptables", "/usr/sbin/iptables"} {
		_, exitCode, err := s.executor.RunCombined(ctx, "test", "-x", path)
		if err == nil && exitCode == 0 {
			s.detectedTool = "iptables"
			return "iptables"
		}
	}

	return ""
}

// GetStatus returns the current firewall status
func (s *FirewallService) GetStatus(ctx context.Context) (*model.FirewallStatus, error) {
	tool := s.DetectTool(ctx)
	if tool == "" {
		return &model.FirewallStatus{
			Enabled: false,
			Tool:    "none",
		}, nil
	}

	status := &model.FirewallStatus{
		Tool: tool,
	}

	switch tool {
	case "ufw":
		output, _, err := s.executor.RunCombined(ctx, "ufw", "status", "verbose")
		if err == nil {
			status.Enabled = strings.Contains(output, "Status: active")
			if strings.Contains(output, "Default: deny (incoming)") {
				status.DefaultIn = "DROP"
			} else {
				status.DefaultIn = "ACCEPT"
			}
			if strings.Contains(output, "Default: allow (outgoing)") {
				status.DefaultOut = "ACCEPT"
			} else {
				status.DefaultOut = "DROP"
			}
			// Count rules
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "Status:") && !strings.HasPrefix(line, "Default:") && !strings.HasPrefix(line, "Logging:") && !strings.HasPrefix(line, "---") {
					if strings.Contains(line, "ALLOW") || strings.Contains(line, "DENY") || strings.Contains(line, "REJECT") {
						status.RuleCount++
					}
				}
			}
		}

	case "nft":
		// Check if nft has rules loaded
		output, exitCode, err := s.executor.RunCombined(ctx, "nft", "list", "ruleset")
		if err == nil && exitCode == 0 {
			// nft is "enabled" if the table exists
			status.Enabled = strings.Contains(output, "table inet filter")

			// Get default policy from chain definition
			// Example: chain INPUT { type filter hook input priority filter; policy accept; }
			lines := strings.Split(output, "\n")
			currentChain := ""
			for _, line := range lines {
				line = strings.TrimSpace(line)

				// Detect chain name
				if strings.HasPrefix(line, "chain ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						currentChain = parts[1]
					}
				}

				// Detect policy
				if strings.Contains(line, "policy") {
					if strings.Contains(line, "policy accept") {
						if currentChain == "INPUT" {
							status.DefaultIn = "ACCEPT"
						} else if currentChain == "OUTPUT" {
							status.DefaultOut = "ACCEPT"
						}
					} else if strings.Contains(line, "policy drop") {
						if currentChain == "INPUT" {
							status.DefaultIn = "DROP"
						} else if currentChain == "OUTPUT" {
							status.DefaultOut = "DROP"
						}
					}
				}

				// Count rules (lines with accept/drop/reject that are actual rules)
				if strings.Contains(line, "accept") || strings.Contains(line, "drop") || strings.Contains(line, "reject") {
					// Skip chain headers and comments
					if !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "table") && !strings.HasPrefix(line, "chain") && !strings.Contains(line, "policy") {
						status.RuleCount++
					}
				}
			}

			// Set defaults if not detected
			if status.DefaultIn == "" {
				status.DefaultIn = "ACCEPT"
			}
			if status.DefaultOut == "" {
				status.DefaultOut = "ACCEPT"
			}
		}

	case "iptables":
		output, _, err := s.executor.RunCombined(ctx, "iptables", "-L", "-n", "--line-numbers")
		if err == nil {
			status.Enabled = true // iptables is always "enabled" if it exists
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				// Parse default policy from chain header
				// Example: Chain INPUT (policy ACCEPT)
				if strings.HasPrefix(line, "Chain") && strings.Contains(line, "policy") {
					if strings.Contains(line, "Chain INPUT") {
						if strings.Contains(line, "policy ACCEPT") {
							status.DefaultIn = "ACCEPT"
						} else if strings.Contains(line, "policy DROP") {
							status.DefaultIn = "DROP"
						}
					} else if strings.Contains(line, "Chain OUTPUT") {
						if strings.Contains(line, "policy ACCEPT") {
							status.DefaultOut = "ACCEPT"
						} else if strings.Contains(line, "policy DROP") {
							status.DefaultOut = "DROP"
						}
					}
				}

				if strings.Contains(line, "ACCEPT") || strings.Contains(line, "DROP") || strings.Contains(line, "REJECT") {
					if !strings.HasPrefix(line, "Chain") && !strings.HasPrefix(line, "num") && !strings.HasPrefix(line, "target") {
						status.RuleCount++
					}
				}
			}

			// Set defaults if not detected
			if status.DefaultIn == "" {
				status.DefaultIn = "ACCEPT"
			}
			if status.DefaultOut == "" {
				status.DefaultOut = "ACCEPT"
			}
		}
		// Also count ip6tables rules
		ip6Output, _, ip6Err := s.executor.RunCombined(ctx, "ip6tables", "-L", "-n", "--line-numbers")
		if ip6Err == nil {
			lines := strings.Split(ip6Output, "\n")
			for _, line := range lines {
				if strings.Contains(line, "ACCEPT") || strings.Contains(line, "DROP") || strings.Contains(line, "REJECT") {
					if !strings.HasPrefix(line, "Chain") && !strings.HasPrefix(line, "num") && !strings.HasPrefix(line, "target") {
						status.RuleCount++
					}
				}
			}
		}
	}

	// Count enabled rules from database (custom rules)
	var dbCount int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM firewall_rules WHERE enabled = 1").Scan(&dbCount)
	status.CustomRuleCount = dbCount
	status.RuleCount += dbCount

	return status, nil
}

// ListRules returns all firewall rules from the database
func (s *FirewallService) ListRules(ctx context.Context) ([]model.FirewallRule, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, chain, protocol, port, action, source, target, enabled, priority, ip_version, remark, created_at FROM firewall_rules ORDER BY priority ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.FirewallRule
	for rows.Next() {
		var r model.FirewallRule
		var enabled int
		var port, source, target, ipVersion, remark sql.NullString
		if err := rows.Scan(&r.ID, &r.Chain, &r.Protocol, &port, &r.Action, &source, &target, &enabled, &r.Priority, &ipVersion, &remark, &r.CreatedAt); err != nil {
			log.Printf("firewall: scan rule row error: %v", err)
			continue
		}
		r.Enabled = enabled != 0
		if port.Valid {
			r.Port = port.String
		}
		if source.Valid {
			r.Source = source.String
		}
		if target.Valid {
			r.Target = target.String
		}
		if ipVersion.Valid && ipVersion.String != "" {
			r.IPVersion = ipVersion.String
		} else {
			r.IPVersion = "ipv4"
		}
		if remark.Valid {
			r.Remark = remark.String
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating firewall rules: %w", err)
	}
	return rules, nil
}

// GetRule returns a firewall rule by ID
func (s *FirewallService) GetRule(ctx context.Context, id int64) (*model.FirewallRule, error) {
	var r model.FirewallRule
	var enabled int
	var port, source, target, ipVersion, remark sql.NullString

	err := s.db.QueryRowContext(ctx,
		"SELECT id, chain, protocol, port, action, source, target, enabled, priority, ip_version, remark, created_at FROM firewall_rules WHERE id = ?", id,
	).Scan(&r.ID, &r.Chain, &r.Protocol, &port, &r.Action, &source, &target, &enabled, &r.Priority, &ipVersion, &remark, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	if port.Valid {
		r.Port = port.String
	}
	if source.Valid {
		r.Source = source.String
	}
	if target.Valid {
		r.Target = target.String
	}
	if ipVersion.Valid && ipVersion.String != "" {
		r.IPVersion = ipVersion.String
	} else {
		r.IPVersion = "ipv4"
	}
	if remark.Valid {
		r.Remark = remark.String
	}
	return &r, nil
}

// CreateRule creates a new firewall rule and applies it
func (s *FirewallService) CreateRule(ctx context.Context, rule *model.FirewallRule) error {
	// Apply the rule to the system
	if err := s.applyRule(ctx, rule); err != nil {
		return fmt.Errorf("failed to apply rule: %w", err)
	}

	// Save to database
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO firewall_rules (chain, protocol, port, action, source, target, enabled, ip_version, remark)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Chain, rule.Protocol, rule.Port, rule.Action, rule.Source, rule.Target, boolToInt(rule.Enabled), rule.IPVersion, rule.Remark)
	if err != nil {
		return err
	}
	rule.ID, _ = result.LastInsertId()
	return nil
}

// UpdateRule updates an existing firewall rule
func (s *FirewallService) UpdateRule(ctx context.Context, rule *model.FirewallRule) error {
	// Get the old rule to remove it from the system if it's enabled
	oldRule, err := s.GetRule(ctx, rule.ID)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	// If the old rule was enabled, remove it from the system first
	if oldRule.Enabled {
		if err := s.removeRule(ctx, oldRule); err != nil {
			log.Printf("firewall: failed to remove old rule from system during update: %v", err)
		}
	}

	// Update database
	if _, err := s.db.ExecContext(ctx,
		`UPDATE firewall_rules SET chain=?, protocol=?, port=?, action=?, source=?, target=?, ip_version=?, remark=? WHERE id=?`,
		rule.Chain, rule.Protocol, rule.Port, rule.Action, rule.Source, rule.Target, rule.IPVersion, rule.Remark, rule.ID); err != nil {
		return fmt.Errorf("failed to update rule in database: %w", err)
	}

	// If the rule was enabled, apply the updated rule to the system
	if oldRule.Enabled {
		if err := s.applyRule(ctx, rule); err != nil {
			log.Printf("firewall: failed to apply updated rule to system: %v", err)
			// Don't return error here - DB is updated, system will be out of sync
			// but user can re-enable the rule to fix it
		}
	}

	return nil
}

// DeleteRule deletes a firewall rule and removes it from the system
func (s *FirewallService) DeleteRule(ctx context.Context, id int64) error {
	rule, err := s.GetRule(ctx, id)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	// Remove from system (collect error but continue with DB cleanup)
	var sysErr error
	if err := s.removeRule(ctx, rule); err != nil {
		log.Printf("firewall: failed to remove rule %d from system: %v", id, err)
		sysErr = fmt.Errorf("failed to remove rule from system: %w", err)
	}

	// Remove from database
	if _, err := s.db.ExecContext(ctx, "DELETE FROM firewall_rules WHERE id = ?", id); err != nil {
		return fmt.Errorf("failed to delete rule from database: %w", err)
	}

	// Return system error if DB deletion succeeded but system removal failed
	return sysErr
}

// EnableRule enables a firewall rule
func (s *FirewallService) EnableRule(ctx context.Context, id int64) error {
	rule, err := s.GetRule(ctx, id)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	if err := s.applyRule(ctx, rule); err != nil {
		return fmt.Errorf("failed to apply rule: %w", err)
	}

	_, err = s.db.ExecContext(ctx, "UPDATE firewall_rules SET enabled=1 WHERE id=?", id)
	return err
}

// DisableRule disables a firewall rule
func (s *FirewallService) DisableRule(ctx context.Context, id int64) error {
	rule, err := s.GetRule(ctx, id)
	if err != nil {
		return fmt.Errorf("rule not found: %w", err)
	}

	if err := s.removeRule(ctx, rule); err != nil {
		return fmt.Errorf("failed to remove rule: %w", err)
	}

	_, err = s.db.ExecContext(ctx, "UPDATE firewall_rules SET enabled=0 WHERE id=?", id)
	return err
}

// MoveRuleUp moves a rule up in priority (lower priority number)
func (s *FirewallService) MoveRuleUp(ctx context.Context, id int64) error {
	// Get all rules ordered by priority
	rules, err := s.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	// Find the rule and its index
	idx := -1
	for i, r := range rules {
		if r.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("rule not found")
	}
	if idx == 0 {
		return nil // Already at top
	}

	// Swap priority with the rule above
	ruleAbove := rules[idx-1]
	currentRule := rules[idx]

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", ruleAbove.Priority, currentRule.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", currentRule.Priority, ruleAbove.ID); err != nil {
		return err
	}

	return tx.Commit()
}

// MoveRuleDown moves a rule down in priority (higher priority number)
func (s *FirewallService) MoveRuleDown(ctx context.Context, id int64) error {
	// Get all rules ordered by priority
	rules, err := s.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	// Find the rule and its index
	idx := -1
	for i, r := range rules {
		if r.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("rule not found")
	}
	if idx == len(rules)-1 {
		return nil // Already at bottom
	}

	// Swap priority with the rule below
	ruleBelow := rules[idx+1]
	currentRule := rules[idx]

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", ruleBelow.Priority, currentRule.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE firewall_rules SET priority=? WHERE id=?", currentRule.Priority, ruleBelow.ID); err != nil {
		return err
	}

	return tx.Commit()
}

// EnableFirewall enables the firewall
func (s *FirewallService) EnableFirewall(ctx context.Context) error {
	tool := s.DetectTool(ctx)
	switch tool {
	case "ufw":
		out, _, err := s.executor.RunCombined(ctx, "ufw", "--force", "enable")
		if err != nil {
			return fmt.Errorf("failed to enable firewall: %s", out)
		}
	case "nft":
		// For nftables, "enable" means ensuring the table exists and applying rules
		if err := s.ensureNftTable(ctx); err != nil {
			return fmt.Errorf("failed to setup nft table: %w", err)
		}
		// Re-apply all enabled rules from database
		rules, err := s.ListRules(ctx)
		if err != nil {
			return fmt.Errorf("failed to list rules: %w", err)
		}
		for _, rule := range rules {
			if rule.Enabled {
				if err := s.applyRule(ctx, &rule); err != nil {
					log.Printf("firewall: failed to apply nft rule %d: %v", rule.ID, err)
				}
			}
		}
	case "iptables":
		// For iptables, ensure basic chains exist and re-apply rules for both iptables and ip6tables
		for _, chain := range []string{"INPUT", "FORWARD", "OUTPUT"} {
			s.executor.RunCombined(ctx, "iptables", "-N", chain)
			s.executor.RunCombined(ctx, "ip6tables", "-N", chain)
		}
		// Re-apply all enabled rules from database
		rules, err := s.ListRules(ctx)
		if err != nil {
			return fmt.Errorf("failed to list rules: %w", err)
		}
		for _, rule := range rules {
			if rule.Enabled {
				if err := s.applyRule(ctx, &rule); err != nil {
					log.Printf("firewall: failed to apply iptables rule %d: %v", rule.ID, err)
				}
			}
		}
	default:
		return fmt.Errorf("unsupported firewall tool: %s", tool)
	}
	return nil
}

// DisableFirewall disables the firewall
func (s *FirewallService) DisableFirewall(ctx context.Context) error {
	tool := s.DetectTool(ctx)
	switch tool {
	case "ufw":
		out, _, err := s.executor.RunCombined(ctx, "ufw", "--force", "disable")
		if err != nil {
			return fmt.Errorf("failed to disable firewall: %s", out)
		}
	case "nft":
		// For nftables, "disable" means flushing all rules
		if _, _, err := s.executor.RunCombined(ctx, "nft", "flush", "ruleset"); err != nil {
			return fmt.Errorf("failed to flush nft ruleset: %v", err)
		}
	case "iptables":
		// SAFETY: Before flushing, ensure default policy is ACCEPT to prevent lockout.
		// If default policy is DROP and we flush all rules, SSH/panel access is lost.
		for _, chain := range []string{"INPUT", "OUTPUT"} {
			if _, _, err := s.executor.RunCombined(ctx, "iptables", "-P", chain, "ACCEPT"); err != nil {
				log.Printf("firewall: failed to set iptables %s policy to ACCEPT: %v", chain, err)
			}
			if _, _, err := s.executor.RunCombined(ctx, "ip6tables", "-P", chain, "ACCEPT"); err != nil {
				log.Printf("firewall: failed to set ip6tables %s policy to ACCEPT: %v", chain, err)
			}
		}
		// Flush all rules for both IPv4 and IPv6
		if _, _, err := s.executor.RunCombined(ctx, "iptables", "-F"); err != nil {
			return fmt.Errorf("failed to flush iptables: %v", err)
		}
		if _, _, err := s.executor.RunCombined(ctx, "ip6tables", "-F"); err != nil {
			log.Printf("firewall: failed to flush ip6tables: %v", err)
		}
	default:
		return fmt.Errorf("unsupported firewall tool: %s", tool)
	}
	return nil
}

// applyRule applies a firewall rule to the system
func (s *FirewallService) applyRule(ctx context.Context, rule *model.FirewallRule) error {
	tool := s.DetectTool(ctx)
	ipVersion := rule.IPVersion
	if ipVersion == "" {
		ipVersion = "ipv4"
	}

	// For "both", apply for ipv4 and ipv6 separately
	if ipVersion == "both" {
		ruleCopy := *rule
		ruleCopy.IPVersion = "ipv4"
		if err := s.applyRuleForVersion(ctx, &ruleCopy, tool); err != nil {
			return err
		}
		ruleCopy.IPVersion = "ipv6"
		return s.applyRuleForVersion(ctx, &ruleCopy, tool)
	}

	return s.applyRuleForVersion(ctx, rule, tool)
}

// applyRuleForVersion applies a rule for a specific IP version
func (s *FirewallService) applyRuleForVersion(ctx context.Context, rule *model.FirewallRule, tool string) error {
	switch tool {
	case "ufw":
		return s.applyUfwRule(ctx, rule)
	case "nft":
		return s.applyNftRule(ctx, rule)
	case "iptables":
		return s.applyIptablesRule(ctx, rule)
	default:
		return fmt.Errorf("no firewall tool available")
	}
}

// removeRule removes a firewall rule from the system
func (s *FirewallService) removeRule(ctx context.Context, rule *model.FirewallRule) error {
	tool := s.DetectTool(ctx)
	ipVersion := rule.IPVersion
	if ipVersion == "" {
		ipVersion = "ipv4"
	}

	// For "both", remove for ipv4 and ipv6 separately
	if ipVersion == "both" {
		ruleCopy := *rule
		ruleCopy.IPVersion = "ipv4"
		if err := s.removeRuleForVersion(ctx, &ruleCopy, tool); err != nil {
			log.Printf("firewall: failed to remove ipv4 rule: %v", err)
		}
		ruleCopy.IPVersion = "ipv6"
		return s.removeRuleForVersion(ctx, &ruleCopy, tool)
	}

	return s.removeRuleForVersion(ctx, rule, tool)
}

// removeRuleForVersion removes a rule for a specific IP version
func (s *FirewallService) removeRuleForVersion(ctx context.Context, rule *model.FirewallRule, tool string) error {
	switch tool {
	case "ufw":
		return s.removeUfwRule(ctx, rule)
	case "nft":
		return s.removeNftRule(ctx, rule)
	case "iptables":
		return s.removeIptablesRule(ctx, rule)
	default:
		return nil
	}
}

// applyUfwRule applies a rule using ufw
func (s *FirewallService) applyUfwRule(ctx context.Context, rule *model.FirewallRule) error {
	args := []string{}

	if rule.Source != "" {
		args = append(args, "from", rule.Source)
	}

	if rule.Port != "" {
		args = append(args, "to", "any", "port", rule.Port)
	}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "proto", rule.Protocol)
	}

	args = append(args, strings.ToLower(rule.Action))

	out, _, err := s.executor.RunCombined(ctx, "ufw", args...)
	if err != nil {
		return fmt.Errorf("ufw error: %s", out)
	}
	return nil
}

// removeUfwRule removes a rule using ufw
func (s *FirewallService) removeUfwRule(ctx context.Context, rule *model.FirewallRule) error {
	args := []string{"delete"}

	if rule.Source != "" {
		args = append(args, "from", rule.Source)
	}

	if rule.Port != "" {
		args = append(args, "to", "any", "port", rule.Port)
	}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "proto", rule.Protocol)
	}

	args = append(args, strings.ToLower(rule.Action))

	out, _, err := s.executor.RunCombined(ctx, "ufw", args...)
	if err != nil {
		return fmt.Errorf("ufw error: %s", out)
	}
	return nil
}

// iptablesTool returns the iptables binary name for the given IP version
func iptablesTool(ipVersion string) string {
	if ipVersion == "ipv6" {
		return "ip6tables"
	}
	return "iptables"
}

// applyIptablesRule applies a rule using iptables
func (s *FirewallService) applyIptablesRule(ctx context.Context, rule *model.FirewallRule) error {
	tool := iptablesTool(rule.IPVersion)
	args := []string{"-A", rule.Chain}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "-p", rule.Protocol)
	}

	if rule.Source != "" {
		args = append(args, "-s", rule.Source)
	}

	if rule.Port != "" {
		args = append(args, "--dport", rule.Port)
	}

	args = append(args, "-j", rule.Action)

	out, _, err := s.executor.RunCombined(ctx, tool, args...)
	if err != nil {
		return fmt.Errorf("%s error: %s", tool, out)
	}
	return nil
}

// removeIptablesRule removes a rule using iptables
func (s *FirewallService) removeIptablesRule(ctx context.Context, rule *model.FirewallRule) error {
	tool := iptablesTool(rule.IPVersion)
	args := []string{"-D", rule.Chain}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, "-p", rule.Protocol)
	}

	if rule.Source != "" {
		args = append(args, "-s", rule.Source)
	}

	if rule.Port != "" {
		args = append(args, "--dport", rule.Port)
	}

	args = append(args, "-j", rule.Action)

	out, _, err := s.executor.RunCombined(ctx, tool, args...)
	if err != nil {
		return fmt.Errorf("%s error: %s", tool, out)
	}
	return nil
}

// applyNftRule applies a rule using nftables
func (s *FirewallService) applyNftRule(ctx context.Context, rule *model.FirewallRule) error {
	// First, ensure the table and chain exist
	if err := s.ensureNftTable(ctx); err != nil {
		return err
	}

	// Build nft command
	// nft add rule inet filter INPUT tcp dport 80 drop
	chain := strings.ToUpper(rule.Chain)

	args := []string{"add", "rule", "inet", "filter", chain}

	if rule.Protocol != "" && rule.Protocol != "all" {
		args = append(args, rule.Protocol)
	}

	if rule.Source != "" {
		// Use "ip6 saddr" for IPv6, "ip saddr" for IPv4
		if rule.IPVersion == "ipv6" {
			args = append(args, "ip6", "saddr", rule.Source)
		} else {
			args = append(args, "ip", "saddr", rule.Source)
		}
	}

	if rule.Port != "" {
		if strings.Contains(rule.Port, "-") {
			// Port range: 8000-9000
			args = append(args, "dport", fmt.Sprintf("{ %s }", rule.Port))
		} else {
			args = append(args, "dport", rule.Port)
		}
	}

	args = append(args, strings.ToLower(rule.Action))

	out, _, err := s.executor.RunCombined(ctx, "nft", args...)
	if err != nil {
		return fmt.Errorf("nft error: %s", out)
	}
	return nil
}

// ensureNftTable ensures the inet filter table and chains exist
func (s *FirewallService) ensureNftTable(ctx context.Context) error {
	// Check if table exists
	_, exitCode, _ := s.executor.RunCombined(ctx, "nft", "list", "table", "inet", "filter")
	if exitCode != 0 {
		// Table doesn't exist, create it
		cmds := [][]string{
			{"add", "table", "inet", "filter"},
			{"add", "chain", "inet", "filter", "INPUT", "{", "type", "filter", "hook", "input", "priority", "0", ";", "}"},
			{"add", "chain", "inet", "filter", "OUTPUT", "{", "type", "filter", "hook", "output", "priority", "0", ";", "}"},
			{"add", "chain", "inet", "filter", "FORWARD", "{", "type", "filter", "hook", "forward", "priority", "0", ";", "}"},
		}
		for _, args := range cmds {
			out, _, err := s.executor.RunCombined(ctx, "nft", args...)
			if err != nil {
				return fmt.Errorf("nft setup error: %s", out)
			}
		}
	}
	return nil
}

// removeNftRule removes a rule using nftables
func (s *FirewallService) removeNftRule(ctx context.Context, rule *model.FirewallRule) error {
	// Get the chain
	chain := strings.ToUpper(rule.Chain)

	// List rules to find the handle (nft -a list chain ...)
	output, _, err := s.executor.RunCombined(ctx, "nft", "-a", "list", "chain", "inet", "filter", chain)
	if err != nil {
		return fmt.Errorf("nft list error: %s", output)
	}

	// Normalize action for comparison (nft uses lowercase)
	actionLower := strings.ToLower(rule.Action)

	// Determine source prefix for matching
	srcPrefix := "ip saddr"
	if rule.IPVersion == "ipv6" {
		srcPrefix = "ip6 saddr"
	}

	// Parse output to find matching rule handle
	lines := strings.Split(output, "\n")
	var handlesToDelete []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if this line contains the action
		if !strings.Contains(line, actionLower) {
			continue
		}

		// Match by port if specified
		if rule.Port != "" {
			portMatch := false
			// Check for exact port match or port in set notation
			if strings.Contains(line, fmt.Sprintf("dport %s", rule.Port)) ||
				strings.Contains(line, fmt.Sprintf("dport { %s }", rule.Port)) {
				portMatch = true
			}
			if !portMatch {
				continue
			}
		}

		// Match by protocol if specified
		if rule.Protocol != "" && rule.Protocol != "all" {
			if !strings.Contains(line, rule.Protocol) {
				continue
			}
		}

		// Match by source if specified
		if rule.Source != "" {
			if !strings.Contains(line, fmt.Sprintf("%s %s", srcPrefix, rule.Source)) {
				continue
			}
		}

		// Extract handle (last field after "# handle")
		parts := strings.Fields(line)
		for i, part := range parts {
			if part == "handle" && i+1 < len(parts) {
				handlesToDelete = append(handlesToDelete, parts[i+1])
				break
			}
		}
	}

	if len(handlesToDelete) == 0 {
		// Rule not found in nft, but we should still update the database
		// This can happen if the rule was already removed manually
		log.Printf("firewall: rule not found in nft, updating database only")
		return nil
	}

	// Delete all matching rules
	for _, handle := range handlesToDelete {
		delOut, _, delErr := s.executor.RunCombined(ctx, "nft", "delete", "rule", "inet", "filter", chain, "handle", handle)
		if delErr != nil {
			log.Printf("firewall: failed to delete nft rule with handle %s: %s", handle, delOut)
		}
	}

	return nil
}

// GetSystemRules returns rules from the system (not from database)
func (s *FirewallService) GetSystemRules(ctx context.Context) ([]model.FirewallRule, error) {
	tool := s.DetectTool(ctx)

	switch tool {
	case "ufw":
		return s.getUfwRules(ctx)
	case "nft":
		return s.getNftRules(ctx)
	case "iptables":
		return s.getIptablesRules(ctx)
	default:
		return nil, fmt.Errorf("no firewall tool available")
	}
}

// RemoveSystemRule removes a rule directly from the system
func (s *FirewallService) RemoveSystemRule(ctx context.Context, rule *model.FirewallRule) error {
	return s.removeRule(ctx, rule)
}

// getUfwRules returns rules from ufw
func (s *FirewallService) getUfwRules(ctx context.Context) ([]model.FirewallRule, error) {
	out, _, err := s.executor.RunCombined(ctx, "ufw", "status", "numbered")
	if err != nil {
		return nil, err
	}

	var rules []model.FirewallRule
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Status:") || strings.HasPrefix(line, "---") {
			continue
		}

		// Parse ufw rule line: [ 1] 22/tcp ALLOW IN Anywhere
		if strings.Contains(line, "ALLOW") || strings.Contains(line, "DENY") || strings.Contains(line, "REJECT") {
			rule := model.FirewallRule{
				Enabled: true,
			}

			if strings.Contains(line, "ALLOW") {
				rule.Action = "ACCEPT"
			} else if strings.Contains(line, "DENY") {
				rule.Action = "DROP"
			} else if strings.Contains(line, "REJECT") {
				rule.Action = "REJECT"
			}

			// Extract port
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.Contains(part, "/") {
					rule.Port = strings.Split(part, "/")[0]
					rule.Protocol = strings.Split(part, "/")[1]
					break
				}
			}

			rule.Chain = "INPUT"
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// getNftRules returns rules from nftables (only from inet filter table)
func (s *FirewallService) getNftRules(ctx context.Context) ([]model.FirewallRule, error) {
	out, _, err := s.executor.RunCombined(ctx, "nft", "list", "ruleset")
	if err != nil {
		return nil, err
	}

	var rules []model.FirewallRule
	lines := strings.Split(out, "\n")
	currentChain := ""
	inInetFilter := false // Only parse rules from "inet filter" table

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Track which table we're in
		if strings.HasPrefix(line, "table ") {
			// "table inet filter {" means we're entering our managed table
			inInetFilter = strings.Contains(line, "table inet filter")
			continue
		}

		// Skip rules from other tables (like "table ip filter")
		if !inInetFilter {
			// Skip until we find our table or end of file
			continue
		}

		// Detect chain name
		if strings.HasPrefix(line, "chain ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Skip chain metadata (type, policy, etc.)
		if strings.Contains(line, "type filter hook") || strings.Contains(line, "policy ") {
			continue
		}

		// Skip closing braces
		if line == "}" {
			continue
		}

		// Parse nft rule line: tcp dport 80 accept
		if strings.Contains(line, "accept") || strings.Contains(line, "drop") || strings.Contains(line, "reject") {
			rule := model.FirewallRule{
				Chain:   currentChain,
				Enabled: true,
			}

			if strings.Contains(line, "accept") {
				rule.Action = "ACCEPT"
			} else if strings.Contains(line, "drop") {
				rule.Action = "DROP"
			} else if strings.Contains(line, "reject") {
				rule.Action = "REJECT"
			}

			// Extract port
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "dport" && i+1 < len(parts) {
					rule.Port = parts[i+1]
					break
				}
			}

			// Extract protocol
			for _, part := range parts {
				if part == "tcp" || part == "udp" {
					rule.Protocol = part
					break
				}
			}

			// Extract source (handle both "ip saddr" and "ip6 saddr")
			for i, part := range parts {
				if part == "saddr" && i+1 < len(parts) {
					rule.Source = parts[i+1]
					// Detect if this is an IPv6 source
					if i > 0 && parts[i-1] == "ip6" {
						rule.IPVersion = "ipv6"
					} else {
						rule.IPVersion = "ipv4"
					}
					break
				}
			}

			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// SetDefaultPolicy sets the default policy for a chain (INPUT or OUTPUT)
func (s *FirewallService) SetDefaultPolicy(ctx context.Context, chain, policy string) error {
	chain = strings.ToUpper(chain)
	policy = strings.ToUpper(policy)

	tool := s.DetectTool(ctx)
	if tool == "" {
		return fmt.Errorf("no firewall tool available")
	}

	switch tool {
	case "ufw":
		return s.setUfwDefaultPolicy(ctx, chain, policy)
	case "nft":
		return s.setNftDefaultPolicy(ctx, chain, policy)
	case "iptables":
		return s.setIptablesDefaultPolicy(ctx, chain, policy)
	default:
		return fmt.Errorf("unsupported firewall tool: %s", tool)
	}
}

// setUfwDefaultPolicy sets default policy via ufw
func (s *FirewallService) setUfwDefaultPolicy(ctx context.Context, chain, policy string) error {
	// ufw uses "incoming" for INPUT and "outgoing" for OUTPUT
	var direction string
	switch chain {
	case "INPUT":
		direction = "incoming"
	case "OUTPUT":
		direction = "outgoing"
	default:
		return fmt.Errorf("ufw only supports INPUT and OUTPUT chains for default policy")
	}

	var ufwPolicy string
	switch policy {
	case "ACCEPT":
		ufwPolicy = "allow"
	case "DROP":
		ufwPolicy = "deny"
	default:
		return fmt.Errorf("invalid policy %q, must be ACCEPT or DROP", policy)
	}

	out, _, err := s.executor.RunCombined(ctx, "ufw", "default", ufwPolicy, direction)
	if err != nil {
		return fmt.Errorf("ufw default policy error: %s", out)
	}
	return nil
}

// setNftDefaultPolicy sets default policy via nftables
func (s *FirewallService) setNftDefaultPolicy(ctx context.Context, chain, policy string) error {
	if err := s.ensureNftTable(ctx); err != nil {
		return fmt.Errorf("failed to setup nft table: %w", err)
	}

	nftPolicy := strings.ToLower(policy)
	// nft chain inet filter INPUT { policy drop; }
	out, _, err := s.executor.RunCombined(ctx, "nft", "chain", "inet", "filter", chain, "{", "policy", nftPolicy, ";", "}")
	if err != nil {
		return fmt.Errorf("nft default policy error: %s", out)
	}
	return nil
}

// setIptablesDefaultPolicy sets default policy via iptables
func (s *FirewallService) setIptablesDefaultPolicy(ctx context.Context, chain, policy string) error {
	out, _, err := s.executor.RunCombined(ctx, "iptables", "-P", chain, policy)
	if err != nil {
		return fmt.Errorf("iptables default policy error: %s", out)
	}
	// Also set ip6tables default policy to keep IPv4/IPv6 in sync
	s.executor.RunCombined(ctx, "ip6tables", "-P", chain, policy)
	return nil
}

// getIptablesRules returns rules from iptables (both IPv4 and IPv6)
func (s *FirewallService) getIptablesRules(ctx context.Context) ([]model.FirewallRule, error) {
	var rules []model.FirewallRule

	// Parse IPv4 iptables rules
	rules4, err := s.parseIptablesOutput(ctx, "iptables", "ipv4")
	if err != nil {
		log.Printf("firewall: failed to get iptables rules: %v", err)
	} else {
		rules = append(rules, rules4...)
	}

	// Parse IPv6 ip6tables rules
	rules6, err := s.parseIptablesOutput(ctx, "ip6tables", "ipv6")
	if err != nil {
		log.Printf("firewall: failed to get ip6tables rules: %v", err)
	} else {
		rules = append(rules, rules6...)
	}

	return rules, nil
}

// parseIptablesOutput parses iptables -L output into rules
func (s *FirewallService) parseIptablesOutput(ctx context.Context, tool, ipVersion string) ([]model.FirewallRule, error) {
	out, _, err := s.executor.RunCombined(ctx, tool, "-L", "-n", "--line-numbers")
	if err != nil {
		return nil, err
	}

	var rules []model.FirewallRule
	currentChain := ""
	lines := strings.Split(out, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse chain header: Chain INPUT (policy ACCEPT)
		if strings.HasPrefix(line, "Chain") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Skip header line
		if strings.HasPrefix(line, "num") || strings.HasPrefix(line, "target") {
			continue
		}

		// Parse rule: num target prot opt source destination
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			rule := model.FirewallRule{
				Chain:     currentChain,
				Action:    parts[1],
				Protocol:  parts[2],
				Source:    parts[4],
				IPVersion: ipVersion,
				Enabled:   true,
			}
			rules = append(rules, rule)
		}
	}
	return rules, nil
}
