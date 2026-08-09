package dbserver

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// ListServers returns all database servers
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

// GetServer returns a database server by ID
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

// SeedServer inserts a predefined server entry if it doesn't already exist
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

// ListVersions returns all versions for a database server
func (r *sqliteRepo) ListVersions(ctx context.Context, dbServerID int64) ([]DBVersion, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, engine_id, version, container_name, config_dir, '', port, status, created_at,
		runtime, image, container_id, volume_name, bind_address, admin_user, admin_password, health_status
		FROM database_instances WHERE engine_id = ? ORDER BY id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []DBVersion
	for rows.Next() {
		var v DBVersion
		if err := rows.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ServiceName,
			&v.ConfigFile, &v.DataDir, &v.Port, &v.Status, &v.CreatedAt,
			&v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName, &v.BindAddress, &v.AdminUser, &v.AdminPassword, &v.HealthStatus); err != nil {
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

// GetVersion returns a version by ID
func (r *sqliteRepo) GetVersion(ctx context.Context, id int64) (*DBVersion, error) {
	v := &DBVersion{}
	err := r.db.QueryRowContext(ctx, `SELECT id, engine_id, version, container_name, config_dir, '', port, status, created_at,
		runtime, image, container_id, volume_name, bind_address, admin_user, admin_password, health_status
		FROM database_instances WHERE id = ?`, id).Scan(
		&v.ID, &v.DBServerID, &v.Version, &v.ServiceName,
		&v.ConfigFile, &v.DataDir, &v.Port, &v.Status, &v.CreatedAt,
		&v.Runtime, &v.Image, &v.ContainerID, &v.VolumeName, &v.BindAddress, &v.AdminUser, &v.AdminPassword, &v.HealthStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// CountVersionsByServerAndVersion counts versions for a given server and version string
func (r *sqliteRepo) CountVersionsByServerAndVersion(ctx context.Context, dbServerID int64, version string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM database_instances WHERE engine_id = ? AND version = ?",
		dbServerID, version).Scan(&count)
	return count, err
}

// CreateVersion inserts a new version record
func (r *sqliteRepo) CreateVersion(ctx context.Context, dbServerID int64, version, serviceName string, port int, status string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO database_instances (engine_id, version, container_name, port, status)
		VALUES (?, ?, ?, ?, ?)`, dbServerID, version, serviceName, port, status)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CreateContainerVersion inserts all metadata for a new managed database container.
func (r *sqliteRepo) CreateContainerVersion(ctx context.Context, v *DBVersion) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO database_instances
		(engine_id, version, runtime, image, container_name, container_id, volume_name, bind_address, port, admin_user, admin_password, status, health_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.DBServerID, v.Version, v.Runtime, v.Image, v.ServiceName, v.ContainerID, v.VolumeName,
		v.BindAddress, v.Port, v.AdminUser, v.AdminPassword, v.Status, v.HealthStatus)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteVersion removes a version record
func (r *sqliteRepo) DeleteVersion(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM database_instances WHERE id = ?", id)
	return err
}

// CountDatabasesByVersion counts databases for a given version
func (r *sqliteRepo) CountDatabasesByVersion(ctx context.Context, versionID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instance_databases WHERE instance_id = ?", versionID).Scan(&count)
	return count, err
}

// UpdateVersionStatus updates the status of a version
func (r *sqliteRepo) UpdateVersionStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET status = ?, health_status = ? WHERE id = ?", status, status, id)
	return err
}

// UpdateVersionPort updates the port of a version
func (r *sqliteRepo) UpdateVersionPort(ctx context.Context, id int64, port int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET port = ? WHERE id = ?", port, id)
	return err
}

func (r *sqliteRepo) UpdateVersionPassword(ctx context.Context, id int64, encryptedPassword string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_instances SET admin_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", encryptedPassword, id)
	return err
}

// UpdateServerStatus updates the status and version summary of a server
func (r *sqliteRepo) UpdateServerStatus(ctx context.Context, id int64, status, versionSummary string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE database_engines SET status = ?, version = ? WHERE id = ?", status, versionSummary, id)
	return err
}
