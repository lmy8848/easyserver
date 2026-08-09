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
const instanceColumns = `id, db_type, version, runtime, image, container_id, volume_name, config_dir, bind_address, port, admin_password, status, created_at`

// scanInstance scans one database_instance row/query result into a DBInstance.
// It works for both *sql.Row and *sql.Rows.
func scanInstance(row interface{ Scan(...any) error }) (DBInstance, error) {
	var v DBInstance
	err := row.Scan(&v.ID, &v.DBType, &v.Version, &v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName,
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

func (r *sqliteRepo) ListAllInstances(ctx context.Context) ([]DBInstance, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+instanceColumns+` FROM database_instances ORDER BY id`)
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
		(db_type, version, runtime, image, container_id, volume_name, config_dir, bind_address, port, admin_password, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.DBType, v.Version, v.Runtime, v.Image, v.ContainerID, v.VolumeName,
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

func (r *sqliteRepo) CountDatabasesByInstance(ctx context.Context, instanceID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instance_databases WHERE instance_id = ?", instanceID).Scan(&count)
	return count, err
}

func (r *sqliteRepo) UpdateInstanceStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET status = ?, health_status = ? WHERE id = ?", status, status, id)
	return err
}

func (r *sqliteRepo) UpdateInstancePort(ctx context.Context, id int64, port int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET port = ? WHERE id = ?", port, id)
	return err
}

func (r *sqliteRepo) UpdateInstancePassword(ctx context.Context, id int64, encryptedPassword string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET admin_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", encryptedPassword, id)
	return err
}

// --- Logical databases ---

func (r *sqliteRepo) ListDatabases(ctx context.Context, instanceID int64) ([]Database, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT d.id, v.db_type, d.instance_id, d.name, d.charset, d.description,
		d.size_bytes, d.status, d.created_at, d.updated_at, COALESCE(v.version, '') as version
		FROM instance_databases d
		LEFT JOIN database_instances v ON d.instance_id = v.id
		WHERE d.instance_id = ? ORDER BY d.id`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []Database
	for rows.Next() {
		var d Database
		if err := rows.Scan(&d.ID, &d.DBType, &d.DBInstanceID, &d.Name, &d.Charset, &d.Description,
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

func (r *sqliteRepo) GetDatabaseByID(ctx context.Context, id int64) (*Database, error) {
	d := &Database{}
	err := r.db.QueryRowContext(ctx, `SELECT d.id, v.db_type, d.instance_id, d.name FROM instance_databases d JOIN database_instances v ON d.instance_id = v.id WHERE d.id = ?`, id).Scan(
		&d.ID, &d.DBType, &d.DBInstanceID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func (r *sqliteRepo) CreateDatabase(ctx context.Context, instanceID int64, name, charset, description string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_databases (instance_id, name, charset, description)
		VALUES (?, ?, ?, ?)`, instanceID, name, charset, description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteDatabase(ctx context.Context, instanceID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_databases WHERE id = ? AND instance_id = ?", id, instanceID)
	return err
}

// --- DB users ---

func (r *sqliteRepo) ListDBUsers(ctx context.Context, instanceID int64) ([]DBUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.id, v.db_type, u.username, u.host, u.privileges, u.created_at
		FROM instance_users u JOIN database_instances v ON u.instance_id = v.id WHERE u.instance_id = ? ORDER BY u.id`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []DBUser
	for rows.Next() {
		var u DBUser
		if err := rows.Scan(&u.ID, &u.DBType, &u.Username, &u.Host, &u.Privileges, &u.CreatedAt); err != nil {
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

func (r *sqliteRepo) GetDBUser(ctx context.Context, instanceID, id int64) (*DBUser, error) {
	u := &DBUser{}
	err := r.db.QueryRowContext(ctx, `SELECT u.id, v.db_type, u.username, u.host FROM instance_users u JOIN database_instances v ON u.instance_id = v.id WHERE u.id = ? AND u.instance_id = ?`,
		id, instanceID).Scan(&u.ID, &u.DBType, &u.Username, &u.Host)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (r *sqliteRepo) CreateDBUser(ctx context.Context, instanceID int64, username, hashedPassword, host string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO instance_users (instance_id, username, password, host) VALUES (?, ?, ?, ?)`,
		instanceID, username, hashedPassword, host)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *sqliteRepo) DeleteDBUser(ctx context.Context, instanceID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instance_users WHERE id = ? AND instance_id = ?", id, instanceID)
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
		`SELECT b.id, v.db_type, b.instance_id, b.database_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.database_id = ? ORDER BY b.created_at DESC`, databaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []DBBackup
	for rows.Next() {
		var b DBBackup
		if err := rows.Scan(&b.ID, &b.DBType, &b.DBInstanceID, &b.DatabaseID, &b.DatabaseName,
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
		`SELECT b.id, v.db_type, b.instance_id, b.database_id, b.database_name, b.backup_type, b.file_path, b.file_size, b.status, b.error_message, b.created_at
		FROM instance_backups b JOIN database_instances v ON b.instance_id = v.id WHERE b.id = ?`, id).Scan(
		&b.ID, &b.DBType, &b.DBInstanceID, &b.DatabaseID, &b.DatabaseName,
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
