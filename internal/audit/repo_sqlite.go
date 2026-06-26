package audit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// sqliteRepo implements Repository for SQLite.
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed audit Repository.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

func (r *sqliteRepo) Log(ctx context.Context, entry *AuditLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.Username, entry.Action, entry.Resource,
		entry.Detail, entry.IP, entry.UserAgent,
	)
	return err
}

func (r *sqliteRepo) Query(ctx context.Context, filter AuditFilter) (int64, []AuditLog, error) {
	where := "1=1"
	args := []interface{}{}

	if filter.Username != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+filter.Username+"%")
	}
	if filter.Action != "" {
		where += " AND action = ?"
		args = append(args, filter.Action)
	}
	if filter.Resource != "" {
		where += " AND resource LIKE ?"
		args = append(args, "%"+filter.Resource+"%")
	}
	if filter.IP != "" {
		where += " AND ip LIKE ?"
		args = append(args, "%"+filter.IP+"%")
	}
	if filter.StartDate != "" {
		where += " AND created_at >= ?"
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		where += " AND created_at <= ?"
		args = append(args, filter.EndDate+" 23:59:59")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs WHERE %s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return 0, nil, err
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, username, action, resource, detail, ip, user_agent, created_at
		 FROM audit_logs WHERE %s ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		where,
	)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(
			&l.ID, &l.UserID, &l.Username, &l.Action,
			&l.Resource, &l.Detail, &l.IP, &l.UserAgent, &l.CreatedAt,
		); err != nil {
			return 0, nil, err
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	return total, logs, nil
}

func (r *sqliteRepo) GetActions(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT action FROM audit_logs ORDER BY action")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

func (r *sqliteRepo) AppendSignedBatch(ctx context.Context, entries []SignedAuditEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at, signature)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, e.UserID, e.Username, e.Action, e.Resource, e.Detail, e.IP, e.UserAgent, e.CreatedAt, e.Signature); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *sqliteRepo) GetSignedEntry(ctx context.Context, id int64) (*SignedAuditEntry, error) {
	var e SignedAuditEntry
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, username, action, resource, detail, ip, user_agent, created_at, signature
		 FROM audit_logs WHERE id = ?`, id,
	).Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.Resource, &e.Detail, &e.IP, &e.UserAgent, &e.CreatedAt, &e.Signature)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *sqliteRepo) ListIDsForVerification(ctx context.Context, limit int) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *sqliteRepo) Clean(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM audit_logs WHERE created_at < ?", before)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
