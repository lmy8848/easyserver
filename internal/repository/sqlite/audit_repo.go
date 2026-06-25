package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// AuditRepository implements repository.AuditRepository for SQLite
type AuditRepository struct {
	db *sql.DB
}

// NewAuditRepository creates a new AuditRepository
func NewAuditRepository(db *sql.DB) repository.AuditRepository {
	return &AuditRepository{db: db}
}

// Log creates a new audit log entry
func (r *AuditRepository) Log(ctx context.Context, entry *model.AuditLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.Username, entry.Action, entry.Resource,
		entry.Detail, entry.IP, entry.UserAgent,
	)
	return err
}

// Query returns audit logs matching the filter
func (r *AuditRepository) Query(ctx context.Context, filter repository.AuditFilter) (int64, []model.AuditLog, error) {
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

	// Get total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs WHERE %s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return 0, nil, err
	}

	// Get logs
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

	var logs []model.AuditLog
	for rows.Next() {
		var log model.AuditLog
		if err := rows.Scan(
			&log.ID, &log.UserID, &log.Username, &log.Action,
			&log.Resource, &log.Detail, &log.IP, &log.UserAgent, &log.CreatedAt,
		); err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return total, logs, nil
}

// GetActions returns distinct action types
func (r *AuditRepository) GetActions(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT action FROM audit_logs ORDER BY action")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			continue
		}
		actions = append(actions, action)
	}
	return actions, nil
}

// Clean deletes audit logs older than the specified time
func (r *AuditRepository) Clean(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM audit_logs WHERE created_at < ?", before)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
