package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// sqliteRepo implements Repository for SQLite.
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed Repository.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// --- Instances ---

// instanceColumns lists database_instances columns in the exact order scanInstance
// expects. Keeping it in one place avoids query/scan drift.
const instanceColumns = `id, db_type, version, container_engine, image, container_id, volume_name, config_dir, bind_address, port, admin_password, status, created_at`

// scanInstance scans one database_instance row/query result into a DBInstance.
// It works for both *sql.Row and *sql.Rows.
func scanInstance(row interface{ Scan(...any) error }) (DBInstance, error) {
	var v DBInstance
	err := row.Scan(&v.ID, &v.DBType, &v.Version, &v.ContainerEngine, &v.Image, &v.ContainerName, &v.VolumeName,
		&v.ConfigDir, &v.BindAddress, &v.Port, &v.AdminPassword, &v.Status, &v.CreatedAt)
	return v, err
}

func (r *sqliteRepo) ListInstances(ctx context.Context, dbType DBType) ([]DBInstance, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+instanceColumns+`
		FROM database_instances WHERE db_type = ? ORDER BY id`, dbType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []DBInstance
	for rows.Next() {
		v, err := scanInstance(rows)
		if err != nil {
			log.Printf("scan instance row: %v", err)
			continue
		}
		instances = append(instances, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return instances, nil
}

func (r *sqliteRepo) GetInstance(ctx context.Context, id int64) (*DBInstance, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+instanceColumns+`
		FROM database_instances WHERE id = ?`, id)
	v, err := scanInstance(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *sqliteRepo) CountInstancesByDBTypeAndVersion(ctx context.Context, dbType DBType, version string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_instances WHERE db_type = ? AND version = ?",
		dbType, version).Scan(&count)
	return count, err
}

func (r *sqliteRepo) CreateInstance(ctx context.Context, v *DBInstance) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO database_instances
		(db_type, version, container_engine, image, container_id, volume_name, config_dir, bind_address, port, admin_password, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.DBType, v.Version, v.ContainerEngine, v.Image, v.ContainerName, v.VolumeName,
		v.ConfigDir, v.BindAddress, v.Port, v.AdminPassword, v.Status)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteInstance(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM database_instances WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) UpdateInstanceStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET status = ? WHERE id = ?", status, id)
	return err
}

func (r *sqliteRepo) UpdateInstancePort(ctx context.Context, id int64, port int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET port = ? WHERE id = ?", port, id)
	return err
}

func (r *sqliteRepo) UpdateInstancePassword(ctx context.Context, id int64, password string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET admin_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", password, id)
	return err
}

// --- Backups ---

func (r *sqliteRepo) CreateBackup(ctx context.Context, backup *DBBackup) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO instance_backups (instance_id, database_name, backup_type, file_path, status)
		VALUES (?, ?, ?, ?, ?)`,
		backup.DBInstanceID, backup.DatabaseName, backup.BackupType, backup.FilePath, backup.Status)
	if err != nil {
		return 0, fmt.Errorf("insert backup record: %w", err)
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE instance_backups SET status = ?, file_size = ?, error_message = ? WHERE id = ?",
		status, fileSize, errorMessage, id)
	return err
}

func (r *sqliteRepo) ListBackups(ctx context.Context, instanceID int64, databaseName string) ([]DBBackup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT b.id, v.db_type, b.instance_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id
		WHERE b.instance_id = ? AND b.database_name = ? ORDER BY b.created_at DESC`, instanceID, databaseName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []DBBackup
	for rows.Next() {
		var b DBBackup
		if err := rows.Scan(&b.ID, &b.DBType, &b.DBInstanceID, &b.DatabaseName,
			&b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt); err != nil {
			log.Printf("scan backup row: %v", err)
			continue
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backups: %w", err)
	}
	return backups, nil
}

func (r *sqliteRepo) ListAllBackups(ctx context.Context) ([]DBBackup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT b.id, v.db_type, b.instance_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id
		ORDER BY b.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []DBBackup
	for rows.Next() {
		var b DBBackup
		if err := rows.Scan(&b.ID, &b.DBType, &b.DBInstanceID, &b.DatabaseName,
			&b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt); err != nil {
			log.Printf("scan backup row: %v", err)
			continue
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backups: %w", err)
	}
	return backups, nil
}

func (r *sqliteRepo) GetBackup(ctx context.Context, id int64) (*DBBackup, error) {
	b := &DBBackup{}
	err := r.db.QueryRowContext(ctx,
		`SELECT b.id, v.db_type, b.instance_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.id = ?`, id).Scan(
		&b.ID, &b.DBType, &b.DBInstanceID, &b.DatabaseName,
		&b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

func (r *sqliteRepo) DeleteBackup(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_backups WHERE id = ?", id)
	return err
}
