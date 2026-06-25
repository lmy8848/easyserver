package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// NotifyService handles sending notifications (webhook, etc.)
type NotifyService struct {
	webhookURL string
	enabled    bool
	httpClient *http.Client
}

// NewNotifyService creates a new NotifyService
func NewNotifyService(webhookURL string, enabled bool) *NotifyService {
	return &NotifyService{
		webhookURL: webhookURL,
		enabled:    enabled,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// LoginEvent represents a login event for notification
type LoginEvent struct {
	Username  string `json:"username"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Time      string `json:"time"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason,omitempty"`
}

// TestWebhook sends a test notification synchronously and returns any error
func (s *NotifyService) TestWebhook() error {
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

// NotifyLogin sends a login notification via webhook
func (s *NotifyService) NotifyLogin(event LoginEvent) {
	if !s.enabled || s.webhookURL == "" {
		return
	}

	go func() {
		if err := s.sendWebhook(event); err != nil {
			log.Printf("notify: failed to send login notification: %v", err)
		}
	}()
}

// NotifyAlert sends an alert notification via webhook
func (s *NotifyService) NotifyAlert(event AlertEvent) {
	if !s.enabled || s.webhookURL == "" {
		return
	}

	go func() {
		if err := s.sendAlertWebhook(event); err != nil {
			log.Printf("notify: failed to send alert notification: %v", err)
		}
	}()
}

// sendWebhook sends the notification to the webhook URL
func (s *NotifyService) sendWebhook(event LoginEvent) error {
	// Format message for different webhook types
	msg := s.formatMessage(event)

	// Try to detect webhook type and format accordingly
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

// formatMessage formats the login event into a readable message
func (s *NotifyService) formatMessage(event LoginEvent) string {
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

// buildPayload builds the webhook payload (supports DingTalk, Feishu, WeCom)
func (s *NotifyService) buildPayload(msg string) map[string]interface{} {
	// Try to detect webhook type by URL
	switch {
	case strings.Contains(s.webhookURL, "dingtalk.com"):
		// DingTalk
		return map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": "EasyServer 登录通知",
				"text":  msg,
			},
		}
	case strings.Contains(s.webhookURL, "feishu.cn"):
		// Feishu
		return map[string]interface{}{
			"msg_type": "text",
			"content": map[string]string{
				"text": msg,
			},
		}
	case strings.Contains(s.webhookURL, "qyapi.weixin.qq.com"):
		// WeCom
		return map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": msg,
			},
		}
	default:
		// Generic webhook (Slack-compatible)
		return map[string]interface{}{
			"text": msg,
		}
	}
}

// sendAlertWebhook sends alert notification to webhook
func (s *NotifyService) sendAlertWebhook(event AlertEvent) error {
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
