package database_mgmt

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"easyserver/internal/dbserver"
)

// SQLiteRepository implements Repository for SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLiteRepository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

// ListDatabases returns databases for a given server, with version info joined.
func (r *SQLiteRepository) ListDatabases(ctx context.Context, dbServerID int64) ([]Database, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT d.id, v.engine_id, d.instance_id, d.name, d.charset, d.description,
		d.size_bytes, d.status, d.created_at, d.updated_at, COALESCE(v.version, '') as version
		FROM instance_databases d
		LEFT JOIN database_instances v ON d.instance_id = v.id
		WHERE v.engine_id = ? ORDER BY d.id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []Database
	for rows.Next() {
		var d Database
		if err := rows.Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name, &d.Charset, &d.Description,
			&d.SizeBytes, &d.Status, &d.CreatedAt, &d.UpdatedAt, &d.Version); err != nil {
			log.Printf("scan database row: %v", err)
			continue
		}
		dbs = append(dbs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate databases: %w", err)
	}
	return dbs, nil
}

// GetDatabase returns a database by server ID and database ID.
func (r *SQLiteRepository) GetDatabase(ctx context.Context, dbServerID, id int64) (*Database, error) {
	d := &Database{}
	err := r.db.QueryRowContext(ctx, `SELECT d.id, v.engine_id, d.instance_id, d.name FROM instance_databases d JOIN database_instances v ON d.instance_id = v.id WHERE d.id = ? AND v.engine_id = ?`,
		id, dbServerID).Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetDatabaseByID returns a database by its ID only.
func (r *SQLiteRepository) GetDatabaseByID(ctx context.Context, id int64) (*Database, error) {
	d := &Database{}
	err := r.db.QueryRowContext(ctx, `SELECT d.id, v.engine_id, d.instance_id, d.name FROM instance_databases d JOIN database_instances v ON d.instance_id = v.id WHERE d.id = ?`, id).Scan(
		&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// CreateDatabase inserts a new database record.
func (r *SQLiteRepository) CreateDatabase(ctx context.Context, dbServerID, dbVersionID int64, name, charset, description string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_databases (instance_id, name, charset, description)
		VALUES (?, ?, ?, ?)`, dbVersionID, name, charset, description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteDatabase removes a database record.
func (r *SQLiteRepository) DeleteDatabase(ctx context.Context, dbServerID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_databases WHERE id = ? AND instance_id IN (SELECT id FROM database_instances WHERE engine_id = ?)", id, dbServerID)
	return err
}

// ListDBUsers returns users for a given server.
func (r *SQLiteRepository) ListDBUsers(ctx context.Context, dbServerID int64) ([]DBUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.id, v.engine_id, u.username, u.host, u.privileges, u.created_at
		FROM instance_users u JOIN database_instances v ON u.instance_id = v.id WHERE v.engine_id = ? ORDER BY u.id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []DBUser
	for rows.Next() {
		var u DBUser
		if err := rows.Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host, &u.Privileges, &u.CreatedAt); err != nil {
			log.Printf("scan db user row: %v", err)
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate db users: %w", err)
	}
	return users, nil
}

// GetDBUser returns a user by server ID and user ID.
func (r *SQLiteRepository) GetDBUser(ctx context.Context, dbServerID, id int64) (*DBUser, error) {
	u := &DBUser{}
	err := r.db.QueryRowContext(ctx, `SELECT u.id, v.engine_id, u.username, u.host FROM instance_users u JOIN database_instances v ON u.instance_id = v.id WHERE u.id = ? AND v.engine_id = ?`,
		id, dbServerID).Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

// CreateDBUser inserts a new database user record.
func (r *SQLiteRepository) CreateDBUser(ctx context.Context, dbServerID int64, username, hashedPassword, host string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_users (instance_id, username, password, host) VALUES (?, ?, ?, ?)`,
		dbServerID, username, hashedPassword, host)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteDBUser removes a database user record.
func (r *SQLiteRepository) DeleteDBUser(ctx context.Context, dbServerID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_users WHERE id = ? AND instance_id IN (SELECT id FROM database_instances WHERE engine_id = ?)", id, dbServerID)
	return err
}

// UpdateDBUserPrivileges updates the privileges string for a user.
func (r *SQLiteRepository) UpdateDBUserPrivileges(ctx context.Context, id int64, privileges string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE instance_users SET privileges = ? WHERE id = ?", privileges, id)
	return err
}

// GetServer returns a lightweight server lookup by ID.
func (r *SQLiteRepository) GetServer(ctx context.Context, id int64) (*dbserver.DBServer, error) {
	ds := &dbserver.DBServer{}
	err := r.db.QueryRowContext(ctx, `SELECT id, name, display_name, status FROM database_engines WHERE id = ?`, id).Scan(
		&ds.ID, &ds.Name, &ds.DisplayName, &ds.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ds, err
}

// GetVersion returns a lightweight version lookup by ID.
func (r *SQLiteRepository) GetVersion(ctx context.Context, id int64) (*dbserver.DBVersion, error) {
	v := &dbserver.DBVersion{}
	err := r.db.QueryRowContext(ctx, `SELECT id, engine_id, version, container_name, port, status, runtime, image, container_id, volume_name, bind_address, admin_user, admin_password, health_status FROM database_instances WHERE id = ?`, id).Scan(
		&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status, &v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName, &v.BindAddress, &v.AdminUser, &v.AdminPassword, &v.HealthStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

// ListVersions returns versions for a server (lightweight).
func (r *SQLiteRepository) ListVersions(ctx context.Context, dbServerID int64) ([]dbserver.DBVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, engine_id, version, container_name, port, status, runtime, image, container_id, volume_name, bind_address, admin_user, admin_password, health_status FROM database_instances WHERE engine_id = ?`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []dbserver.DBVersion
	for rows.Next() {
		var v dbserver.DBVersion
		if err := rows.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status, &v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName, &v.BindAddress, &v.AdminUser, &v.AdminPassword, &v.HealthStatus); err != nil {
			log.Printf("scan version row: %v", err)
			continue
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return versions, nil
}

// CreateBackup inserts a new backup record and returns its ID.
func (r *SQLiteRepository) CreateBackup(ctx context.Context, backup *DBBackup) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO instance_backups (instance_id, database_id, database_name, backup_type, file_path, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		backup.DBVersionID, backup.DatabaseID, backup.DatabaseName, backup.BackupType, backup.FilePath, backup.Status)
	if err != nil {
		return 0, fmt.Errorf("insert backup record: %w", err)
	}
	return result.LastInsertId()
}

// UpdateBackupStatus updates the status of a backup record.
func (r *SQLiteRepository) UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE instance_backups SET status = ?, file_size = ?, error_message = ? WHERE id = ?",
		status, fileSize, errorMessage, id)
	return err
}

// ListBackups returns all backups for a database.
func (r *SQLiteRepository) ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT b.id, v.engine_id, b.instance_id, b.database_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.database_id = ? ORDER BY b.created_at DESC`, databaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []DBBackup
	for rows.Next() {
		var b DBBackup
		if err := rows.Scan(&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName,
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

// GetBackup returns a backup by ID.
func (r *SQLiteRepository) GetBackup(ctx context.Context, id int64) (*DBBackup, error) {
	b := &DBBackup{}
	err := r.db.QueryRowContext(ctx,
		`SELECT b.id, v.engine_id, b.instance_id, b.database_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.id = ?`, id).Scan(
		&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName,
		&b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

// DeleteBackup deletes a backup record.
func (r *SQLiteRepository) DeleteBackup(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_backups WHERE id = ?", id)
	return err
}
