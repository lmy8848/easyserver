package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/repository"
)

// TokenBlacklistRepository implements repository.TokenBlacklistRepository for SQLite
type TokenBlacklistRepository struct {
	db *sql.DB
}

// NewTokenBlacklistRepository creates a new TokenBlacklistRepository
func NewTokenBlacklistRepository(db *sql.DB) repository.TokenBlacklistRepository {
	return &TokenBlacklistRepository{db: db}
}

// Add adds a token to the blacklist
func (r *TokenBlacklistRepository) Add(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	return err
}

// IsBlacklisted checks if a token is blacklisted
func (r *TokenBlacklistRepository) IsBlacklisted(ctx context.Context, token string) (bool, error) {
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

// AddUserInvalidation adds a user invalidation marker.
// Uses a transaction to atomically delete old markers and insert the new one,
// closing the race window where a concurrent check could see no marker.
func (r *TokenBlacklistRepository) AddUserInvalidation(ctx context.Context, userID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete any existing markers for this user first
	if _, err := tx.ExecContext(ctx, "DELETE FROM token_blacklist WHERE user_id = ? AND token LIKE 'user_%_all'", userID); err != nil {
		return err
	}

	// Add a marker with the current time as the invalidation point
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, fmt.Sprintf("user_%d_all", userID), time.Now().Add(365*24*time.Hour),
	); err != nil {
		return err
	}

	return tx.Commit()
}

// IsUserInvalidated checks if a user's tokens have been invalidated
func (r *TokenBlacklistRepository) IsUserInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
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
		return false, nil // On error, assume not invalidated
	}
	return issuedAt.Before(createdAt), nil
}

// Clean removes expired tokens from the blacklist
func (r *TokenBlacklistRepository) Clean(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM token_blacklist WHERE expires_at < ?", time.Now())
	return err
}
