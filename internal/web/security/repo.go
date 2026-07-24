package security

import (
	"context"
	"time"
)

// SecurityConfig holds per-website rate-limit and auto-ban settings.
type SecurityConfig struct {
	ID                  int64     `json:"id"`
	WebsiteID           int64     `json:"website_id"`
	RateLimitEnabled    bool      `json:"rate_limit_enabled"`
	RateLimitRate       int       `json:"rate_limit_rate"`  // requests per second
	RateLimitBurst      int       `json:"rate_limit_burst"` // burst buffer
	LimitConn           int       `json:"limit_conn"`       // max concurrent connections per IP
	AutoBanEnabled      bool      `json:"auto_ban_enabled"`
	AutoBanThreshold    int       `json:"auto_ban_threshold"`     // max requests per minute per IP
	AutoBan404Threshold int       `json:"auto_ban_404_threshold"` // max 404s per minute per IP (scan detection)
	AutoBanDuration     int       `json:"auto_ban_duration"`      // ban duration in seconds (0 = permanent)
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// BannedIP records a banned IP (per-website or global).
type BannedIP struct {
	ID        int64      `json:"id"`
	WebsiteID *int64     `json:"website_id,omitempty"`
	IP        string     `json:"ip"`
	Reason    string     `json:"reason"`
	Source    string     `json:"source"` // "auto" or "manual"
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// SecurityRepository defines data access for website security.
type SecurityRepository interface {
	// Config
	GetConfig(ctx context.Context, websiteID int64) (*SecurityConfig, error)
	CreateConfig(ctx context.Context, websiteID int64) (*SecurityConfig, error)
	UpdateConfig(ctx context.Context, cfg *SecurityConfig) error

	// Banned IPs
	ListBannedIPs(ctx context.Context, websiteID int64) ([]BannedIP, error)
	ListGlobalBannedIPs(ctx context.Context) ([]BannedIP, error)
	ListAllBannedIPs(ctx context.Context) ([]BannedIP, error)
	GetBannedIP(ctx context.Context, id int64) (*BannedIP, error)
	AddBannedIP(ctx context.Context, websiteID *int64, ip, reason, source string, expiresAt *time.Time) (*BannedIP, error)
	RemoveBannedIP(ctx context.Context, id int64) error
	RemoveExpiredBannedIPs(ctx context.Context) (int, error)
	IsIPBanned(ctx context.Context, ip string, websiteID int64) (bool, error)
}
