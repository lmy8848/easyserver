package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// UserRepository implements repository.UserRepository for SQLite
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sql.DB) repository.UserRepository {
	return &UserRepository{db: db}
}

// GetByID returns a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*model.User, error) {
	user := &model.User{}
	var mustChangePass, totpEnabled int
	var lastLogin, lockedUntil, expiresAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, must_change_pass, last_login, last_login_ip,
		        login_attempts, locked_until, expires_at, ip_whitelist,
		        totp_secret, totp_enabled, totp_backup_codes, created_at, updated_at
		 FROM users WHERE id = ?`, id,
	).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role,
		&mustChangePass, &lastLogin, &user.LastLoginIP,
		&user.LoginAttempts, &lockedUntil, &expiresAt, &user.IPWhitelist,
		&user.TotpSecret, &totpEnabled, &user.TotpBackupCodes,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user.MustChangePass = mustChangePass != 0
	user.TotpEnabled = totpEnabled != 0
	user.LastLogin = lastLogin
	user.LockedUntil = lockedUntil
	user.ExpiresAt = expiresAt

	return user, nil
}

// GetByUsername returns a user by username
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	user := &model.User{}
	var mustChangePass, totpEnabled int
	var lastLogin, lockedUntil, expiresAt sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, must_change_pass, last_login, last_login_ip,
		        login_attempts, locked_until, expires_at, ip_whitelist,
		        totp_secret, totp_enabled, totp_backup_codes, created_at, updated_at
		 FROM users WHERE username = ?`, username,
	).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role,
		&mustChangePass, &lastLogin, &user.LastLoginIP,
		&user.LoginAttempts, &lockedUntil, &expiresAt, &user.IPWhitelist,
		&user.TotpSecret, &totpEnabled, &user.TotpBackupCodes,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	user.MustChangePass = mustChangePass != 0
	user.TotpEnabled = totpEnabled != 0
	user.LastLogin = lastLogin
	user.LockedUntil = lockedUntil
	user.ExpiresAt = expiresAt

	return user, nil
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, must_change_pass, ip_whitelist)
		 VALUES (?, ?, ?, ?, ?)`,
		user.Username, user.PasswordHash, user.Role, user.MustChangePass, user.IPWhitelist,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id

	return nil
}

// Update updates an existing user
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = ?, role = ?, ip_whitelist = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		user.Username, user.Role, user.IPWhitelist, user.ID,
	)
	return err
}

// Delete deletes a user by ID
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	return err
}

// List returns a paginated list of users
func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]model.User, int64, error) {
	// Get total count
	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get users
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, password_hash, role, must_change_pass, last_login, last_login_ip,
		        login_attempts, locked_until, expires_at, ip_whitelist,
		        totp_secret, totp_enabled, totp_backup_codes, created_at, updated_at
		 FROM users ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var user model.User
		var mustChangePass, totpEnabled int
		var lastLogin, lockedUntil, expiresAt sql.NullTime

		if err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Role,
			&mustChangePass, &lastLogin, &user.LastLoginIP,
			&user.LoginAttempts, &lockedUntil, &expiresAt, &user.IPWhitelist,
			&user.TotpSecret, &totpEnabled, &user.TotpBackupCodes,
			&user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			continue
		}

		user.MustChangePass = mustChangePass != 0
		user.TotpEnabled = totpEnabled != 0
		user.LastLogin = lastLogin
		user.LockedUntil = lockedUntil
		user.ExpiresAt = expiresAt

		users = append(users, user)
	}

	return users, total, nil
}

// UpdateLoginAttempts updates login attempts and lock status
func (r *UserRepository) UpdateLoginAttempts(ctx context.Context, id int64, attempts int, lockedUntil *time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET login_attempts = ?, locked_until = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		attempts, lockedUntil, id,
	)
	return err
}

// UpdatePassword updates the user's password hash
func (r *UserRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET password_hash = ?, must_change_pass = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		passwordHash, id,
	)
	return err
}

// SetMustChangePass sets the must_change_pass flag
func (r *UserRepository) SetMustChangePass(ctx context.Context, id int64, mustChange bool) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET must_change_pass = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		mustChange, id,
	)
	return err
}
