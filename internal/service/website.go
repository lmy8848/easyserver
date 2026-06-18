package service

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"easyserver/internal/model"
)

type WebsiteService struct {
	db *sql.DB
}

func NewWebsiteService(db *sql.DB) *WebsiteService {
	return &WebsiteService{db: db}
}

func (s *WebsiteService) InitTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS websites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			web_server_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			domain TEXT NOT NULL UNIQUE,
			root_path TEXT NOT NULL,
			port INTEGER DEFAULT 80,
			ssl_enabled INTEGER DEFAULT 0,
			ssl_cert_path TEXT DEFAULT '',
			ssl_key_path TEXT DEFAULT '',
			proxy_enabled INTEGER DEFAULT 0,
			proxy_pass TEXT DEFAULT '',
			custom_config TEXT DEFAULT '',
			access_log TEXT DEFAULT '',
			error_log TEXT DEFAULT '',
			status TEXT DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_websites_domain ON websites(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_websites_server ON websites(web_server_id)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	// Migration: add new columns if missing
	s.db.Exec("ALTER TABLE websites ADD COLUMN web_server_id INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE websites ADD COLUMN project_type TEXT DEFAULT 'static'")
	s.db.Exec("ALTER TABLE websites ADD COLUMN app_port INTEGER DEFAULT 0")

	return nil
}

// List returns websites for a specific web server
func (s *WebsiteService) List(webServerID int64) ([]model.Website, error) {
	rows, err := s.db.Query(`SELECT id, web_server_id, name, domain, root_path, port,
		project_type, app_port, ssl_enabled, ssl_cert_path, ssl_key_path, proxy_enabled, proxy_pass,
		custom_config, access_log, error_log, status, created_at, updated_at
		FROM websites WHERE web_server_id = ? ORDER BY id DESC`, webServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []model.Website
	for rows.Next() {
		var w model.Website
		var projectType string
		var appPort int
		err := rows.Scan(&w.ID, &w.WebServerID, &w.Name, &w.Domain, &w.RootPath, &w.Port,
			&projectType, &appPort, &w.SSLEnabled, &w.SSLCertPath, &w.SSLKeyPath, &w.ProxyEnabled, &w.ProxyPass,
			&w.CustomConfig, &w.AccessLog, &w.ErrorLog, &w.Status, &w.CreatedAt, &w.UpdatedAt)
		if err != nil {
			continue
		}
		w.ProjectType = projectType
		w.AppPort = appPort
		sites = append(sites, w)
	}
	return sites, nil
}

// Get returns a specific website
func (s *WebsiteService) Get(webServerID, id int64) (*model.Website, error) {
	w := &model.Website{}
	var projectType string
	var appPort int
	err := s.db.QueryRow(`SELECT id, web_server_id, name, domain, root_path, port,
		project_type, app_port, ssl_enabled, ssl_cert_path, ssl_key_path, proxy_enabled, proxy_pass,
		custom_config, access_log, error_log, status, created_at, updated_at
		FROM websites WHERE id = ? AND web_server_id = ?`, id, webServerID).Scan(
		&w.ID, &w.WebServerID, &w.Name, &w.Domain, &w.RootPath, &w.Port,
		&projectType, &appPort, &w.SSLEnabled, &w.SSLCertPath, &w.SSLKeyPath, &w.ProxyEnabled, &w.ProxyPass,
		&w.CustomConfig, &w.AccessLog, &w.ErrorLog, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.ProjectType = projectType
	w.AppPort = appPort
	return w, nil
}

// Create creates a new website
func (s *WebsiteService) Create(webServerID int64, req *model.CreateWebsiteRequest) (*model.Website, error) {
	// Check web server is installed
	ws, _ := s.getWebServer(webServerID)
	if ws == nil {
		return nil, fmt.Errorf("web server not found")
	}
	if ws.Status == "not_installed" {
		return nil, fmt.Errorf("cannot add website: %s is not installed", ws.DisplayName)
	}

	// Check domain uniqueness
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM websites WHERE domain = ?", req.Domain).Scan(&count)
	if count > 0 {
		return nil, fmt.Errorf("domain %s already exists", req.Domain)
	}

	port := req.Port
	if port == 0 {
		port = 80
	}

	// Auto-configure based on project type
	projectType := req.ProjectType
	if projectType == "" {
		projectType = "static"
	}
	appPort := req.AppPort
	proxyEnabled := false
	proxyPass := ""

	switch projectType {
	case "nodejs":
		if appPort == 0 {
			appPort = 3000
		}
		proxyEnabled = true
		proxyPass = fmt.Sprintf("http://127.0.0.1:%d", appPort)
	case "python":
		if appPort == 0 {
			appPort = 8000
		}
		proxyEnabled = true
		proxyPass = fmt.Sprintf("http://127.0.0.1:%d", appPort)
	case "java":
		if appPort == 0 {
			appPort = 8080
		}
		proxyEnabled = true
		proxyPass = fmt.Sprintf("http://127.0.0.1:%d", appPort)
	case "proxy":
		if appPort == 0 {
			appPort = 8080
		}
		proxyEnabled = true
		proxyPass = fmt.Sprintf("http://127.0.0.1:%d", appPort)
	case "php":
		// php-fpm handled by Nginx fastcgi_pass
		if appPort == 0 {
			appPort = 9000
		}
	}

	accessLog := fmt.Sprintf("/var/log/nginx/%s_access.log", req.Domain)
	errorLog := fmt.Sprintf("/var/log/nginx/%s_error.log", req.Domain)

	result, err := s.db.Exec(`INSERT INTO websites
		(web_server_id, name, domain, root_path, port, project_type, app_port, proxy_enabled, proxy_pass, custom_config, access_log, error_log)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		webServerID, req.Name, req.Domain, req.RootPath, port, projectType, appPort, proxyEnabled, proxyPass, req.CustomConfig, accessLog, errorLog)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	// Create root directory
	os.MkdirAll(req.RootPath, 0755)

	// Write Nginx config
	s.writeConfigForServer(webServerID, &model.Website{
		ID:           id,
		WebServerID:  webServerID,
		Name:         req.Name,
		Domain:       req.Domain,
		RootPath:     req.RootPath,
		Port:         port,
		ProjectType:  projectType,
		AppPort:      appPort,
		ProxyEnabled: proxyEnabled,
		ProxyPass:    proxyPass,
		CustomConfig: req.CustomConfig,
		AccessLog:    accessLog,
		ErrorLog:     errorLog,
	})

	return &model.Website{
		ID:          id,
		WebServerID: webServerID,
		Name:        req.Name,
		Domain:      req.Domain,
		RootPath:    req.RootPath,
		Port:        port,
		ProjectType: projectType,
		AppPort:     appPort,
		Status:      "active",
	}, nil
}

// Update updates a website
func (s *WebsiteService) Update(webServerID, id int64, req *model.UpdateWebsiteRequest) error {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("website not found")
	}

	oldDomain := w.Domain

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
		w.Name = *req.Name
	}
	if req.Domain != nil && *req.Domain != w.Domain {
		// Check new domain uniqueness
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM websites WHERE domain = ? AND id != ?", *req.Domain, id).Scan(&count)
		if count > 0 {
			return fmt.Errorf("domain %s already exists", *req.Domain)
		}
		updates = append(updates, "domain = ?")
		args = append(args, *req.Domain)
		w.Domain = *req.Domain
	}
	if req.RootPath != nil {
		updates = append(updates, "root_path = ?")
		args = append(args, *req.RootPath)
		w.RootPath = *req.RootPath
	}
	if req.Port != nil {
		updates = append(updates, "port = ?")
		args = append(args, *req.Port)
		w.Port = *req.Port
	}
	if req.ProjectType != nil {
		updates = append(updates, "project_type = ?")
		args = append(args, *req.ProjectType)
		w.ProjectType = *req.ProjectType
	}
	if req.AppPort != nil {
		updates = append(updates, "app_port = ?")
		args = append(args, *req.AppPort)
		w.AppPort = *req.AppPort
	}
	if req.CustomConfig != nil {
		updates = append(updates, "custom_config = ?")
		args = append(args, *req.CustomConfig)
		w.CustomConfig = *req.CustomConfig
	}

	if len(updates) == 0 {
		return nil
	}

	updates = append(updates, "updated_at = datetime('now')")
	query := "UPDATE websites SET " + strings.Join(updates, ", ") + " WHERE id = ? AND web_server_id = ?"
	args = append(args, id, webServerID)

	if _, err := s.db.Exec(query, args...); err != nil {
		return err
	}

	// If domain changed, remove old config first
	if oldDomain != w.Domain {
		s.removeConfigForServer(webServerID, oldDomain)
	}

	// Write new config
	s.writeConfigForServer(webServerID, w)

	// If site is active and domain changed, create new symlink
	if w.Status == "active" && oldDomain != w.Domain {
		ws, _ := s.getWebServer(webServerID)
		if ws != nil && ws.SitesAvailable != "" && ws.SitesEnabled != "" {
			confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
			linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
			os.MkdirAll(ws.SitesEnabled, 0755)
			os.Symlink(confPath, linkPath)
		}
	}

	// Reload web server
	ws, _ := s.getWebServer(webServerID)
	if ws != nil && ws.Status == "running" {
		s.reloadWebServer(ws)
	}

	return nil
}

// Delete deletes a website
func (s *WebsiteService) Delete(webServerID, id int64) error {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("website not found")
	}

	s.removeConfigForServer(webServerID, w.Domain)
	_, err = s.db.Exec("DELETE FROM websites WHERE id = ? AND web_server_id = ?", id, webServerID)
	return err
}

// Enable enables a website
func (s *WebsiteService) Enable(webServerID, id int64) error {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("website not found")
	}

	// Check web server is running
	ws, _ := s.getWebServer(webServerID)
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.Status == "not_installed" {
		return fmt.Errorf("cannot enable website: %s is not installed", ws.DisplayName)
	}
	if ws.Status == "stopped" {
		return fmt.Errorf("cannot enable website: %s is stopped, start it first", ws.DisplayName)
	}

	// Write config
	if err := s.writeConfigForServer(webServerID, w); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Create symlink (for Nginx/Apache style)
	if ws.SitesAvailable != "" && ws.SitesEnabled != "" {
		confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		if _, err := os.Stat(linkPath); os.IsNotExist(err) {
			os.Symlink(confPath, linkPath)
		}
	}

	// Reload web server
	s.reloadWebServer(ws)

	s.db.Exec("UPDATE websites SET status = 'active', updated_at = datetime('now') WHERE id = ? AND web_server_id = ?", id, webServerID)
	return nil
}

// Disable disables a website
func (s *WebsiteService) Disable(webServerID, id int64) error {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("website not found")
	}

	ws, _ := s.getWebServer(webServerID)
	if ws != nil && ws.SitesEnabled != "" {
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		os.Remove(linkPath)
		s.reloadWebServer(ws)
	}

	s.db.Exec("UPDATE websites SET status = 'disabled', updated_at = datetime('now') WHERE id = ? AND web_server_id = ?", id, webServerID)
	return nil
}

// GetLogs returns logs for a website
func (s *WebsiteService) GetLogs(webServerID, id int64, logType string, lines int) (string, error) {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return "", err
	}
	if w == nil {
		return "", fmt.Errorf("website not found")
	}

	logPath := w.AccessLog
	if logType == "error" {
		logPath = w.ErrorLog
	}
	if logPath == "" {
		return "", nil
	}
	if lines <= 0 {
		lines = 200
	}

	out, err := exec.Command("tail", "-n", fmt.Sprintf("%d", lines), logPath).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(no log file: %s)", logPath), nil
	}
	return string(out), nil
}

// ApplySSL applies SSL certificate using certbot
func (s *WebsiteService) ApplySSL(webServerID, id int64, email string) error {
	w, err := s.Get(webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("website not found")
	}

	// Check web server is running
	ws, _ := s.getWebServer(webServerID)
	if ws == nil || ws.Status != "running" {
		return fmt.Errorf("cannot apply SSL: web server is not running")
	}

	if _, err := exec.LookPath("certbot"); err != nil {
		return fmt.Errorf("certbot is not installed. Install with: apt install certbot python3-certbot-nginx")
	}

	args := []string{"--nginx", "-d", w.Domain, "--non-interactive", "--agree-tos"}
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	out, err := exec.Command("certbot", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed: %s", string(out))
	}

	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", w.Domain)
	keyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", w.Domain)
	s.db.Exec("UPDATE websites SET ssl_enabled = 1, ssl_cert_path = ?, ssl_key_path = ?, updated_at = datetime('now') WHERE id = ?",
		certPath, keyPath, id)
	return nil
}

// Internal helpers

func (s *WebsiteService) getWebServer(id int64) (*model.WebServer, error) {
	ws := &model.WebServer{}
	err := s.db.QueryRow(`SELECT id, name, display_name, config_path, sites_available, sites_enabled, service_name, status
		FROM web_servers WHERE id = ?`, id).Scan(
		&ws.ID, &ws.Name, &ws.DisplayName, &ws.ConfigPath, &ws.SitesAvailable, &ws.SitesEnabled, &ws.ServiceName, &ws.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ws, err
}

func (s *WebsiteService) writeConfigForServer(webServerID int64, w *model.Website) error {
	ws, err := s.getWebServer(webServerID)
	if err != nil || ws == nil {
		return fmt.Errorf("web server not found")
	}

	// Only generate config for Nginx currently
	if ws.Name != "nginx" {
		return nil
	}

	os.MkdirAll(ws.SitesAvailable, 0755)
	os.MkdirAll(ws.SitesEnabled, 0755)

	confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")

	if w.CustomConfig != "" {
		return os.WriteFile(confPath, []byte(w.CustomConfig), 0644)
	}

	// Select template based on project type
	var config string
	switch w.ProjectType {
	case "php":
		config = nginxPHPTemplate(w)
	case "nodejs", "python", "java", "proxy":
		config = nginxProxyTemplate(w)
	default: // static
		config = nginxStaticTemplate(w)
	}

	return os.WriteFile(confPath, []byte(config), 0644)
}

func (s *WebsiteService) removeConfigForServer(webServerID int64, domain string) {
	ws, _ := s.getWebServer(webServerID)
	if ws == nil {
		return
	}
	if ws.SitesEnabled != "" {
		os.Remove(filepath.Join(ws.SitesEnabled, domain+".conf"))
	}
	if ws.SitesAvailable != "" {
		os.Remove(filepath.Join(ws.SitesAvailable, domain+".conf"))
	}
}

func (s *WebsiteService) reloadWebServer(ws *model.WebServer) {
	if ws.ServiceName == "" {
		return
	}
	// Test config first (for Nginx)
	if ws.Name == "nginx" {
		if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
			log.Printf("website: nginx config test failed: %s", string(out))
			return
		}
	}
	exec.Command("systemctl", "reload", ws.ServiceName).Run()
}

// Nginx config templates per project type

func nginxStaticTemplate(w *model.Website) string {
	return fmt.Sprintf(`# EasyServer - Static site: %s
server {
    listen %d;
    server_name %s;
    root %s;
    index index.html index.htm;

    location / {
        try_files $uri $uri/ /index.html;
    }

    access_log %s;
    error_log %s;
}
`, w.Name, w.Port, w.Domain, w.RootPath, w.AccessLog, w.ErrorLog)
}

func nginxProxyTemplate(w *model.Website) string {
	return fmt.Sprintf(`# EasyServer - %s proxy: %s
server {
    listen %d;
    server_name %s;

    location / {
        proxy_pass %s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }

    access_log %s;
    error_log %s;
}
`, w.ProjectType, w.Name, w.Port, w.Domain, w.ProxyPass, w.AccessLog, w.ErrorLog)
}

func nginxPHPTemplate(w *model.Website) string {
	return fmt.Sprintf(`# EasyServer - PHP site: %s
server {
    listen %d;
    server_name %s;
    root %s;
    index index.php index.html index.htm;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass 127.0.0.1:%d;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }

    location ~ /\.ht {
        deny all;
    }

    access_log %s;
    error_log %s;
}
`, w.Name, w.Port, w.Domain, w.RootPath, w.AppPort, w.AccessLog, w.ErrorLog)
}
