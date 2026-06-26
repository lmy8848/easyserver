package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type sqliteTokenRepo struct {
	db *sql.DB
}

func NewSQLiteTokenRepository(db *sql.DB) TokenBlacklistRepo {
	return &sqliteTokenRepo{db: db}
}

func (r *sqliteTokenRepo) Add(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	return err
}

func (r *sqliteTokenRepo) IsBlacklisted(ctx context.Context, token string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM token_blacklist WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqliteTokenRepo) AddUserInvalidation(ctx context.Context, userID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM token_blacklist WHERE user_id = ? AND token LIKE 'user_%_all'", userID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, fmt.Sprintf("user_%d_all", userID), time.Now().Add(365*24*time.Hour),
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *sqliteTokenRepo) IsUserInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT created_at FROM token_blacklist
		 WHERE user_id = ? AND token LIKE 'user_%_all' AND expires_at > datetime('now')
		 ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, nil
	}
	return issuedAt.Before(createdAt), nil
}

func (r *sqliteTokenRepo) Clean(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM token_blacklist WHERE expires_at < ?", time.Now())
	return err
}
