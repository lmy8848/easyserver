package web

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteWebsiteRepo implements WebsiteRepository for SQLite.
type sqliteWebsiteRepo struct {
	db *sql.DB
}

// NewSQLiteWebsiteRepository creates a new SQLite-backed WebsiteRepository.
func NewSQLiteWebsiteRepository(db *sql.DB) WebsiteRepository {
	return &sqliteWebsiteRepo{db: db}
}

// List returns websites for a specific web server
func (r *sqliteWebsiteRepo) List(ctx context.Context, webServerID int64) ([]Website, error) {
			rows, err := r.db.QueryContext(ctx, `SELECT id, web_server_id, name, domain, root_path, port,
				project_type, app_port, ssl_enabled, ssl_cert_path, ssl_key_path, proxy_enabled, proxy_pass,
				custom_config, config_options, process_id, build_command, start_command,
				runtime_version_id, access_log, error_log, status, created_at, updated_at
				FROM websites WHERE web_server_id = ? ORDER BY id DESC`, webServerID)
		if err != nil {
			return nil, fmt.Errorf("list websites: %w", err)
		}
		defer rows.Close()

		var sites []Website
		for rows.Next() {
			var w Website
				if err := rows.Scan(&w.ID, &w.WebServerID, &w.Name, &w.Domain, &w.RootPath, &w.Port,
					&w.ProjectType, &w.AppPort, &w.SSLEnabled, &w.SSLCertPath, &w.SSLKeyPath, &w.ProxyEnabled, &w.ProxyPass,
					&w.CustomConfig, &w.ConfigOptions, &w.ProcessID, &w.BuildCommand, &w.StartCommand,
					&w.RuntimeVersionID, &w.AccessLog, &w.ErrorLog, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
					continue
				}
			sites = append(sites, w)
		}
	return sites, nil
}

// Get returns a specific website by id and web server id
func (r *sqliteWebsiteRepo) Get(ctx context.Context, webServerID, id int64) (*Website, error) {
	w := &Website{}
			err := r.db.QueryRowContext(ctx, `SELECT id, web_server_id, name, domain, root_path, port,
				project_type, app_port, ssl_enabled, ssl_cert_path, ssl_key_path, proxy_enabled, proxy_pass,
				custom_config, config_options, process_id, build_command, start_command,
				runtime_version_id, access_log, error_log, status, created_at, updated_at
				FROM websites WHERE id = ? AND web_server_id = ?`, id, webServerID).Scan(
				&w.ID, &w.WebServerID, &w.Name, &w.Domain, &w.RootPath, &w.Port,
				&w.ProjectType, &w.AppPort, &w.SSLEnabled, &w.SSLCertPath, &w.SSLKeyPath, &w.ProxyEnabled, &w.ProxyPass,
				&w.CustomConfig, &w.ConfigOptions, &w.ProcessID, &w.BuildCommand, &w.StartCommand,
				&w.RuntimeVersionID, &w.AccessLog, &w.ErrorLog, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get website %d: %w", id, err)
	}
	return w, nil
}

// Create inserts a new website and returns its id
func (r *sqliteWebsiteRepo) Create(ctx context.Context, w *Website) (int64, error) {
		result, err := r.db.ExecContext(ctx, `INSERT INTO websites
				(web_server_id, name, domain, root_path, port, project_type, app_port,
				proxy_enabled, proxy_pass, custom_config, config_options, process_id,
				build_command, start_command, runtime_version_id, access_log, error_log)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				w.WebServerID, w.Name, w.Domain, w.RootPath, w.Port, w.ProjectType, w.AppPort,
				w.ProxyEnabled, w.ProxyPass, w.CustomConfig, w.ConfigOptions, w.ProcessID,
				w.BuildCommand, w.StartCommand, w.RuntimeVersionID, w.AccessLog, w.ErrorLog)
	if err != nil {
		return 0, fmt.Errorf("create website: %w", err)
	}
	id, _ := result.LastInsertId()
	return id, nil
}

// Update updates all mutable fields of a website
func (r *sqliteWebsiteRepo) Update(ctx context.Context, w *Website) error {
		_, err := r.db.ExecContext(ctx, `UPDATE websites SET
				name = ?, domain = ?, root_path = ?, port = ?, project_type = ?, app_port = ?,
				proxy_enabled = ?, proxy_pass = ?, custom_config = ?, config_options = ?,
				process_id = ?, build_command = ?, start_command = ?, runtime_version_id = ?,
				updated_at = datetime('now')
				WHERE id = ? AND web_server_id = ?`,
				w.Name, w.Domain, w.RootPath, w.Port, w.ProjectType, w.AppPort,
				w.ProxyEnabled, w.ProxyPass, w.CustomConfig, w.ConfigOptions, w.ProcessID,
				w.BuildCommand, w.StartCommand, w.RuntimeVersionID, w.ID, w.WebServerID)
	return err
}

// Delete removes a website by id and web server id
func (r *sqliteWebsiteRepo) Delete(ctx context.Context, webServerID, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM websites WHERE id = ? AND web_server_id = ?", id, webServerID)
	return err
}

// UpdateStatus updates the status of a website
func (r *sqliteWebsiteRepo) UpdateStatus(ctx context.Context, webServerID, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE websites SET status = ?, updated_at = datetime('now') WHERE id = ? AND web_server_id = ?",
		status, id, webServerID)
	return err
}

// UpdateSSL updates SSL certificate paths for a website
func (r *sqliteWebsiteRepo) UpdateSSL(ctx context.Context, id int64, certPath, keyPath string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE websites SET ssl_enabled = 1, ssl_cert_path = ?, ssl_key_path = ?, updated_at = datetime('now') WHERE id = ?",
		certPath, keyPath, id)
	return err
}

// CountByPort returns the number of websites using a given port on a web server
func (r *sqliteWebsiteRepo) CountByPort(ctx context.Context, webServerID int64, port int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE web_server_id = ? AND port = ?", webServerID, port).Scan(&count)
	return count, err
}

// CountByPortExcludingID returns the number of websites using a given port, excluding a specific id
func (r *sqliteWebsiteRepo) CountByPortExcludingID(ctx context.Context, webServerID int64, port int, excludeID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE web_server_id = ? AND port = ? AND id != ?", webServerID, port, excludeID).Scan(&count)
	return count, err
}

// UpdateProcessID updates the linked process ID for a website
func (r *sqliteWebsiteRepo) UpdateProcessID(ctx context.Context, id int64, processID int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE websites SET process_id = ?, updated_at = datetime('now') WHERE id = ?", processID, id)
	return err
}

// CountByDomain returns the number of websites with a given domain
func (r *sqliteWebsiteRepo) CountByDomain(ctx context.Context, domain string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE domain = ?", domain).Scan(&count)
	return count, err
}

// CountByDomainExcludingID returns the number of websites with a given domain, excluding a specific id
func (r *sqliteWebsiteRepo) CountByDomainExcludingID(ctx context.Context, domain string, excludeID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE domain = ? AND id != ?", domain, excludeID).Scan(&count)
	return count, err
}
