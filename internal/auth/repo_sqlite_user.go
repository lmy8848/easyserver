package auth

import (
	"context"
	"database/sql"
	"fmt"

	"easyserver/internal/infra/errx"
)

type sqliteUserRepo struct {
	db *sql.DB
}

func NewSQLiteUserRepository(db *sql.DB) UserRepo {
	return &sqliteUserRepo{db: db}
}

func (r *sqliteUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	user := &User{}
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
			return nil, errx.NotFound("user not found")
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

func (r *sqliteUserRepo) GetByUsername(ctx context.Context, username string) (*User, error) {
	user := &User{}
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
			return nil, errx.NotFound("user not found")
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

func (r *sqliteUserRepo) Create(ctx context.Context, user *User) error {
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

func (r *sqliteUserRepo) List(ctx context.Context, offset, limit int) ([]User, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

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

	var users []User
	for rows.Next() {
		var user User
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *sqliteUserRepo) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET password_hash = ?, must_change_pass = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		passwordHash, id,
	)
	return err
}

func (r *sqliteUserRepo) UpdateUsername(ctx context.Context, id int64, username string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		username, id,
	)
	return err
}

func (r *sqliteUserRepo) SetMustChangePass(ctx context.Context, id int64, mustChange bool) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET must_change_pass = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		mustChange, id,
	)
	return err
}

// maxAttempts<=0 时仅计数，不触发锁定。返回本次是否触发了锁定。
func (r *sqliteUserRepo) IncrementLoginAttempts(ctx context.Context, id int64, maxAttempts int, lockoutSeconds int) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET
		login_attempts = login_attempts + 1,
		locked_until = CASE
			WHEN ? > 0 AND login_attempts + 1 >= ? THEN datetime('now', ?)
			ELSE locked_until
		END
		WHERE id = ?`, maxAttempts, maxAttempts, fmt.Sprintf("+%d seconds", lockoutSeconds), id)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0 && maxAttempts > 0, nil
}

func (r *sqliteUserRepo) ResetLoginState(ctx context.Context, id int64, ip string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET login_attempts = 0, locked_until = NULL, last_login = CURRENT_TIMESTAMP, last_login_ip = ? WHERE id = ?",
		ip, id)
	return err
}
