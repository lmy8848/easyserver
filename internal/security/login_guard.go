package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"easyserver/internal/firewall"
	"easyserver/internal/infra/apperror"
)

// LoginEvent is one login-related event from the audit log.
type LoginEvent struct {
	Time      string `json:"time"`
	IP        string `json:"ip"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	UserAgent string `json:"user_agent"`
	Anomaly   string `json:"anomaly,omitempty"` // brute-force / off-hours
}

// Anomaly is a detected login anomaly (e.g. brute-force from one IP).
type Anomaly struct {
	IP          string `json:"ip"`
	FailedCount int    `json:"failed_count"`
	LastAttempt string `json:"last_attempt"`
	Reason      string `json:"reason"`
}

// BannedIP is a login-anomaly ban rule in the firewall.
type BannedIP struct {
	ID        int64  `json:"id"`
	IP        string `json:"ip"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
}

const banRemarkPrefix = "登录异常封禁"

// GetLoginHistory returns recent login-related events from the audit log.
func (s *Service) GetLoginHistory(ctx context.Context, limit int) ([]LoginEvent, error) {
	if s.audit == nil {
		return nil, apperror.ErrInternal.WithMessage("audit service 不可用")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	records, err := s.audit.GetLoginHistory(ctx, limit)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	var events []LoginEvent
	for _, r := range records {
		ev := LoginEvent{
			Time:      r.CreatedAt.Format("2006-01-02 15:04:05"),
			IP:        r.IP,
			Username:  r.Username,
			Action:    r.Action,
			UserAgent: r.UserAgent,
		}
		// Off-hours flag: 0:00-6:00.
		if r.CreatedAt.Hour() < 6 {
			ev.Anomaly = "off-hours"
		}
		events = append(events, ev)
	}
	return events, nil
}

// GetAnomalies detects brute-force attempts: IPs with >=threshold LOGIN_FAILED
// in the last window minutes.
func (s *Service) GetAnomalies(ctx context.Context, windowMinutes, threshold int) ([]Anomaly, error) {
	if s.audit == nil {
		return nil, apperror.ErrInternal.WithMessage("audit service 不可用")
	}
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	if threshold <= 0 {
		threshold = 10
	}
	cutoff := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	failedByIP := s.audit.CountFailedLoginsByIP(ctx, cutoff, threshold)
	var anomalies []Anomaly
	for ip, count := range failedByIP {
		anomalies = append(anomalies, Anomaly{
			IP:          ip,
			FailedCount: count,
			Reason:      fmt.Sprintf("%d 分钟内失败 %d 次", windowMinutes, count),
		})
	}
	return anomalies, nil
}

// BanIP adds a firewall DROP rule for the IP.
func (s *Service) BanIP(ctx context.Context, ip, reason string) error {
	if s.firewall == nil {
		return apperror.ErrInternal.WithMessage("firewall service 不可用")
	}
	if ip == "" {
		return apperror.ErrBadRequest.WithMessage("IP 不能为空")
	}
	rule := &firewall.FirewallRule{
		Chain:    "INPUT",
		Protocol: "all",
		Action:   "DROP",
		Source:   ip,
		Enabled:  true,
		Remark:   banRemarkPrefix + ": " + reason,
	}
	if err := s.firewall.CreateRule(ctx, rule); err != nil {
		return apperror.WrapError(err)
	}
	return nil
}

// UnbanIP removes login-anomaly ban rules matching the IP.
func (s *Service) UnbanIP(ctx context.Context, ip string) error {
	if s.firewall == nil {
		return apperror.ErrInternal.WithMessage("firewall service 不可用")
	}
	rules, err := s.firewall.ListRules(ctx)
	if err != nil {
		return apperror.WrapError(err)
	}
	removed := 0
	for _, r := range rules {
		if r.Source == ip && strings.HasPrefix(r.Remark, banRemarkPrefix) {
			if err := s.firewall.DeleteRule(ctx, r.ID); err != nil {
				return apperror.WrapError(err)
			}
			removed++
		}
	}
	if removed == 0 {
		return apperror.ErrNotFound.WithMessage("未找到该 IP 的登录异常封禁规则")
	}
	return nil
}

// ListBannedIPs returns firewall rules created by login-anomaly bans.
func (s *Service) ListBannedIPs(ctx context.Context) ([]BannedIP, error) {
	if s.firewall == nil {
		return nil, apperror.ErrInternal.WithMessage("firewall service 不可用")
	}
	rules, err := s.firewall.ListRules(ctx)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	var banned []BannedIP
	for _, r := range rules {
		if strings.HasPrefix(r.Remark, banRemarkPrefix) {
			banned = append(banned, BannedIP{
				ID:        r.ID,
				IP:        r.Source,
				Remark:    r.Remark,
				CreatedAt: r.CreatedAt,
			})
		}
	}
	return banned, nil
}
