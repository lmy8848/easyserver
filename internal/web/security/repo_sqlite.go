package security

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type sqliteSecurityRepo struct {
	db *sql.DB
}

func NewSQLiteSecurityRepository(db *sql.DB) SecurityRepository {
	return &sqliteSecurityRepo{db: db}
}

// GetConfig returns the security config for a website (or error if not found).
func (r *sqliteSecurityRepo) GetConfig(ctx context.Context, websiteID int64) (*SecurityConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, website_id, rate_limit_enabled, rate_limit_rate, rate_limit_burst,
		       limit_conn, auto_ban_enabled, auto_ban_threshold, auto_ban_404_threshold,
		       auto_ban_duration, created_at, updated_at
		FROM website_security_config WHERE website_id = ?`, websiteID)
	return scanSecurityConfig(row)
}

// CreateConfig inserts a default security config for a website.
func (r *sqliteSecurityRepo) CreateConfig(ctx context.Context, websiteID int64) (*SecurityConfig, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO website_security_config (website_id) VALUES (?)`, websiteID)
	if err != nil {
		return nil, fmt.Errorf("create security config: %w", err)
	}
	return r.GetConfig(ctx, websiteID)
}

// UpdateConfig writes mutable fields of the config.
func (r *sqliteSecurityRepo) UpdateConfig(ctx context.Context, cfg *SecurityConfig) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE website_security_config SET
			rate_limit_enabled = ?, rate_limit_rate = ?, rate_limit_burst = ?,
			limit_conn = ?, auto_ban_enabled = ?, auto_ban_threshold = ?,
			auto_ban_404_threshold = ?, auto_ban_duration = ?,
			updated_at = datetime('now')
		WHERE website_id = ?`,
		cfg.RateLimitEnabled, cfg.RateLimitRate, cfg.RateLimitBurst,
		cfg.LimitConn, cfg.AutoBanEnabled, cfg.AutoBanThreshold,
		cfg.AutoBan404Threshold, cfg.AutoBanDuration, cfg.WebsiteID)
	return err
}

// ListBannedIPs returns bans scoped to a website (including global bans).
func (r *sqliteSecurityRepo) ListBannedIPs(ctx context.Context, websiteID int64) ([]BannedIP, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, website_id, ip, reason, source, expires_at, created_at
		FROM website_banned_ip
		WHERE website_id = ? OR website_id IS NULL
		ORDER BY created_at DESC`, websiteID)
	if err != nil {
		return nil, fmt.Errorf("list banned ips: %w", err)
	}
	defer rows.Close()
	return scanBannedIPRows(rows)
}

// ListGlobalBannedIPs returns only global (website_id IS NULL) bans.
func (r *sqliteSecurityRepo) ListGlobalBannedIPs(ctx context.Context) ([]BannedIP, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, website_id, ip, reason, source, expires_at, created_at
		FROM website_banned_ip WHERE website_id IS NULL
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list global banned ips: %w", err)
	}
	defer rows.Close()
	return scanBannedIPRows(rows)
}

// ListAllBannedIPs returns every ban record (global + per-website).
func (r *sqliteSecurityRepo) ListAllBannedIPs(ctx context.Context) ([]BannedIP, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, website_id, ip, reason, source, expires_at, created_at
		FROM website_banned_ip ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all banned ips: %w", err)
	}
	defer rows.Close()
	return scanBannedIPRows(rows)
}

// GetBannedIP returns a single ban record by ID.
func (r *sqliteSecurityRepo) GetBannedIP(ctx context.Context, id int64) (*BannedIP, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, website_id, ip, reason, source, expires_at, created_at
		FROM website_banned_ip WHERE id = ?`, id)
	return scanBannedIP(row)
}

// AddBannedIP inserts a ban record.
func (r *sqliteSecurityRepo) AddBannedIP(ctx context.Context, websiteID *int64, ip, reason, source string, expiresAt *time.Time) (*BannedIP, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO website_banned_ip (website_id, ip, reason, source, expires_at)
		VALUES (?, ?, ?, ?, ?)`, websiteID, ip, reason, source, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("add banned ip: %w", err)
	}
	id, _ := res.LastInsertId()
	row := r.db.QueryRowContext(ctx, `
		SELECT id, website_id, ip, reason, source, expires_at, created_at
		FROM website_banned_ip WHERE id = ?`, id)
	return scanBannedIP(row)
}

// RemoveBannedIP deletes a ban record by ID.
func (r *sqliteSecurityRepo) RemoveBannedIP(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM website_banned_ip WHERE id = ?`, id)
	return err
}

// RemoveExpiredBannedIPs deletes bans whose expires_at is in the past.
func (r *sqliteSecurityRepo) RemoveExpiredBannedIPs(ctx context.Context) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM website_banned_ip WHERE expires_at IS NOT NULL AND expires_at < datetime('now')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// IsIPBanned checks if an IP is currently banned for a website (or globally).
func (r *sqliteSecurityRepo) IsIPBanned(ctx context.Context, ip string, websiteID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM website_banned_ip
		WHERE ip = ? AND (website_id = ? OR website_id IS NULL)
		  AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		ip, websiteID).Scan(&count)
	return count > 0, err
}

// --- scan helpers ---

func scanSecurityConfig(row *sql.Row) (*SecurityConfig, error) {
	var cfg SecurityConfig
	err := row.Scan(&cfg.ID, &cfg.WebsiteID, &cfg.RateLimitEnabled, &cfg.RateLimitRate,
		&cfg.RateLimitBurst, &cfg.LimitConn, &cfg.AutoBanEnabled, &cfg.AutoBanThreshold,
		&cfg.AutoBan404Threshold, &cfg.AutoBanDuration, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan security config: %w", err)
	}
	return &cfg, nil
}

func scanBannedIP(row *sql.Row) (*BannedIP, error) {
	var b BannedIP
	var websiteID sql.NullInt64
	err := row.Scan(&b.ID, &websiteID, &b.IP, &b.Reason, &b.Source, &b.ExpiresAt, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan banned ip: %w", err)
	}
	if websiteID.Valid {
		b.WebsiteID = &websiteID.Int64
	}
	return &b, nil
}

func scanBannedIPRows(rows *sql.Rows) ([]BannedIP, error) {
	var bans []BannedIP
	for rows.Next() {
		var b BannedIP
		var websiteID sql.NullInt64
		if err := rows.Scan(&b.ID, &websiteID, &b.IP, &b.Reason, &b.Source, &b.ExpiresAt, &b.CreatedAt); err != nil {
			continue
		}
		if websiteID.Valid {
			b.WebsiteID = &websiteID.Int64
		}
		bans = append(bans, b)
	}
	return bans, rows.Err()
}
