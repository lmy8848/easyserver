package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// DBBackupRepository implements repository.DBBackupRepository for SQLite
type DBBackupRepository struct {
	db *sql.DB
}

// NewDBBackupRepository creates a new DBBackupRepository
func NewDBBackupRepository(db *sql.DB) repository.DBBackupRepository {
	return &DBBackupRepository{db: db}
}

// CreateBackup inserts a new backup record and returns its ID
func (r *DBBackupRepository) CreateBackup(ctx context.Context, backup *model.DBBackup) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO db_backups (db_server_id, db_version_id, database_id, database_name, backup_type, file_path, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		backup.DBServerID, backup.DBVersionID, backup.DatabaseID, backup.DatabaseName, backup.BackupType, backup.FilePath, backup.Status)
	if err != nil {
		return 0, fmt.Errorf("insert backup record: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// UpdateBackupStatus updates the status, file_size, and error_message of a backup
func (r *DBBackupRepository) UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE db_backups SET status = ?, file_size = ?, error_message = ? WHERE id = ?`,
		status, fileSize, errorMessage, id)
	if err != nil {
		return fmt.Errorf("update backup status: %w", err)
	}
	return nil
}

// ListBackups returns all backups for a given database, ordered by creation date descending
func (r *DBBackupRepository) ListBackups(ctx context.Context, databaseID int64) ([]model.DBBackup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, db_server_id, db_version_id, database_id, database_name, backup_type, file_path, file_size, status, error_message, created_at
		FROM db_backups WHERE database_id = ? ORDER BY created_at DESC`, databaseID)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []model.DBBackup
	for rows.Next() {
		var b model.DBBackup
		if err := rows.Scan(&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName, &b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt); err != nil {
			continue
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backups: %w", err)
	}
	return backups, nil
}

// GetBackup returns a backup by ID, or nil if not found
func (r *DBBackupRepository) GetBackup(ctx context.Context, id int64) (*model.DBBackup, error) {
	var b model.DBBackup
	err := r.db.QueryRowContext(ctx,
		`SELECT id, db_server_id, db_version_id, database_id, database_name, backup_type, file_path, file_size, status, error_message, created_at
		FROM db_backups WHERE id = ?`, id).Scan(&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName, &b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get backup: %w", err)
	}
	return &b, nil
}

// DeleteBackup deletes a backup record by ID
func (r *DBBackupRepository) DeleteBackup(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM db_backups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}
