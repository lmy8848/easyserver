package web

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"
)

// WebsiteService manages website deployment and configuration.
type WebsiteService struct {
	repo          WebsiteRepository
	webServerRepo ServerRepository
	executor      executor.CommandExecutor
}

func NewWebsiteService(repo WebsiteRepository, webServerRepo ServerRepository, exec executor.CommandExecutor) *WebsiteService {
	return &WebsiteService{repo: repo, webServerRepo: webServerRepo, executor: exec}
}

// List returns websites for a specific web server
func (s *WebsiteService) List(ctx context.Context, webServerID int64) ([]Website, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.List(ctx, webServerID)
}

// Get returns a specific website
func (s *WebsiteService) Get(ctx context.Context, webServerID, id int64) (*Website, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.Get(ctx, webServerID, id)
}

// Create creates a new website
func (s *WebsiteService) Create(ctx context.Context, webServerID int64, req *CreateWebsiteRequest) (*Website, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate domain safety
	if err := validateDomain(req.Domain); err != nil {
		return nil, err
	}
	// Validate root_path safety
	if err := validateRootPath(req.RootPath); err != nil {
		return nil, err
	}

	// Check web server is installed
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil {
		return nil, apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.Status == "not_installed" {
		return nil, apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无法添加网站：%s 未安装", ws.DisplayName))
	}

	// Check domain uniqueness
	count, _ := s.repo.CountByDomain(ctx, req.Domain)
	if count > 0 {
		return nil, apperror.ErrConflict.WithMessage(fmt.Sprintf("域名 %s 已存在", req.Domain))
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

	website := &Website{
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
	}

	id, err := s.repo.Create(ctx, website)
	if err != nil {
		return nil, err
	}

	// Create root directory
	os.MkdirAll(req.RootPath, 0755)

	// Write Nginx config
	website.ID = id
	s.writeConfigForServer(webServerID, website)

	return &Website{
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
func (s *WebsiteService) Update(ctx context.Context, webServerID, id int64, req *UpdateWebsiteRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return apperror.ErrNotFound.WithMessage("网站不存在")
	}

	oldDomain := w.Domain

	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Domain != nil && *req.Domain != w.Domain {
		// Check new domain uniqueness
		count, _ := s.repo.CountByDomainExcludingID(ctx, *req.Domain, id)
		if count > 0 {
			return apperror.ErrConflict.WithMessage(fmt.Sprintf("域名 %s 已存在", *req.Domain))
		}
		w.Domain = *req.Domain
	}
	if req.RootPath != nil {
		if err := validateRootPath(*req.RootPath); err != nil {
			return err
		}
		w.RootPath = *req.RootPath
	}
	if req.Port != nil {
		w.Port = *req.Port
	}
	if req.ProjectType != nil {
		w.ProjectType = *req.ProjectType
	}
	if req.AppPort != nil {
		w.AppPort = *req.AppPort
	}
	if req.CustomConfig != nil {
		w.CustomConfig = *req.CustomConfig
	}

	if err := s.repo.Update(ctx, w); err != nil {
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
		ws, _ := s.webServerRepo.Get(ctx, webServerID)
		if ws != nil && ws.SitesAvailable != "" && ws.SitesEnabled != "" {
			confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
			linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
			os.MkdirAll(ws.SitesEnabled, 0755)
			os.Symlink(confPath, linkPath)
		}
	}

	// Reload web server
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws != nil && ws.Status == "running" {
		s.reloadWebServer(ctx, ws)
	}

	return nil
}

// Delete deletes a website
func (s *WebsiteService) Delete(ctx context.Context, webServerID, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return apperror.ErrNotFound.WithMessage("网站不存在")
	}

	s.removeConfigForServer(webServerID, w.Domain)
	return s.repo.Delete(ctx, webServerID, id)
}

// Enable enables a website
func (s *WebsiteService) Enable(ctx context.Context, webServerID, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return apperror.ErrNotFound.WithMessage("网站不存在")
	}

	// Check web server is running
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.Status == "not_installed" {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无法启用网站：%s 未安装", ws.DisplayName))
	}
	if ws.Status == "stopped" {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无法启用网站：%s 已停止，请先启动", ws.DisplayName))
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
	s.reloadWebServer(ctx, ws)

	return s.repo.UpdateStatus(ctx, webServerID, id, "active")
}

// Disable disables a website
func (s *WebsiteService) Disable(ctx context.Context, webServerID, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return apperror.ErrNotFound.WithMessage("网站不存在")
	}

	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws != nil && ws.SitesEnabled != "" {
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		os.Remove(linkPath)
		s.reloadWebServer(ctx, ws)
	}

	return s.repo.UpdateStatus(ctx, webServerID, id, "disabled")
}

// GetLogs returns logs for a website
func (s *WebsiteService) GetLogs(ctx context.Context, webServerID, id int64, logType string, lines int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return "", err
	}
	if w == nil {
		return "", apperror.ErrNotFound.WithMessage("网站不存在")
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

	out, _, err := s.executor.RunCombined(ctx, "tail", "-n", fmt.Sprintf("%d", lines), logPath)
	if err != nil {
		return fmt.Sprintf("(no log file: %s)", logPath), nil
	}
	return out, nil
}

// ApplySSL applies SSL certificate using certbot
func (s *WebsiteService) ApplySSL(ctx context.Context, webServerID, id int64, email string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return apperror.ErrNotFound.WithMessage("网站不存在")
	}

	// Check web server is running
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil || ws.Status != "running" {
		return apperror.ErrBadRequest.WithMessage("无法申请 SSL：Web 服务器未运行")
	}

	if _, err := s.executor.LookPath("certbot"); err != nil {
		return apperror.ErrBadRequest.WithMessage("certbot 未安装，请运行: apt install certbot python3-certbot-nginx")
	}

	args := []string{"--nginx", "-d", w.Domain, "--non-interactive", "--agree-tos"}
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	out, _, err := s.executor.RunCombined(ctx, "certbot", args...)
	if err != nil {
		return fmt.Errorf("certbot failed: %s", out)
	}

	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", w.Domain)
	keyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", w.Domain)
	return s.repo.UpdateSSL(ctx, id, certPath, keyPath)
}

// Internal helpers

// validateDomain validates that a domain name is safe to use in file paths
func validateDomain(domain string) error {
	if domain == "" {
		return apperror.ErrBadRequest.WithMessage("域名不能为空")
	}
	// Only allow alphanumeric, hyphens, dots
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`)
	if !domainRegex.MatchString(domain) {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的域名：%s", domain))
	}
	if len(domain) > 253 {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("域名过长：%d 字符", len(domain)))
	}
	return nil
}

func validateRootPath(p string) error {
	if p == "" {
		return apperror.ErrBadRequest.WithMessage("根路径不能为空")
	}
	if !strings.HasPrefix(p, "/") {
		return apperror.ErrBadRequest.WithMessage("根路径必须是绝对路径（以 / 开头）")
	}
	if strings.Contains(p, "..") {
		return apperror.ErrBadRequest.WithMessage("根路径不能包含 '..'")
	}
	// Reject shell metacharacters that could enable injection
	shellMeta := []string{";", "|", "&", "$", "`", "(", ")", "{", "}", "\n", "\r", "\x00"}
	for _, m := range shellMeta {
		if strings.Contains(p, m) {
			return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("根路径包含无效字符：%q", m))
		}
	}
	return nil
}

func (s *WebsiteService) writeConfigForServer(webServerID int64, w *Website) error {
	ws, err := s.webServerRepo.Get(context.Background(), webServerID)
	if err != nil || ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
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
	ws, _ := s.webServerRepo.Get(context.Background(), webServerID)
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

func (s *WebsiteService) reloadWebServer(ctx context.Context, ws *WebServer) {
	if ws.ServiceName == "" {
		return
	}
	// Test config first (for Nginx)
	if ws.Name == "nginx" {
		if out, _, err := s.executor.RunCombined(ctx, "nginx", "-t"); err != nil {
			log.Printf("website: nginx config test failed: %s", out)
			return
		}
	}
	s.executor.RunCombined(ctx, "systemctl", "reload", ws.ServiceName)
}

// Nginx config templates per project type

// sanitizeNginxValue removes newlines, carriage returns, and other control characters
// that could inject arbitrary nginx config directives.
func sanitizeNginxValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\x00' {
			continue
		}
		if r < 0x20 && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func nginxStaticTemplate(w *Website) string {
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
`, sanitizeNginxValue(w.Name), w.Port, sanitizeNginxValue(w.Domain), sanitizeNginxValue(w.RootPath), sanitizeNginxValue(w.AccessLog), sanitizeNginxValue(w.ErrorLog))
}

func nginxProxyTemplate(w *Website) string {
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
`, sanitizeNginxValue(w.ProjectType), sanitizeNginxValue(w.Name), w.Port, sanitizeNginxValue(w.Domain), sanitizeNginxValue(w.ProxyPass), sanitizeNginxValue(w.AccessLog), sanitizeNginxValue(w.ErrorLog))
}

func nginxPHPTemplate(w *Website) string {
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
`, sanitizeNginxValue(w.Name), w.Port, sanitizeNginxValue(w.Domain), sanitizeNginxValue(w.RootPath), w.AppPort, sanitizeNginxValue(w.AccessLog), sanitizeNginxValue(w.ErrorLog))
}
