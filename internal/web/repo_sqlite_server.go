package web

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteServerRepo implements ServerRepository for SQLite.
type sqliteServerRepo struct {
	db *sql.DB
}

// NewSQLiteServerRepository creates a new SQLite-backed ServerRepository.
func NewSQLiteServerRepository(db *sql.DB) ServerRepository {
	return &sqliteServerRepo{db: db}
}

// List returns all web servers ordered by id
func (r *sqliteServerRepo) List(ctx context.Context) ([]WebServer, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
		config_path, config_file, sites_available, sites_enabled, service_name, binary_path,
		default_port, log_dir, status, version, created_at
		FROM web_servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list web servers: %w", err)
	}
	defer rows.Close()

	var servers []WebServer
	for rows.Next() {
		var ws WebServer
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.Description,
			&ws.InstallCmd, &ws.UninstallCmd, &ws.ConfigPath, &ws.ConfigFile,
			&ws.SitesAvailable, &ws.SitesEnabled, &ws.ServiceName, &ws.BinaryPath,
			&ws.DefaultPort, &ws.LogDir, &ws.Status, &ws.Version, &ws.CreatedAt); err != nil {
			continue
		}
		servers = append(servers, ws)
	}
	return servers, nil
}

// Get returns a web server by id
func (r *sqliteServerRepo) Get(ctx context.Context, id int64) (*WebServer, error) {
	ws := &WebServer{}
	err := r.db.QueryRowContext(ctx, `SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
		config_path, config_file, sites_available, sites_enabled, service_name, binary_path,
		default_port, log_dir, status, version, created_at
		FROM web_servers WHERE id = ?`, id).Scan(
		&ws.ID, &ws.Name, &ws.DisplayName, &ws.Description,
		&ws.InstallCmd, &ws.UninstallCmd, &ws.ConfigPath, &ws.ConfigFile,
		&ws.SitesAvailable, &ws.SitesEnabled, &ws.ServiceName, &ws.BinaryPath,
		&ws.DefaultPort, &ws.LogDir, &ws.Status, &ws.Version, &ws.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get web server %d: %w", id, err)
	}
	return ws, nil
}

// Create inserts a new web server
func (r *sqliteServerRepo) Create(ctx context.Context, ws *WebServer) error {
	result, err := r.db.ExecContext(ctx, `INSERT INTO web_servers
		(name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file,
		sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ws.Name, ws.DisplayName, ws.Description, ws.InstallCmd, ws.UninstallCmd,
		ws.ConfigPath, ws.ConfigFile, ws.SitesAvailable, ws.SitesEnabled,
		ws.ServiceName, ws.BinaryPath, ws.DefaultPort, ws.LogDir)
	if err != nil {
		return fmt.Errorf("create web server: %w", err)
	}
	ws.ID, _ = result.LastInsertId()
	return nil
}

// Delete removes a web server by id
func (r *sqliteServerRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM web_servers WHERE id = ?", id)
	return err
}

// UpdateStatus updates the status of a web server
func (r *sqliteServerRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE web_servers SET status = ? WHERE id = ?", status, id)
	return err
}

// UpdateStatusAndVersion updates the status and version of a web server
func (r *sqliteServerRepo) UpdateStatusAndVersion(ctx context.Context, id int64, status, version string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE web_servers SET status = ?, version = ? WHERE id = ?", status, version, id)
	return err
}

// CountWebsitesByServerID returns the number of websites using a given web server
func (r *sqliteServerRepo) CountWebsitesByServerID(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE web_server_id = ?", serverID).Scan(&count)
	return count, err
}
