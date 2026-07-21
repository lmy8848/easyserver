package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type sqliteSessionRepo struct {
	db          *sql.DB
	idleTimeout time.Duration
}

func NewSQLiteSessionRepository(db *sql.DB, idleTimeout time.Duration) SessionRepo {
	return &sqliteSessionRepo{db: db, idleTimeout: idleTimeout}
}

func (r *sqliteSessionRepo) Create(ctx context.Context, session *Session) error {
	// Store only a SHA-256 of the token, never the plaintext JWT.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, username, role, ip, user_agent, client_type, device_id, device_info, last_active, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hashToken(session.Token), session.UserID, session.Username, session.Role,
		session.IP, session.UserAgent, session.ClientType, session.DeviceID, session.DeviceInfo,
		time.Now(), session.ExpiresAt,
	)
	return err
}

func (r *sqliteSessionRepo) GetByToken(ctx context.Context, token string) (*Session, error) {
	session := &Session{}
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, username, role, ip, user_agent, client_type, device_id, device_info, last_active, expires_at
		 FROM sessions WHERE token = ?`, hashToken(token),
	).Scan(
		&session.UserID, &session.Username, &session.Role,
		&session.IP, &session.UserAgent, &session.ClientType, &session.DeviceID, &session.DeviceInfo,
		&session.LoginAt, &session.ExpiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}
	session.Token = token
	return session, nil
}

func (r *sqliteSessionRepo) DeleteByToken(ctx context.Context, token string) error {
	// Expects a PLAINTEXT token; hashes internally (used by Logout which passes
	// the raw JWT). For the hash-already-stored path see DeleteByStoredToken.
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", hashToken(token))
	return err
}

func (r *sqliteSessionRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (r *sqliteSessionRepo) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

func (r *sqliteSessionRepo) DeleteInactive(ctx context.Context, inactiveSince time.Time) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE last_active < ?", inactiveSince)
	return err
}

func (r *sqliteSessionRepo) DeleteByUserIDExcept(ctx context.Context, userID int64, exceptToken string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token != ?", userID, hashToken(exceptToken))
	return err
}

// DeleteByStoredToken deletes by the stored (already-hashed) token value as-is,
// without re-hashing. The kick path and same-device mobile refresh pass hashes
// obtained from GetActiveByUserID, so hashing again would never match.
func (r *sqliteSessionRepo) DeleteByStoredToken(ctx context.Context, storedToken string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", storedToken)
	return err
}

// DeleteMobileByUserID removes all mobile sessions for a user (same-device
// refresh / orphan cleanup when binding is enforced).
func (r *sqliteSessionRepo) DeleteMobileByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND client_type = 'mobile'", userID)
	return err
}

// DeleteMobileByUserIDExcept removes all mobile sessions for a user except the
// one matching exceptToken (plaintext, hashed for comparison). Used to drop the
// old mobile session after the new one is created (same-device refresh).
func (r *sqliteSessionRepo) DeleteMobileByUserIDExcept(ctx context.Context, userID int64, exceptToken string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND client_type = 'mobile' AND token != ?", userID, hashToken(exceptToken))
	return err
}

func (r *sqliteSessionRepo) IsValid(ctx context.Context, token string) (bool, error) {
	var count int
	// When idleTimeout <= 0 the idle check is disabled; only expiry applies.
	if r.idleTimeout > 0 {
		err := r.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at > ? AND last_active > ?",
			hashToken(token), time.Now(), time.Now().Add(-r.idleTimeout),
		).Scan(&count)
		if err != nil {
			return false, err
		}
	} else {
		err := r.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at > ?",
			hashToken(token), time.Now(),
		).Scan(&count)
		if err != nil {
			return false, err
		}
	}
	return count > 0, nil
}

func (r *sqliteSessionRepo) GetActiveByUserID(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, username, role, ip, user_agent, client_type, device_id, device_info, last_active, expires_at, token
		 FROM sessions WHERE user_id = ? AND expires_at > ?
		 ORDER BY last_active DESC`,
		userID, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.UserID, &s.Username, &s.Role, &s.IP, &s.UserAgent,
			&s.ClientType, &s.DeviceID, &s.DeviceInfo,
			&s.LoginAt, &s.ExpiresAt, &s.Token,
		); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *sqliteSessionRepo) GetActive(ctx context.Context) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, username, role, ip, user_agent, client_type, device_id, device_info, last_active, expires_at, token
		 FROM sessions WHERE last_active > ? AND expires_at > ?
		 ORDER BY last_active DESC`,
		time.Now().Add(-5*time.Minute), time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.UserID, &s.Username, &s.Role, &s.IP, &s.UserAgent,
			&s.ClientType, &s.DeviceID, &s.DeviceInfo,
			&s.LoginAt, &s.ExpiresAt, &s.Token,
		); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *sqliteSessionRepo) UpdateActivity(ctx context.Context, token string) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE sessions SET last_active = ? WHERE token = ?",
		time.Now(), hashToken(token),
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

func (r *sqliteSessionRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE last_active > ? AND expires_at > ?",
		time.Now().Add(-5*time.Minute), time.Now(),
	).Scan(&count)
	return count, err
}
