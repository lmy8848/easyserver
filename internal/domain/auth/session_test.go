package auth

import (
	"sync"
	"testing"
	"time"

	"easyserver/internal/infra/config"
)

// 回归：会话列表应包含"有效但已静置超过 5 分钟"的设备。此前用 5 分钟活跃
// 窗口过滤，静置设备会从"登录会话"列表消失、也无法被踢出。
func TestGetActiveSessions_IncludesIdleButValid(t *testing.T) {
	ctx := t.Context()
	var wg sync.WaitGroup

	store := config.NewStore(&config.Config{
		Auth: config.AuthConfig{
			SessionTimeout:         config.Duration(24 * time.Hour),
			IdleTimeout:            config.Duration(30 * time.Minute),
			SessionCleanupInterval: config.Duration(time.Hour),
		},
	})
	svc := NewSessionService(ctx, &wg, store)

	if err := svc.CreateSession(ctx, "idle-token", 1, "192.0.2.1", "test-agent", "mobile", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// 模拟静置 10 分钟
	svc.mu.Lock()
	svc.sessions["idle-token"].LoginAt = time.Now().Add(-10 * time.Minute)
	svc.mu.Unlock()

	sessions, err := svc.GetActiveSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Token != "idle-token" {
		t.Fatalf("expected idle-but-valid session to be listed, got %d sessions", len(sessions))
	}

	// 过期会话不应出现
	if err := svc.CreateSession(ctx, "expired-token", 1, "192.0.2.2", "test-agent", "web", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	sessions, _ = svc.GetActiveSessions(ctx)
	if len(sessions) != 1 {
		t.Fatalf("expired session must be excluded, got %d sessions", len(sessions))
	}

	// 超过空闲超时（30m）的会话已不可用，不应出现在列表里
	if err := svc.CreateSession(ctx, "idle-dead-token", 1, "192.0.2.3", "test-agent", "web", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.sessions["idle-dead-token"].LoginAt = time.Now().Add(-40 * time.Minute)
	svc.mu.Unlock()
	sessions, _ = svc.GetActiveSessions(ctx)
	if len(sessions) != 1 {
		t.Fatalf("idle-expired session must be excluded, got %d sessions", len(sessions))
	}
}

// 回归：直连通道铸造的临时凭据会话（mobile-direct）不应出现在用户的
// 会话列表中——它们不是登录会话，此前与真实移动端登录同标 "mobile"，
// 导致会话管理里多出"移动端在线记录"。
func TestGetUserSessions_ExcludesMobileDirect(t *testing.T) {
	ctx := t.Context()
	var wg sync.WaitGroup

	store := config.NewStore(&config.Config{
		Auth: config.AuthConfig{
			SessionTimeout:         config.Duration(24 * time.Hour),
			IdleTimeout:            config.Duration(30 * time.Minute),
			SessionCleanupInterval: config.Duration(time.Hour),
		},
	})
	svc := NewSessionService(ctx, &wg, store)

	if err := svc.CreateSession(ctx, "login-token", 1, "192.0.2.1", "test-agent", "mobile", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateSession(ctx, "direct-token", 1, "mobile-direct", "device-signature", ClientTypeMobileDirect, time.Now().Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}

	sessions, err := svc.GetUserSessions(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Token != "login-token" {
		t.Fatalf("expected only the login session, got %d sessions", len(sessions))
	}
}
