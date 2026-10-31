package plugin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"easyserver/internal/infra/errx"
)

// sqliteRepo 实现 PluginRepository。
//
// 密钥说明：plugin_secrets 以 AES-256-GCM 加密，密钥由构造方传入
// （当前从面板 JWT 密钥 SHA-256 派生；换取专用密钥只需改 router 接线）。
type sqliteRepo struct {
	db   *sql.DB
	aead cipher.AEAD
}

// NewSQLiteRepository 创建仓储；secretKey 为密钥加密根密钥（任意长度）。
// legacyRootKey 非空且不同于 secretKey 时，把库内以旧根密钥（原从 JWT 密钥
// 派生）加密的条目一次性重加密迁移到新根密钥——JWT 轮换不再殃及插件密钥。
func NewSQLiteRepository(ctx context.Context, db *sql.DB, secretKey, legacyRootKey string) (PluginRepository, error) {
	aead, err := newAEAD(secretKey)
	if err != nil {
		return nil, err
	}
	r := &sqliteRepo{db: db, aead: aead}
	if legacyRootKey != "" && legacyRootKey != secretKey {
		if err := r.migrateFromLegacy(ctx, legacyRootKey); err != nil {
			return nil, fmt.Errorf("migrate plugin secrets: %w", err)
		}
	}
	return r, nil
}

// migrateFromLegacy 把旧根密钥加密的条目重加密为新密钥。
// 解不开的条目跳过（既非旧密钥也非新密钥密文，多半是损坏数据），不阻塞启动。
func (r *sqliteRepo) migrateFromLegacy(ctx context.Context, legacyRootKey string) error {
	legacy, err := newAEAD(legacyRootKey)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT plugin_name, key, encrypted_value FROM plugin_secrets")
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct{ plugin, key, enc string }
	var pending []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.plugin, &it.key, &it.enc); err != nil {
			return err
		}
		pending = append(pending, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	migrated := 0
	for _, it := range pending {
		sealed, err := base64.StdEncoding.DecodeString(it.enc)
		if err != nil {
			continue
		}
		ns := legacy.NonceSize()
		if len(sealed) < ns {
			continue
		}
		plain, err := legacy.Open(nil, sealed[:ns], sealed[ns:], []byte(it.plugin+"/"+it.key))
		if err != nil {
			continue // 新密钥密文或损坏数据
		}
		if err := r.SetSecret(ctx, it.plugin, it.key, string(plain)); err != nil {
			return err
		}
		migrated++
	}
	return nil
}

func newAEAD(key string) (cipher.AEAD, error) {
	h := sha256.Sum256([]byte("easyserver:plugin-secrets:" + key))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (r *sqliteRepo) UpsertPlugin(ctx context.Context, rec *PluginRecord) error {
	approved := strings.Join(rec.PermissionsApproved, ",")
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plugins (name, version, api_version, manifest_path, enabled, status, permissions_approved)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			version = excluded.version,
			api_version = excluded.api_version,
			manifest_path = excluded.manifest_path,
			updated_at = CURRENT_TIMESTAMP`,
		rec.Name, rec.Version, rec.APIVersion, rec.ManifestPath, boolInt(rec.Enabled), rec.Status, approved)
	return err
}

func (r *sqliteRepo) GetPlugin(ctx context.Context, name string) (*PluginRecord, error) {
	row := r.db.QueryRowContext(ctx, pluginSelect+" WHERE name = ?", name)
	return scanPlugin(row.Scan)
}

func (r *sqliteRepo) ListPlugins(ctx context.Context) ([]PluginRecord, error) {
	rows, err := r.db.QueryContext(ctx, pluginSelect+" ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PluginRecord
	for rows.Next() {
		rec, err := scanPlugin(rows.Scan)
		if err != nil {
			logManagerf(" scan row error: %v", err)
			continue
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

const pluginSelect = "SELECT id, name, version, api_version, manifest_path, enabled, status, pid, permissions_approved, restart_count, last_error, last_seen, socket_path, created_at, updated_at FROM plugins"

func scanPlugin(scan func(dest ...any) error) (*PluginRecord, error) {
	var rec PluginRecord
	var enabled int
	var approved, lastErr, lastSeen, socketPath sql.NullString
	if err := scan(&rec.ID, &rec.Name, &rec.Version, &rec.APIVersion, &rec.ManifestPath,
		&enabled, &rec.Status, &rec.PID, &approved, &rec.RestartCount, &lastErr,
		&lastSeen, &socketPath, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	rec.Enabled = enabled != 0
	rec.PermissionsApproved = nil
	if approved.String != "" {
		rec.PermissionsApproved = strings.Split(approved.String, ",")
	}
	rec.LastError = lastErr.String
	rec.LastSeen = lastSeen.String
	rec.SocketPath = socketPath.String
	return &rec, nil
}

func (r *sqliteRepo) SetPluginStatus(ctx context.Context, name, status string, pid int, lastErr string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE plugins SET status = ?, pid = ?, last_error = ?, last_seen = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE name = ?`, status, pid, lastErr, name)
	return err
}

func (r *sqliteRepo) SetPluginEnabled(ctx context.Context, name string, enabled bool, approved []string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE plugins SET enabled = ?, permissions_approved = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?`,
		boolInt(enabled), strings.Join(approved, ","), name)
	return err
}

func (r *sqliteRepo) IncRestartCount(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE plugins SET restart_count = restart_count + 1, updated_at = CURRENT_TIMESTAMP WHERE name = ?", name)
	return err
}

func (r *sqliteRepo) DeletePlugin(ctx context.Context, name string) error {
	// 单事务清理三张表：中途失败整体回滚，避免留下残余行（配置/密钥孤儿）
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // 已提交时 Rollback 为 no-op
	for _, q := range []string{
		"DELETE FROM plugin_secrets WHERE plugin_name = ?",
		"DELETE FROM plugin_config WHERE plugin_name = ?",
		"DELETE FROM plugins WHERE name = ?",
	} {
		if _, err := tx.ExecContext(ctx, q, name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- 配置 KV ----

func (r *sqliteRepo) GetConfig(ctx context.Context, plugin, key string) (string, error) {
	var v sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT value FROM plugin_config WHERE plugin_name = ? AND key = ?", plugin, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errx.NotFound("配置项不存在: %s/%s", plugin, key)
	}
	if err != nil {
		return "", err
	}
	return v.String, nil
}

func (r *sqliteRepo) SetConfig(ctx context.Context, plugin, key, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plugin_config (plugin_name, key, value, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(plugin_name, key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		plugin, key, value)
	return err
}

func (r *sqliteRepo) ListConfig(ctx context.Context, plugin string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT key, value FROM plugin_config WHERE plugin_name = ?", plugin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k.String] = v.String
	}
	return out, rows.Err()
}

func (r *sqliteRepo) DeleteConfig(ctx context.Context, plugin, key string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM plugin_config WHERE plugin_name = ? AND key = ?", plugin, key)
	return err
}

// ---- 密钥（AES-256-GCM，密文 base64）----

func (r *sqliteRepo) SetSecret(ctx context.Context, plugin, key, plaintext string) error {
	nonce := make([]byte, r.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := r.aead.Seal(nonce, nonce, []byte(plaintext), []byte(plugin+"/"+key))
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plugin_secrets (plugin_name, key, encrypted_value, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(plugin_name, key) DO UPDATE SET encrypted_value = excluded.encrypted_value, updated_at = CURRENT_TIMESTAMP`,
		plugin, key, base64.StdEncoding.EncodeToString(sealed))
	return err
}

func (r *sqliteRepo) GetSecret(ctx context.Context, plugin, key string) (string, error) {
	var enc string
	err := r.db.QueryRowContext(ctx, "SELECT encrypted_value FROM plugin_secrets WHERE plugin_name = ? AND key = ?", plugin, key).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errx.NotFound("密钥不存在: %s/%s", plugin, key)
	}
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := r.aead.NonceSize()
	if len(sealed) < ns {
		return "", errors.New("密文长度异常")
	}
	plain, err := r.aead.Open(nil, sealed[:ns], sealed[ns:], []byte(plugin+"/"+key))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (r *sqliteRepo) DeleteSecret(ctx context.Context, plugin, key string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM plugin_secrets WHERE plugin_name = ? AND key = ?", plugin, key)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
