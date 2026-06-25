package service

import (
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/model"
)

// AlertEvent represents a triggered alert for notification
type AlertEvent struct {
	RuleName  string  `json:"rule_name"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Duration  int     `json:"duration"`
	Timestamp string  `json:"timestamp"`
	Message   string  `json:"message"`
}

const alertCooldownDuration = 5 * time.Minute // 同一规则冷却时间

// AlertService evaluates alert rules against monitoring data
type AlertService struct {
	mu        sync.RWMutex
	rules     []model.AlertRule
	states    map[int64]*model.AlertState // ruleID -> state
	notify    *NotifyService
	notifSvc  *NotificationService
	cooldowns map[int64]time.Time // ruleID -> last notification time
}

// NewAlertService creates a new alert evaluation service
func NewAlertService(notify *NotifyService, notifSvc *NotificationService) *AlertService {
	return &AlertService{
		states:    make(map[int64]*model.AlertState),
		cooldowns: make(map[int64]time.Time),
		notify:    notify,
		notifSvc:  notifSvc,
	}
}

// SetRules updates the active alert rules
func (s *AlertService) SetRules(rules []model.AlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = rules
	// Clean up states for removed rules
	activeIDs := make(map[int64]bool)
	for _, r := range rules {
		activeIDs[r.ID] = true
	}
	for id := range s.states {
		if !activeIDs[id] {
			delete(s.states, id)
		}
	}
}

// Evaluate checks a monitor point against all active rules
func (s *AlertService) Evaluate(point *model.MonitorPoint) {
	s.mu.RLock()
	rules := make([]model.AlertRule, len(s.rules))
	copy(rules, s.rules)
	s.mu.RUnlock()

	now := time.Now()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		value := s.extractMetric(point, rule.Metric)
		if value < 0 {
			continue
		}

		above := value >= rule.Threshold

		s.mu.Lock()
		state, exists := s.states[rule.ID]
		if !exists {
			state = &model.AlertState{}
			s.states[rule.ID] = state
		}

		if above {
			if state.FirstAbove.IsZero() {
				state.FirstAbove = now
			}
			// Check if duration threshold is met
			if !state.Triggered && now.Sub(state.FirstAbove) >= time.Duration(rule.Duration)*time.Second {
				// Check cooldown
				if last, ok := s.cooldowns[rule.ID]; !ok || now.Sub(last) >= alertCooldownDuration {
					state.Triggered = true
					s.cooldowns[rule.ID] = now
					s.mu.Unlock()
					s.sendAlert(rule, value)
					continue
				}
			}
		} else {
			// Reset state when value drops below threshold
			state.FirstAbove = time.Time{}
			state.Triggered = false
		}
		s.mu.Unlock()
	}
}

// extractMetric gets the metric value from a monitor point
func (s *AlertService) extractMetric(p *model.MonitorPoint, metric string) float64 {
	switch metric {
	case "cpu_percent":
		return p.CPUPercent
	case "mem_percent":
		return p.MemPercent
	case "disk_percent":
		return p.DiskPercent
	case "load_1m":
		return p.CPULoad1m
	case "load_5m":
		return p.CPULoad5m
	case "load_15m":
		return p.CPULoad15m
	default:
		return -1
	}
}

// sendAlert sends a notification for a triggered alert
func (s *AlertService) sendAlert(rule model.AlertRule, value float64) {
	metricNames := map[string]string{
		"cpu_percent":  "CPU 使用率",
		"mem_percent":  "内存使用率",
		"disk_percent": "磁盘使用率",
		"load_1m":      "1 分钟负载",
		"load_5m":      "5 分钟负载",
		"load_15m":     "15 分钟负载",
	}

	metricName := metricNames[rule.Metric]
	if metricName == "" {
		metricName = rule.Metric
	}

	message := fmt.Sprintf("⚠️ 告警：%s %s 当前 %.1f%% 超过阈值 %.1f%%（持续 %d 秒）", rule.Name, metricName, value, rule.Threshold, rule.Duration)

	// Create notification in database
	if s.notifSvc != nil {
		s.notifSvc.CreateIfNotExists(model.CreateNotificationRequest{
			Type:    "alert",
			Title:   fmt.Sprintf("告警：%s", rule.Name),
			Message: message,
			Level:   "warning",
		})
	}

	// Send webhook notification
	if s.notify != nil {
		event := AlertEvent{
			RuleName:  rule.Name,
			Metric:    metricName,
			Value:     value,
			Threshold: rule.Threshold,
			Duration:  rule.Duration,
			Timestamp: time.Now().Format(time.RFC3339),
			Message:   message,
		}

		log.Printf("alert: triggered rule %q: %s = %.1f (threshold: %.1f)", rule.Name, rule.Metric, value, rule.Threshold)
		s.notify.NotifyAlert(event)
	}
}

// GetRules returns the current alert rules
func (s *AlertService) GetRules() []model.AlertRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := make([]model.AlertRule, len(s.rules))
	copy(rules, s.rules)
	return rules
}
