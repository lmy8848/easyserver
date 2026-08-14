package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"easyserver/internal/auth"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
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
	store      *config.Store
	httpClient *http.Client
}

// NewService creates a new notify Service. Webhook 配置（URL/开关）运行时
// 实时读 store：settings 修改后立即生效。
func NewService(store *config.Store) *Service {
	return &Service{
		store:      store,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// webhookConfig 返回当前生效的 webhook 配置（URL + 开关）。
func (s *Service) webhookConfig() (url string, enabled bool) {
	cfg := s.store.Get()
	return cfg.Notify.WebhookURL, cfg.Notify.Enabled
}

// TestWebhook sends a test notification synchronously and returns any error.
func (s *Service) TestWebhook() error {
	url, _ := s.webhookConfig()
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

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
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
	url, enabled := s.webhookConfig()
	if !enabled || url == "" {
		return
	}

	infra.Go(func() {
		if err := s.sendWebhook(event); err != nil {
			log.Printf("notify: failed to send login notification: %v", err)
		}
	})
}

// NotifyAlert sends an alert notification via webhook.
func (s *Service) NotifyAlert(event AlertEvent) {
	url, enabled := s.webhookConfig()
	if !enabled || url == "" {
		return
	}

	infra.Go(func() {
		if err := s.sendAlertWebhook(event); err != nil {
			log.Printf("notify: failed to send alert notification: %v", err)
		}
	})
}

func (s *Service) sendWebhook(event LoginEvent) error {
	url, _ := s.webhookConfig()
	msg := s.formatMessage(event)
	payload := s.buildPayload(msg)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
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

func (s *Service) buildPayload(msg string) map[string]any {
	url, _ := s.webhookConfig()
	switch {
	case strings.Contains(url, "dingtalk.com"):
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": "EasyServer 登录通知",
				"text":  msg,
			},
		}
	case strings.Contains(url, "feishu.cn"):
		return map[string]any{
			"msg_type": "text",
			"content": map[string]string{
				"text": msg,
			},
		}
	case strings.Contains(url, "qyapi.weixin.qq.com"):
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": msg,
			},
		}
	default:
		return map[string]any{
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

	url, _ := s.webhookConfig()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
