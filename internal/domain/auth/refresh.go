package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"easyserver/internal/infra/errx"
)

// RefreshService 管理不透明刷新令牌：只存 SHA-256 哈希、按设备绑定、
// 一次性轮换、整链（family）吊销、旧令牌重放检测。
//
// 设计要点（见 ADR / 审查记录）：
//   - refresh 是 32 字节随机串，泄漏后服务端可立即吊销，不像 JWT 只能等过期；
//   - 每次使用都轮换，旧令牌保留 rotated_at，若在宽限期外再次使用 → 判定重放，
//     吊销整个 family；
//   - 并发刷新（同一设备多个请求同时 401）在 ClientGrace 内返回同一对新令牌，
//     避免误判重放；
//   - 设备吊销 / 退出登录 / 改密码 → 吊销对应 family，access 最长 15 分钟自然过期。
const (
	refreshTokenBytes = 32
	RefreshTTL        = 90 * 24 * time.Hour
	refreshGrace      = 60 * time.Second
)

type RefreshService struct {
	db *sql.DB
}

func NewRefreshService(db *sql.DB) *RefreshService { return &RefreshService{db: db} }

type RefreshPair struct {
	Token     string // 新 refresh 明文（只返回给客户端一次）
	ExpiresAt time.Time
}

func hashRefresh(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newOpaque(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Issue 为登录成功的设备签发一条新的 refresh 链。
func (s *RefreshService) Issue(ctx context.Context, userID int64, deviceID string) (*RefreshPair, error) {
	token, err := newOpaque(refreshTokenBytes)
	if err != nil {
		return nil, err
	}
	family, err := newOpaque(16)
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(RefreshTTL)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (token_hash, family_id, user_id, device_id, expires_at)
		VALUES (?, ?, ?, ?, ?)`, hashRefresh(token), family, userID, deviceID, exp)
	if err != nil {
		return nil, fmt.Errorf("issue refresh: %w", err)
	}
	return &RefreshPair{Token: token, ExpiresAt: exp}, nil
}

// Rotate 校验旧 refresh 并签发新的一对（同一 family）。返回的新 refresh 只给客户端一次。
func (s *RefreshService) Rotate(ctx context.Context, oldToken, deviceID string) (*RefreshPair, error) {
	oldHash := hashRefresh(oldToken)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		id        int64
		family    string
		userID    int64
		exp       time.Time
		rotatedAt sql.NullTime
		revokedAt sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT id, family_id, user_id, expires_at, rotated_at, revoked_at
		FROM refresh_tokens WHERE token_hash = ?`, oldHash).
		Scan(&id, &family, &userID, &exp, &rotatedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return nil, errx.Unauthorized("refresh token 无效")
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid || time.Now().After(exp) {
		return nil, errx.Unauthorized("refresh token 已失效")
	}
	// 设备绑定：换设备的令牌不能复用
	var boundDevice string
	if err := tx.QueryRowContext(ctx, `SELECT device_id FROM refresh_tokens WHERE id = ?`, id).Scan(&boundDevice); err != nil {
		return nil, err
	}
	if deviceID != "" && boundDevice != deviceID {
		return nil, errx.Unauthorized("refresh token 与设备不匹配")
	}

	// 已在宽限期内被轮换过：返回同一个 successor（客户端重试拿到同一对，避免误判重放）
	if rotatedAt.Valid && rotatedAt.Time.Add(refreshGrace).After(time.Now()) {
		var next string
		if err := tx.QueryRowContext(ctx, `
			SELECT token_hash FROM refresh_tokens
			WHERE rotated_from = ? AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
			ORDER BY id DESC LIMIT 1`, oldHash).Scan(&next); err != nil {
			return nil, errx.Unauthorized("refresh token 已轮换")
		}
		_ = tx.Commit()
		// successor 明文不存库，无法回放：宽限期内也改为签发新令牌，但保留旧记录
		return s.issueInFamily(ctx, family, userID, boundDevice)
	}
	// 已被轮换且超过宽限：判定重放，吊销整链
	if rotatedAt.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE family_id = ?`, family); err != nil {
			return nil, err
		}
		_ = tx.Commit()
		return nil, errx.Unauthorized("refresh token 已被使用，整链已吊销")
	}

	// 第一次轮换：标记旧令牌并签发新令牌
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET rotated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, err
	}
	pair, err := s.issueInTx(ctx, tx, family, userID, boundDevice, oldHash)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

// Identity 返回 refresh 所属用户与用户名（刷新时重新签发 access 用）。
func (s *RefreshService) Identity(ctx context.Context, token string) (int64, string, error) {
	var uid int64
	var uname string
	err := s.db.QueryRowContext(ctx, `
		SELECT r.user_id, u.username FROM refresh_tokens r
		JOIN users u ON u.id = r.user_id
		WHERE r.token_hash = ?`, hashRefresh(token)).Scan(&uid, &uname)
	if err == sql.ErrNoRows {
		return 0, "", errx.Unauthorized("refresh token 无效")
	}
	if err != nil {
		return 0, "", err
	}
	return uid, uname, nil
}

// RevokeFamily 吊销某设备/用户的所有 refresh（登出、吊销设备、改密码）。
func (s *RefreshService) RevokeFamily(ctx context.Context, userID int64, deviceID string) error {
	if deviceID != "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP
			WHERE user_id = ? AND device_id = ? AND revoked_at IS NULL`, userID, deviceID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND revoked_at IS NULL`, userID)
	return err
}

// CleanupExpired 删除过期或吊销超过保留期的记录（每日调用一次即可）。
func (s *RefreshService) CleanupExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM refresh_tokens
		WHERE expires_at < datetime('now', '-7 days')
		   OR revoked_at < datetime('now', '-7 days')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *RefreshService) issueInFamily(ctx context.Context, family string, userID int64, deviceID string) (*RefreshPair, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	pair, err := s.issueInTx(ctx, tx, family, userID, deviceID, "")
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pair, nil
}

func (s *RefreshService) issueInTx(ctx context.Context, tx *sql.Tx, family string, userID int64, deviceID, rotatedFrom string) (*RefreshPair, error) {
	token, err := newOpaque(refreshTokenBytes)
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(RefreshTTL)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (token_hash, family_id, user_id, device_id, expires_at, rotated_from)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))`, hashRefresh(token), family, userID, deviceID, exp, rotatedFrom)
	if err != nil {
		return nil, err
	}
	return &RefreshPair{Token: token, ExpiresAt: exp}, nil
}
