package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"easyserver/internal/auth"
)

// LoginEvent is an alias for auth.LoginEvent.
type LoginEvent = auth.LoginEvent

// AlertEvent represents a triggered alert for notification.
type AlertEvent struct {
	RuleName  string  `json:"rule_name"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Duration  int     `json:"duration"`
	Timestamp string  `json:"timestamp"`
	Message   string  `json:"message"`
}

// Service handles sending notifications (webhook, etc.)
type Service struct {
	webhookURL string
	enabled    bool
	httpClient *http.Client
}

// NewService creates a new notify Service.
func NewService(webhookURL string, enabled bool) *Service {
	return &Service{
		webhookURL: webhookURL,
		enabled:    enabled,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// TestWebhook sends a test notification synchronously and returns any error.
func (s *Service) TestWebhook() error {
	event := LoginEvent{
		Username:  "test",
		IP:        "127.0.0.1",
		UserAgent: "EasyServer Test",
		Time:      time.Now().Format(time.RFC3339),
		Success:   true,
	}

	msg := s.formatMessage(event)
	payload := s.buildPayload(msg)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// NotifyLogin sends a login notification via webhook.
func (s *Service) NotifyLogin(event LoginEvent) {
	if !s.enabled || s.webhookURL == "" {
		return
	}

	go func() {
		if err := s.sendWebhook(event); err != nil {
			log.Printf("notify: failed to send login notification: %v", err)
		}
	}()
}

// NotifyAlert sends an alert notification via webhook.
func (s *Service) NotifyAlert(event AlertEvent) {
	if !s.enabled || s.webhookURL == "" {
		return
	}

	go func() {
		if err := s.sendAlertWebhook(event); err != nil {
			log.Printf("notify: failed to send alert notification: %v", err)
		}
	}()
}

func (s *Service) sendWebhook(event LoginEvent) error {
	msg := s.formatMessage(event)
	payload := s.buildPayload(msg)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *Service) formatMessage(event LoginEvent) string {
	status := "✅ 登录成功"
	if !event.Success {
		status = "❌ 登录失败"
	}

	return fmt.Sprintf("**EasyServer 登录通知**\n\n"+
		"- 状态: %s\n"+
		"- 用户: %s\n"+
		"- IP: %s\n"+
		"- 时间: %s\n"+
		"- User-Agent: %s",
		status,
		event.Username,
		event.IP,
		event.Time,
		event.UserAgent,
	)
}

func (s *Service) buildPayload(msg string) map[string]interface{} {
	switch {
	case strings.Contains(s.webhookURL, "dingtalk.com"):
		return map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": "EasyServer 登录通知",
				"text":  msg,
			},
		}
	case strings.Contains(s.webhookURL, "feishu.cn"):
		return map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": msg,
			},
		}
	case strings.Contains(s.webhookURL, "qyapi.weixin.qq.com"):
		return map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": msg,
			},
		}
	default:
		return map[string]interface{}{
			"text": msg,
		}
	}
}

func (s *Service) sendAlertWebhook(event AlertEvent) error {
	msg := fmt.Sprintf("**EasyServer 监控告警**\n\n"+
		"- 规则: %s\n"+
		"- 指标: %s\n"+
		"- 当前值: %.1f%%\n"+
		"- 阈值: %.1f%%\n"+
		"- 持续: %d 秒\n"+
		"- 时间: %s",
		event.RuleName,
		event.Metric,
		event.Value,
		event.Threshold,
		event.Duration,
		event.Timestamp,
	)

	payload := s.buildPayload(msg)
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := s.httpClient.Post(s.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
