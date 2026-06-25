package model

// FirewallRule represents a local firewall rule
type FirewallRule struct {
	ID        int64  `json:"id"`
	Chain     string `json:"chain"`      // INPUT, OUTPUT, FORWARD
	Protocol  string `json:"protocol"`   // tcp, udp, all
	Port      string `json:"port"`       // single port or range (e.g., "80", "8000-9000")
	Action    string `json:"action"`     // ACCEPT, DROP, REJECT
	Source    string `json:"source"`     // source IP/CIDR, empty = any
	Target    string `json:"target"`     // destination IP, empty = any
	Enabled   bool   `json:"enabled"`
	Priority  int64  `json:"priority"`   // lower = higher precedence
	IPVersion string `json:"ip_version"` // ipv4, ipv6, both
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
}

// FirewallStatus represents the firewall status
type FirewallStatus struct {
	Enabled        bool   `json:"enabled"`
	Tool           string `json:"tool"`           // iptables, nftables, ufw
	Version        string `json:"version"`
	RuleCount      int    `json:"rule_count"`      // total rules (system + custom)
	CustomRuleCount int   `json:"custom_rule_count"` // custom rules from database
	DefaultIn      string `json:"default_in"`      // ACCEPT, DROP, REJECT
	DefaultOut     string `json:"default_out"`     // ACCEPT, DROP, REJECT
}

// CreateFirewallRuleRequest is the request for creating a firewall rule
type CreateFirewallRuleRequest struct {
	Chain     string `json:"chain" binding:"required"`
	Protocol  string `json:"protocol"`
	Port      string `json:"port"`
	Action    string `json:"action" binding:"required"`
	Source    string `json:"source"`
	IPVersion string `json:"ip_version"`
	Remark    string `json:"remark"`
}

// SetDefaultPolicyRequest is the request for setting default chain policy
type SetDefaultPolicyRequest struct {
	Chain  string `json:"chain" binding:"required"`  // INPUT or OUTPUT
	Policy string `json:"policy" binding:"required"` // ACCEPT or DROP
}

// UpdateFirewallRuleRequest is the request for updating a firewall rule
type UpdateFirewallRuleRequest struct {
	Chain     *string `json:"chain"`
	Protocol  *string `json:"protocol"`
	Port      *string `json:"port"`
	Action    *string `json:"action"`
	Source    *string `json:"source"`
	IPVersion *string `json:"ip_version"`
	Remark    *string `json:"remark"`
}

// BulkIDsRequest is the request for bulk operations on firewall rules
type BulkIDsRequest struct {
	IDs []int64 `json:"ids" binding:"required,min=1"`
}

// FirewallLogEntry represents a parsed firewall log entry
type FirewallLogEntry struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`    // ACCEPT, DROP, REJECT, LOG
	Protocol  string `json:"protocol"`  // tcp, udp, icmp, etc.
	SrcIP     string `json:"src_ip"`
	DstIP     string `json:"dst_ip"`
	SrcPort   int    `json:"src_port"`
	DstPort   int    `json:"dst_port"`
	Interface string `json:"interface"` // IN, OUT interface
	Raw       string `json:"raw"`       // original log line
}
