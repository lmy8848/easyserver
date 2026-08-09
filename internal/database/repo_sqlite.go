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

// --- Engine catalog ---

func (r *sqliteRepo) ListServers(ctx context.Context) ([]DBServer, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, display_name, description, default_port, status, version, created_at
		FROM database_engines ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []DBServer
	for rows.Next() {
		var ds DBServer
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.DisplayName, &ds.Description,
			&ds.DefaultPort, &ds.Status, &ds.Version, &ds.CreatedAt); err != nil {
			log.Printf("scan db server row: %v", err)
			continue
		}
		servers = append(servers, ds)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate db servers: %w", err)
	}
	return servers, nil
}

func (r *sqliteRepo) GetServer(ctx context.Context, id int64) (*DBServer, error) {
	ds := &DBServer{}
	err := r.db.QueryRowContext(ctx, `SELECT id, name, display_name, description, default_port, status, version, created_at
		FROM database_engines WHERE id = ?`, id).Scan(
		&ds.ID, &ds.Name, &ds.DisplayName, &ds.Description,
		&ds.DefaultPort, &ds.Status, &ds.Version, &ds.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ds, nil
}

func (r *sqliteRepo) SeedServer(ctx context.Context, name, displayName, description string, defaultPort int) error {
	var count int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_engines WHERE name = ?", name).Scan(&count)
	if count == 0 {
		_, err := r.db.ExecContext(ctx, `INSERT INTO database_engines (name, display_name, description, default_port)
			VALUES (?, ?, ?, ?)`, name, displayName, description, defaultPort)
		return err
	}
	return nil
}

// --- Instances ---

// instanceColumns lists database_instances columns in the exact order scanInstance
// expects. Keeping it in one place avoids the query/scan drift that duplicated
// SELECTs caused.
const instanceColumns = `id, engine_id, version, container_name, config_dir, bind_address, port, admin_user, admin_password, status, health_status, created_at, runtime, image, container_id, volume_name`

// scanInstance scans one database_instance row/query result into a DBInstance.
// It works for both *sql.Row and *sql.Rows.
func scanInstance(row interface{ Scan(...any) error }) (DBInstance, error) {
	var v DBInstance
	err := row.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ContainerName, &v.ConfigDir,
		&v.BindAddress, &v.Port, &v.AdminUser, &v.AdminPassword, &v.Status, &v.HealthStatus,
		&v.CreatedAt, &v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName)
	return v, err
}

func (r *sqliteRepo) ListVersions(ctx context.Context, dbServerID int64) ([]DBInstance, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+instanceColumns+`
		FROM database_instances WHERE engine_id = ? ORDER BY id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []DBInstance
	for rows.Next() {
		v, err := scanInstance(rows)
		if err != nil {
			log.Printf("scan instance row: %v", err)
			continue
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances: %w", err)
	}
	return versions, nil
}

func (r *sqliteRepo) GetVersion(ctx context.Context, id int64) (*DBInstance, error) {
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

func (r *sqliteRepo) CountVersionsByServerAndVersion(ctx context.Context, dbServerID int64, version string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_instances WHERE engine_id = ? AND version = ?",
		dbServerID, version).Scan(&count)
	return count, err
}

func (r *sqliteRepo) CreateVersion(ctx context.Context, dbServerID int64, version, containerName string, port int, status string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO database_instances (engine_id, version, container_name, port, status)
		VALUES (?, ?, ?, ?, ?)`, dbServerID, version, containerName, port, status)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) CreateContainerVersion(ctx context.Context, v *DBInstance) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO database_instances
		(engine_id, version, runtime, image, container_name, container_id, volume_name, config_dir, bind_address, port, admin_user, admin_password, status, health_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.DBServerID, v.Version, v.Runtime, v.Image, v.ContainerName, v.ContainerID, v.VolumeName,
		v.ConfigDir, v.BindAddress, v.Port, v.AdminUser, v.AdminPassword, v.Status, v.HealthStatus)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteVersion(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM database_instances WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) CountDatabasesByVersion(ctx context.Context, versionID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instance_databases WHERE instance_id = ?", versionID).Scan(&count)
	return count, err
}

func (r *sqliteRepo) UpdateVersionStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET status = ?, health_status = ? WHERE id = ?", status, status, id)
	return err
}

func (r *sqliteRepo) UpdateVersionPort(ctx context.Context, id int64, port int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET port = ? WHERE id = ?", port, id)
	return err
}

func (r *sqliteRepo) UpdateVersionPassword(ctx context.Context, id int64, encryptedPassword string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET admin_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", encryptedPassword, id)
	return err
}

func (r *sqliteRepo) UpdateServerStatus(ctx context.Context, id int64, status, versionSummary string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_engines SET status = ?, version = ? WHERE id = ?", status, versionSummary, id)
	return err
}

// --- Logical databases ---

func (r *sqliteRepo) ListDatabases(ctx context.Context, dbServerID int64) ([]Database, error) {
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
		if err := rows.Scan(&d.ID, &d.DBServerID, &d.DBInstanceID, &d.Name, &d.Charset, &d.Description,
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

func (r *sqliteRepo) GetDatabase(ctx context.Context, dbServerID, id int64) (*Database, error) {
	d := &Database{}
	err := r.db.QueryRowContext(ctx, `SELECT d.id, v.engine_id, d.instance_id, d.name FROM instance_databases d JOIN database_instances v ON d.instance_id = v.id WHERE d.id = ? AND v.engine_id = ?`,
		id, dbServerID).Scan(&d.ID, &d.DBServerID, &d.DBInstanceID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (r *sqliteRepo) GetDatabaseByID(ctx context.Context, id int64) (*Database, error) {
	d := &Database{}
	err := r.db.QueryRowContext(ctx, `SELECT d.id, v.engine_id, d.instance_id, d.name FROM instance_databases d JOIN database_instances v ON d.instance_id = v.id WHERE d.id = ?`, id).Scan(
		&d.ID, &d.DBServerID, &d.DBInstanceID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (r *sqliteRepo) CreateDatabase(ctx context.Context, dbServerID, dbVersionID int64, name, charset, description string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_databases (instance_id, name, charset, description)
		VALUES (?, ?, ?, ?)`, dbVersionID, name, charset, description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteDatabase(ctx context.Context, dbServerID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_databases WHERE id = ? AND instance_id IN (SELECT id FROM database_instances WHERE engine_id = ?)", id, dbServerID)
	return err
}

// --- DB users ---

func (r *sqliteRepo) ListDBUsers(ctx context.Context, dbServerID int64) ([]DBUser, error) {
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

func (r *sqliteRepo) GetDBUser(ctx context.Context, dbServerID, id int64) (*DBUser, error) {
	u := &DBUser{}
	err := r.db.QueryRowContext(ctx, `SELECT u.id, v.engine_id, u.username, u.host FROM instance_users u JOIN database_instances v ON u.instance_id = v.id WHERE u.id = ? AND v.engine_id = ?`,
		id, dbServerID).Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *sqliteRepo) CreateDBUser(ctx context.Context, dbServerID int64, username, hashedPassword, host string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_users (instance_id, username, password, host) VALUES (?, ?, ?, ?)`,
		dbServerID, username, hashedPassword, host)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteDBUser(ctx context.Context, dbServerID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_users WHERE id = ? AND instance_id IN (SELECT id FROM database_instances WHERE engine_id = ?)", id, dbServerID)
	return err
}

func (r *sqliteRepo) UpdateDBUserPrivileges(ctx context.Context, id int64, privileges string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE instance_users SET privileges = ? WHERE id = ?", privileges, id)
	return err
}

// --- Backups ---

func (r *sqliteRepo) CreateBackup(ctx context.Context, backup *DBBackup) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO instance_backups (instance_id, database_id, database_name, backup_type, file_path, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		backup.DBInstanceID, backup.DatabaseID, backup.DatabaseName, backup.BackupType, backup.FilePath, backup.Status)
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

func (r *sqliteRepo) ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error) {
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
		if err := rows.Scan(&b.ID, &b.DBServerID, &b.DBInstanceID, &b.DatabaseID, &b.DatabaseName,
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
		`SELECT b.id, v.engine_id, b.instance_id, b.database_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.id = ?`, id).Scan(
		&b.ID, &b.DBServerID, &b.DBInstanceID, &b.DatabaseID, &b.DatabaseName,
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
