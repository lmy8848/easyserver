package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"easyserver/internal/firewall"
	"easyserver/internal/infra/apperror"
)

// LoginEvent is one login-related activity record.
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

// GetLoginHistory returns recent login-related activities (LOGIN_* actions).
func (s *Service) GetLoginHistory(ctx context.Context, limit int) ([]LoginEvent, error) {
	if s.auth == nil {
		return nil, apperror.ErrInternal.WithMessage("auth service 不可用")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	acts, err := s.auth.GetAllActivities(ctx, limit)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	var events []LoginEvent
	for _, a := range acts {
		if !strings.HasPrefix(a.Action, "LOGIN") {
			continue
		}
		ev := LoginEvent{
			Time:      a.CreatedAt.Format("2006-01-02 15:04:05"),
			IP:        a.IP,
			Username:  a.Username,
			Action:    a.Action,
			UserAgent: a.UserAgent,
		}
		// Off-hours flag: 0:00-6:00.
		if a.CreatedAt.Hour() < 6 {
			ev.Anomaly = "off-hours"
		}
		events = append(events, ev)
	}
	return events, nil
}

// GetAnomalies detects brute-force attempts: IPs with >=threshold LOGIN_FAILED
// in the last window minutes.
func (s *Service) GetAnomalies(ctx context.Context, windowMinutes, threshold int) ([]Anomaly, error) {
	if s.auth == nil {
		return nil, apperror.ErrInternal.WithMessage("auth service 不可用")
	}
	if windowMinutes <= 0 {
		windowMinutes = 5
	}
	if threshold <= 0 {
		threshold = 10
	}
	acts, err := s.auth.GetAllActivities(ctx, 1000)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	cutoff := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	ipFails := map[string]*Anomaly{}
	for _, a := range acts {
		if a.Action != "LOGIN_FAILED" || a.IP == "" || a.CreatedAt.Before(cutoff) {
			continue
		}
		if ipFails[a.IP] == nil {
			ipFails[a.IP] = &Anomaly{IP: a.IP}
		}
		ipFails[a.IP].FailedCount++
		ipFails[a.IP].LastAttempt = a.CreatedAt.Format("2006-01-02 15:04:05")
	}
	var anomalies []Anomaly
	for _, an := range ipFails {
		if an.FailedCount >= threshold {
			an.Reason = fmt.Sprintf("%d 分钟内失败 %d 次", windowMinutes, an.FailedCount)
			anomalies = append(anomalies, *an)
		}
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
