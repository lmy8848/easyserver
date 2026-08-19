package security

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/domain/firewall"
	"easyserver/internal/infra/errx"
	"easyserver/internal/util"
)

const (
	// nginxBanFile is the shared deny-list included by every website server block.
	nginxBanFile = "/etc/nginx/conf.d/banned_ips.conf"
)

// Service orchestrates website security: rate-limit config + IP banning.
type SecurityService struct {
	repo     SecurityRepository
	firewall *firewall.Service
}

func NewSecurityService(repo SecurityRepository, fw *firewall.Service) *SecurityService {
	return &SecurityService{repo: repo, firewall: fw}
}

// --- Config ---

func (s *SecurityService) GetConfig(ctx context.Context, websiteID int64) (*SecurityConfig, error) {
	cfg, err := s.repo.GetConfig(ctx, websiteID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		// Auto-create default config on first access.
		cfg, err = s.repo.CreateConfig(ctx, websiteID)
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (s *SecurityService) UpdateConfig(ctx context.Context, websiteID int64, updater func(cfg *SecurityConfig)) (*SecurityConfig, error) {
	cfg, err := s.GetConfig(ctx, websiteID)
	if err != nil {
		return nil, err
	}
	updater(cfg)
	if err := s.repo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// --- IP Ban ---

// BanIP bans an IP for a website (or globally if websiteID is nil).
// It writes a deny directive to the nginx ban file and adds an iptables DROP rule.
func (s *SecurityService) BanIP(ctx context.Context, websiteID *int64, ip, reason, source string, durationSecs int) error {
	if ip == "" {
		return errx.BadRequest("IP 不能为空")
	}

	var expiresAt *time.Time
	if durationSecs > 0 {
		t := time.Now().Add(time.Duration(durationSecs) * time.Second)
		expiresAt = &t
	}

	// Record in DB.
	if _, err := s.repo.AddBannedIP(ctx, websiteID, ip, reason, source, expiresAt); err != nil {
		return err
	}

	// Network-layer ban via firewall (iptables DROP).
	// Remark carries the IP so UnbanIP can target the exact rule instead of
	// wiping every website-security rule.
	if s.firewall != nil {
		rule := &firewall.FirewallRule{
			Chain:    "INPUT",
			Protocol: "all",
			Action:   "DROP",
			Source:   ip,
			Enabled:  true,
			Remark:   fmt.Sprintf("网站安全封禁 %s: %s", ip, reason),
		}
		if err := s.firewall.CreateRule(ctx, rule); err != nil {
			return fmt.Errorf("防火墙规则创建失败: %w", err)
		}
	}

	// Application-layer ban via nginx deny file.
	return s.refreshNginxBanFile(ctx)
}

// UnbanIP removes a ban by ID (clears nginx deny + iptables rule).
func (s *SecurityService) UnbanIP(ctx context.Context, banID int64) error {
	// Look up the ban first so we know which IP's firewall rule to remove.
	ban, err := s.repo.GetBannedIP(ctx, banID)
	if err != nil {
		return err
	}
	if ban == nil {
		return errx.NotFound("封禁记录不存在")
	}
	ip := ban.IP

	if err := s.repo.RemoveBannedIP(ctx, banID); err != nil {
		return err
	}

	// Remove only the firewall rule(s) matching this IP + our remark prefix.
	if s.firewall != nil {
		rules, err := s.firewall.ListRules(ctx)
		if err == nil {
			needle := "网站安全封禁 " + ip
			for _, r := range rules {
				if r.Source == ip && strings.HasPrefix(r.Remark, needle) {
					_ = s.firewall.DeleteRule(ctx, r.ID)
				}
			}
		}
	}

	return s.refreshNginxBanFile(ctx)
}

// ListBannedIPs returns all active bans for a website (including global).
func (s *SecurityService) ListBannedIPs(ctx context.Context, websiteID int64) ([]BannedIP, error) {
	bans, err := s.repo.ListBannedIPs(ctx, websiteID)
	if err != nil {
		return nil, err
	}
	// Filter out expired.
	var active []BannedIP
	now := time.Now()
	for _, b := range bans {
		if b.ExpiresAt == nil || b.ExpiresAt.After(now) {
			active = append(active, b)
		}
	}
	return active, nil
}

// CleanupExpired removes expired bans from DB + nginx file + firewall.
func (s *SecurityService) CleanupExpired(ctx context.Context) error {
	n, err := s.repo.RemoveExpiredBannedIPs(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return s.refreshNginxBanFile(ctx)
	}
	return nil
}

// --- Nginx ban file ---

// refreshNginxBanFile rewrites the shared deny file from all active bans
// (global + per-website) and reloads nginx.
func (s *SecurityService) refreshNginxBanFile(ctx context.Context) error {
	bans, err := s.repo.ListAllBannedIPs(ctx)
	if err != nil {
		return err
	}

	// Deduplicate IPs (a global ban and a per-website ban may share an IP).
	seen := map[string]bool{}
	var lines []string
	lines = append(lines, "# EasyServer - auto-generated IP ban list. Do not edit manually.")
	lines = append(lines, "# Updated: "+time.Now().Format(util.TimeLayout))
	now := time.Now()
	for _, b := range bans {
		if b.ExpiresAt != nil && b.ExpiresAt.Before(now) {
			continue
		}
		if seen[b.IP] {
			continue
		}
		seen[b.IP] = true
		lines = append(lines, fmt.Sprintf("deny %s;", b.IP))
	}
	lines = append(lines, "")

	dir := filepath.Dir(nginxBanFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errx.Internal("创建 nginx 配置目录失败: %w", err)
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(nginxBanFile, []byte(content), 0644); err != nil {
		return errx.Internal("写入 nginx 封禁文件失败: %w", err)
	}

	// Reload nginx (graceful, no downtime).
	if _, err := exec.CommandContext(ctx, "nginx", "-t").CombinedOutput(); err != nil {
		return errx.Internal("nginx 配置测试失败，未重载")
	}
	if _, err := exec.CommandContext(ctx, "nginx", "-s", "reload").CombinedOutput(); err != nil {
		return errx.Internal("nginx 重载失败")
	}
	return nil
}

// NginxBanFilePath returns the path of the shared ban file (for reference).
func (s *SecurityService) NginxBanFilePath() string {
	return nginxBanFile
}
