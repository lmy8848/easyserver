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
		`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, type)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.Username, entry.Action, entry.Resource,
		entry.Detail, entry.IP, entry.UserAgent, entry.Type,
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
	if filter.Type != "" {
		where += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.StartDate != "" {
		where += " AND created_at >= ?"
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		where += " AND created_at <= ?"
		args = append(args, filter.EndDate+" 23:59:59")
	}
	if filter.Status != "" {
		if filter.Type == "request" {
			switch filter.Status {
			case "2xx":
				where += " AND CAST(json_extract(detail, '$.status') AS INTEGER) BETWEEN 200 AND 299"
			case "4xx":
				where += " AND CAST(json_extract(detail, '$.status') AS INTEGER) BETWEEN 400 AND 499"
			case "5xx":
				where += " AND CAST(json_extract(detail, '$.status') AS INTEGER) >= 500"
			}
		} else if filter.Type == "operation" {
			switch filter.Status {
			case "success":
				where += " AND (CAST(json_extract(detail, '$.status') AS INTEGER) < 400 OR json_extract(detail, '$.success') = 1 OR (json_extract(detail, '$.status') IS NULL AND json_extract(detail, '$.success') IS NULL))"
			case "failed":
				where += " AND (CAST(json_extract(detail, '$.status') AS INTEGER) >= 400 OR json_extract(detail, '$.success') = 0)"
			}
		}
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs WHERE %s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return 0, nil, err
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, username, action, resource, detail, ip, user_agent, type, created_at
		 FROM audit_logs WHERE %s ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
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
			&l.Resource, &l.Detail, &l.IP, &l.UserAgent, &l.Type, &l.CreatedAt,
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

func (r *sqliteRepo) GetActions(ctx context.Context, logType string) ([]string, error) {
	var rows *sql.Rows
	var err error
	if logType != "" {
		rows, err = r.db.QueryContext(ctx, "SELECT DISTINCT action FROM audit_logs WHERE type = ? ORDER BY action", logType)
	} else {
		rows, err = r.db.QueryContext(ctx, "SELECT DISTINCT action FROM audit_logs ORDER BY action")
	}
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

func (r *sqliteRepo) AppendBatch(ctx context.Context, entries []AuditLog) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, e.UserID, e.Username, e.Action, e.Resource, e.Detail, e.IP, e.UserAgent, e.Type, e.CreatedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *sqliteRepo) Clean(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM audit_logs WHERE created_at < ?", before)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
