package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/infra/config"
)

// 邮件渠道：465 端口走隐式 TLS，其它端口优先 STARTTLS；失败信息原样返回，
// 供设置页「测试发送」展示具体原因（认证失败/证书/端口不通）。

func (s *Service) emailConfig() config.NotifyConfig {
	return s.store.Get().Notify
}

func mailAddr(cfg config.NotifyConfig) string {
	port := cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	return net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(port))
}

func (s *Service) smtpAuth(cfg config.NotifyConfig) smtp.Auth {
	if cfg.SMTPUsername == "" {
		return nil
	}
	return smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
}

// SendEmail 发送纯文本邮件（主题 + 正文）。
func (s *Service) SendEmail(ctx context.Context, subject, body string) error {
	cfg := s.emailConfig()
	if !cfg.EmailEnabled {
		return errors.New("邮件通知未启用")
	}
	if cfg.SMTPHost == "" || len(cfg.SMTPTo) == 0 {
		return errors.New("SMTP 主机或收件人为空")
	}

	from := cfg.SMTPFrom
	if from == "" {
		from = cfg.SMTPUsername
	}
	if from == "" {
		return errors.New("发件人为空")
	}

	// 头部注入防护：地址/主题不允许换行，收件人必须是合法地址
	for _, v := range append([]string{from, subject}, cfg.SMTPTo...) {
		if strings.ContainsAny(v, "\r\n") {
			return errors.New("邮件头部包含非法换行符")
		}
	}
	if _, err := mail.ParseAddress(from); err != nil {
		return fmt.Errorf("发件人地址无效 %q: %w", from, err)
	}
	for _, rcpt := range cfg.SMTPTo {
		if _, err := mail.ParseAddress(rcpt); err != nil {
			return fmt.Errorf("收件人地址无效 %q: %w", rcpt, err)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(cfg.SMTPTo, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.BEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&b, "Date: %s\r\n\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString(body)

	addr := mailAddr(cfg)
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	// 465：隐式 TLS（SMTPS）
	if cfg.SMTPPort == 465 {
		td := &tls.Dialer{
			NetDialer: dialer,
			Config:    &tls.Config{ServerName: cfg.SMTPHost},
		}
		conn, err := td.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("连接 SMTP 失败: %w", err)
		}
		return s.sendWithConn(conn, cfg, from, b.String())
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	return s.sendWithConn(conn, cfg, from, b.String())
}

func (s *Service) sendWithConn(conn net.Conn, cfg config.NotifyConfig, from, msg string) error {
	defer conn.Close()
	c, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer c.Close()

	if cfg.SMTPPort != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: cfg.SMTPHost}); err != nil {
				return fmt.Errorf("STARTTLS 失败: %w", err)
			}
		}
	}
	if auth := s.smtpAuth(cfg); auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("SMTP 认证失败: %w", err)
			}
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM 失败: %w", err)
	}
	for _, rcpt := range cfg.SMTPTo {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s 失败: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA 失败: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("写入邮件失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("结束邮件失败: %w", err)
	}
	return c.Quit()
}

// TestEmail 发送一封测试邮件（设置页按钮用）。
func (s *Service) TestEmail() error {
	return s.SendEmail(context.Background(), "EasyServer 测试邮件",
		"这是一封来自 EasyServer 面板的测试邮件。\n\n收到说明 SMTP 配置正确。")
}
