// Package mobiledirect 实现移动端专用直连通道：独立 TLS 端口 + Ed25519 设备签名，
// 绕过 Cloudflare 直达源站。仅代理 /api/ 业务接口，不提供面板页面；
// 未通过设备签名的请求一律 401，扫描者拿不到任何可利用入口。
package mobiledirect

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// 设备签名请求头。签名覆盖：METHOD\nREQUEST-URI\n时间戳\n随机串\nbody 的 SHA-256。
const (
	HeaderDeviceID  = "X-Device-Id"
	HeaderTimestamp = "X-Device-Timestamp"
	HeaderNonce     = "X-Device-Nonce"
	HeaderSignature = "X-Device-Signature"

	maxInlineBodyHash = 1 << 20   // ≤1MB 内存缓冲，更大走临时文件
	maxDirectBodySize = 512 << 20 // 与面板上传上限一致
)

// DeviceKey 是一台已注册设备的公钥记录。PublicKey 不参与 JSON 输出。
type DeviceKey struct {
	ID        int64  `json:"id"`
	DeviceID  string `json:"device_id"`
	UserID    int64  `json:"user_id"`
	Name      string `json:"name"`
	PublicKey string `json:"-"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen_at,omitempty"`
	Revoked   bool   `json:"revoked"`
}

// Store 负责 device_keys 表的读写。
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Register 注册或更新设备公钥（同一 device_id 只能属于一个用户）。
func (s *Store) Register(ctx context.Context, userID int64, deviceID, name, publicKeyB64 string) (*DeviceKey, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(deviceID) > 128 {
		return nil, errx.BadRequest("device_id 无效")
	}
	publicKeyB64 = strings.TrimSpace(publicKeyB64)
	pub, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errx.BadRequest("public_key 需为 base64 编码的 32 字节 Ed25519 公钥")
	}
	if name == "" {
		name = deviceID
	}
	// 幂等：同设备重复注册只更新公钥与名称；被吊销的设备重新注册即恢复。
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO device_keys (device_id, user_id, public_key, name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			public_key = excluded.public_key,
			name = excluded.name,
			revoked_at = NULL
		WHERE device_keys.user_id = excluded.user_id`,
		deviceID, userID, publicKeyB64, name)
	if err != nil {
		return nil, errx.Internal("保存设备公钥失败: %w", err)
	}
	return s.GetByDeviceID(ctx, deviceID)
}

// GetByDeviceID 按设备号取设备记录（含已吊销标记，调用方自行判断）。
func (s *Store) GetByDeviceID(ctx context.Context, deviceID string) (*DeviceKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, device_id, user_id, name, public_key, created_at, last_seen_at, revoked_at
		FROM device_keys WHERE device_id = ?`, deviceID)
	var d DeviceKey
	var lastSeen, revoked sql.NullString
	err := row.Scan(&d.ID, &d.DeviceID, &d.UserID, &d.Name, &d.PublicKey, &d.CreatedAt, &lastSeen, &revoked)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.LastSeen = lastSeen.String
	d.Revoked = revoked.Valid
	return &d, nil
}

// GetByID 按主键取设备（限本人），用于吊销时拿到 device_id 以连带吊销 refresh。
func (s *Store) GetByID(ctx context.Context, userID, id int64) (*DeviceKey, error) {
	var d DeviceKey
	var lastSeen, revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, device_id, user_id, name, created_at, last_seen_at, revoked_at
		FROM device_keys WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&d.ID, &d.DeviceID, &d.UserID, &d.Name, &d.CreatedAt, &lastSeen, &revoked)
	if err == sql.ErrNoRows {
		return nil, errx.NotFound("设备不存在")
	}
	if err != nil {
		return nil, err
	}
	d.LastSeen = lastSeen.String
	d.Revoked = revoked.Valid
	return &d, nil
}

// List 返回某用户的全部设备（含已吊销，便于前端展示）。
func (s *Store) List(ctx context.Context, userID int64) ([]DeviceKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, device_id, user_id, name, created_at, last_seen_at, revoked_at
		FROM device_keys WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceKey
	for rows.Next() {
		var d DeviceKey
		var lastSeen, revoked sql.NullString
		if err := rows.Scan(&d.ID, &d.DeviceID, &d.UserID, &d.Name, &d.CreatedAt, &lastSeen, &revoked); err != nil {
			return nil, err
		}
		d.LastSeen = lastSeen.String
		d.Revoked = revoked.Valid
		out = append(out, d)
	}
	return out, rows.Err()
}

// Revoke 吊销设备（只允许设备所属用户操作）。
func (s *Store) Revoke(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE device_keys SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errx.NotFound("设备不存在或已吊销")
	}
	return nil
}

// Touch 记录最近使用时间（失败不影响请求）。
func (s *Store) Touch(ctx context.Context, deviceID string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE device_keys SET last_seen_at = CURRENT_TIMESTAMP WHERE device_id = ?`, deviceID)
}

// replayGuard 记录近期已用过的 (device_id, nonce)，阻止重放。
type replayGuard struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	ttl   time.Duration
	limit int
}

func newReplayGuard(ttl time.Duration) *replayGuard {
	return &replayGuard{seen: make(map[string]time.Time), ttl: ttl, limit: 20000}
}

// seenBefore 返回 true 表示该 nonce 仍在有效期内（即重放）。
func (g *replayGuard) seenBefore(key string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if exp, ok := g.seen[key]; ok && now.Before(exp) {
		return true
	}
	if len(g.seen) > g.limit {
		for k, exp := range g.seen {
			if now.After(exp) {
				delete(g.seen, k)
			}
		}
	}
	g.seen[key] = now.Add(g.ttl)
	return false
}

// cachedToken 是直连通道为设备铸造的短期会话令牌（15 分钟），按设备缓存，
// 避免每个请求都写一条会话记录。
type cachedToken struct {
	token string
	exp   time.Time
}

// tokenForDevice 返回设备可用的短期 JWT：缓存命中且未临近过期则复用，
// 否则调用 mint 铸造并写入会话表（主引擎的 JWT+会话校验因此天然通过）。
func (s *Server) tokenForDevice(ctx context.Context, deviceID string, userID int64) (string, error) {
	s.tokMu.Lock()
	defer s.tokMu.Unlock()
	if t, ok := s.tokens[deviceID]; ok && time.Now().Before(t.exp.Add(-2*time.Minute)) {
		return t.token, nil
	}
	tok, exp, err := s.mint(ctx, userID)
	if err != nil {
		return "", err
	}
	s.tokens[deviceID] = cachedToken{token: tok, exp: exp}
	return tok, nil
}

// deviceAuth 校验设备签名并把 user_id/username 注入上下文。
// 失败一律返回笼统 401（不区分设备不存在/签名错误/已吊销，避免探测）。
func (s *Server) deviceAuth(c *gin.Context) {
	deviceID := c.GetHeader(HeaderDeviceID)
	tsStr := c.GetHeader(HeaderTimestamp)
	nonce := c.GetHeader(HeaderNonce)
	sigB64 := c.GetHeader(HeaderSignature)
	if deviceID == "" || tsStr == "" || nonce == "" || sigB64 == "" ||
		len(nonce) > 64 || len(tsStr) > 20 || len(deviceID) > 128 {
		abortUnauthorized(c)
		return
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		abortUnauthorized(c)
		return
	}
	now := time.Now()
	skew := s.cfg.ClockSkew.Duration()
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	signedAt := time.Unix(ts, 0)
	if now.Sub(signedAt) > skew || signedAt.Sub(now) > skew {
		abortUnauthorized(c)
		return
	}

	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		abortUnauthorized(c)
		return
	}

	// 读取请求体计算摘要，随后还原 body 供代理转发（大请求落临时文件）。
	bodyHash, err := bufferBodyForHash(c.Request)
	if err != nil {
		abortUnauthorized(c)
		return
	}

	canonical := strings.Join([]string{
		c.Request.Method,
		c.Request.URL.RequestURI(),
		tsStr,
		nonce,
		bodyHash,
	}, "\n")

	device, err := s.store.GetByDeviceID(c.Request.Context(), deviceID)
	if err != nil || device == nil || device.Revoked {
		abortUnauthorized(c)
		return
	}
	pub, err := base64.StdEncoding.DecodeString(device.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		abortUnauthorized(c)
		return
	}
	if !ed25519.Verify(pub, []byte(canonical), sig) {
		abortUnauthorized(c)
		return
	}

	if s.replay.seenBefore(deviceID+"\x00"+nonce, now) {
		abortUnauthorized(c)
		return
	}

	// 为设备铸造短期 JWT，并以 Cookie 注入转发请求：主引擎的 JWT/会话中间件、
	// 审计、WebSocket 鉴权都按正常登录态处理，无需单独的旁路。
	tok, err := s.tokenForDevice(c.Request.Context(), device.DeviceID, device.UserID)
	if err != nil {
		abortUnauthorized(c)
		return
	}
	c.Request.Header.Set("Cookie", middleware.AuthCookieName+"="+tok)
	c.Set("device_id", device.DeviceID)
	s.store.Touch(c.Request.Context(), device.DeviceID)
	c.Next()
}

// abortUnauthorized 返回统一 401，不泄漏失败原因。
func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "unauthorized", "data": nil})
}

// bufferBodyForHash 读出请求体并计算 SHA-256，随后把 body 还原给代理转发。
// ≤1MB 用内存；更大的请求落临时文件，避免大上传把内存打满。
func bufferBodyForHash(req *http.Request) (string, error) {
	h := sha256.New()
	if req.Body == nil {
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	if req.ContentLength >= 0 && req.ContentLength <= maxInlineBodyHash {
		data, err := io.ReadAll(io.LimitReader(req.Body, maxInlineBodyHash+1))
		if err != nil {
			return "", err
		}
		if len(data) > maxInlineBodyHash {
			return "", fmt.Errorf("body too large for inline buffering")
		}
		h.Write(data)
		req.Body = io.NopCloser(strings.NewReader(string(data)))
		req.ContentLength = int64(len(data))
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	f, err := os.CreateTemp("", "easyserver-md-*")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(req.Body, maxDirectBodySize+1))
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if n > maxDirectBodySize {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("body too large")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	req.Body = &tempFileBody{File: f}
	req.ContentLength = n
	return hex.EncodeToString(h.Sum(nil)), nil
}

// tempFileBody 关闭时删除临时文件。
type tempFileBody struct{ *os.File }

func (t *tempFileBody) Close() error {
	name := t.Name()
	err := t.File.Close()
	_ = os.Remove(name)
	return err
}
