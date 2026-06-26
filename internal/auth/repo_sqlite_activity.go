package auth

import (
	"context"
	"database/sql"
)

type sqliteActivityRepo struct {
	db *sql.DB
}

func NewSQLiteActivityRepository(db *sql.DB) ActivityRepo {
	return &sqliteActivityRepo{db: db}
}

func (r *sqliteActivityRepo) Log(ctx context.Context, entry *UserActivity) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO user_activities (user_id, username, action, ip, user_agent) VALUES (?, ?, ?, ?, ?)",
		entry.UserID, entry.Username, entry.Action, entry.IP, entry.UserAgent,
	)
	return err
}

func (r *sqliteActivityRepo) GetByUserID(ctx context.Context, userID int64, limit int) ([]UserActivity, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, user_id, username, action, ip, user_agent, created_at FROM user_activities WHERE user_id = ? ORDER BY id DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []UserActivity
	for rows.Next() {
		var a UserActivity
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			continue
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

func (r *sqliteActivityRepo) GetAll(ctx context.Context, limit int) ([]UserActivity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, user_id, username, action, ip, user_agent, created_at FROM user_activities ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []UserActivity
	for rows.Next() {
		var a UserActivity
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			continue
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}
