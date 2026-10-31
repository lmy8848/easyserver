package auth

import (
	"sync"
	"time"
)

// VerifyAttemptLimiter 按 key（user_id）记录 2FA 验证（TOTP/备用码）失败次数，
// 连续失败 max 次后锁定 coolDown 时长。
//
// 背景与取舍：密码路径已有 DB 持久化的账户锁定；2FA 验证此前完全无尝试限制，
// 已知密码的攻击者可在线穷举 6 位 TOTP（限流仅按精确 IP，IPv6 轮换可绕）。
// 进程内实现即可达成目的——重启清零的窗口价值有限，持久化需要表迁移。
type VerifyAttemptLimiter struct {
	mu       sync.Mutex
	failed   map[int64]verifyAttempt
	max      int
	coolDown time.Duration
}

type verifyAttempt struct {
	count       int
	lockedUntil time.Time
}

func NewVerifyAttemptLimiter(max int, coolDown time.Duration) *VerifyAttemptLimiter {
	return &VerifyAttemptLimiter{
		failed:   make(map[int64]verifyAttempt),
		max:      max,
		coolDown: coolDown,
	}
}

// Allowed 返回该 key 当前是否允许尝试（未处于锁定期）。
func (l *VerifyAttemptLimiter) Allowed(key int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.failed[key]
	return !ok || !time.Now().Before(a.lockedUntil)
}

// RecordFailure 记录一次失败，连续失败达到阈值即进入锁定。
func (l *VerifyAttemptLimiter) RecordFailure(key int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.failed[key]
	if time.Now().Before(a.lockedUntil) {
		return // 锁定中不累计
	}
	a.count++
	if a.count >= l.max {
		a.lockedUntil = time.Now().Add(l.coolDown)
		a.count = 0
	}
	l.failed[key] = a
}

// Reset 验证成功后清零。
func (l *VerifyAttemptLimiter) Reset(key int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failed, key)
}
