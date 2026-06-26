package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type sqliteSessionRepo struct {
	db *sql.DB
}

func NewSQLiteSessionRepository(db *sql.DB) SessionRepo {
	return &sqliteSessionRepo{db: db}
}

func (r *sqliteSessionRepo) Create(ctx context.Context, session *Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, username, role, ip, user_agent, last_active, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.Token, session.UserID, session.Username, session.Role,
		session.IP, session.UserAgent, time.Now(), session.ExpiresAt,
	)
	return err
}

func (r *sqliteSessionRepo) GetByToken(ctx context.Context, token string) (*Session, error) {
	session := &Session{}
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, username, role, ip, user_agent, last_active, expires_at
		 FROM sessions WHERE token = ?`, token,
	).Scan(
		&session.UserID, &session.Username, &session.Role,
		&session.IP, &session.UserAgent, &session.LoginAt, &session.ExpiresAt,
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
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
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
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token != ?", userID, exceptToken)
	return err
}

func (r *sqliteSessionRepo) IsValid(ctx context.Context, token string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqliteSessionRepo) GetActiveByUserID(ctx context.Context, userID int64) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, username, role, ip, user_agent, last_active, expires_at, token
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
		`SELECT user_id, username, role, ip, user_agent, last_active, expires_at, token
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
		time.Now(), token,
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
