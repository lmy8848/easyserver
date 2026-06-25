package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// SessionRepository implements repository.SessionRepository for SQLite
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a new SessionRepository
func NewSessionRepository(db *sql.DB) repository.SessionRepository {
	return &SessionRepository{db: db}
}

// Create creates a new session
func (r *SessionRepository) Create(ctx context.Context, session *model.Session) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, username, role, ip, user_agent, last_active, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.Token, session.UserID, session.Username, session.Role,
		session.IP, session.UserAgent, time.Now(), session.ExpiresAt,
	)
	return err
}

// GetByToken returns a session by token
func (r *SessionRepository) GetByToken(ctx context.Context, token string) (*model.Session, error) {
	session := &model.Session{}
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

// DeleteByToken deletes a session by token
func (r *SessionRepository) DeleteByToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	return err
}

// DeleteByUserID deletes all sessions for a user
func (r *SessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// DeleteExpired deletes expired sessions
func (r *SessionRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// GetActiveByUserID returns active sessions for a user
func (r *SessionRepository) GetActiveByUserID(ctx context.Context, userID int64) ([]model.Session, error) {
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

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
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

// GetActive returns all active sessions
func (r *SessionRepository) GetActive(ctx context.Context) ([]model.Session, error) {
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

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
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

// UpdateActivity updates the last_active timestamp
func (r *SessionRepository) UpdateActivity(ctx context.Context, token string) error {
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

// Count returns the count of active sessions
func (r *SessionRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE last_active > ? AND expires_at > ?",
		time.Now().Add(-5*time.Minute), time.Now(),
	).Scan(&count)
	return count, err
}
