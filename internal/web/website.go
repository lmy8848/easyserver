package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"easyserver/internal/infra/errx"
	"easyserver/internal/web/security"
)

// WebsiteService manages website deployment and configuration.
type WebsiteService struct {
	repo          WebsiteRepository
	webServerRepo ServerRepository
	securityRepo  security.SecurityRepository
}

func NewWebsiteService(repo WebsiteRepository, webServerRepo ServerRepository, securityRepo security.SecurityRepository) *WebsiteService {
	return &WebsiteService{repo: repo, webServerRepo: webServerRepo, securityRepo: securityRepo}
}

// List returns websites for a specific web server
func (s *WebsiteService) List(ctx context.Context, webServerID int64) ([]Website, error) {
	return s.repo.List(ctx, webServerID)
}

// Get returns a specific website
func (s *WebsiteService) Get(ctx context.Context, webServerID, id int64) (*Website, error) {
	return s.repo.Get(ctx, webServerID, id)
}

// Create creates a new website
func (s *WebsiteService) Create(ctx context.Context, webServerID int64, req *CreateWebsiteRequest) (*Website, error) {
	// Validate domain safety
	if err := validateDomain(req.Domain); err != nil {
		return nil, err
	}
	// Validate root_path safety
	if err := validateRootPath(req.RootPath); err != nil {
		return nil, err
	}

	if err := req.ValidateDomain(); err != nil {
		return nil, errx.BadRequest("无效的域名: %w", err)
	}

	// Check web server is installed
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil {
		return nil, errx.NotFound("Web 服务器不存在")
	}
	if ws.Status == "not_installed" {
		return nil, errx.BadRequest("无法添加网站：%s 未安装", ws.DisplayName)
	}

	// Check domain uniqueness
	count, _ := s.repo.CountByDomain(ctx, req.Domain)
	if count > 0 {
		return nil, errx.Conflict("域名 %s 已存在", req.Domain)
	}

	// Check port conflict
	checkPort := req.Port
	if checkPort == 0 {
		checkPort = 80
	}
	portCount, _ := s.repo.CountByPort(ctx, webServerID, checkPort)
	if portCount > 0 {
		return nil, errx.Conflict("端口 %d 已被其他网站占用", checkPort)
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
		proxyEnabled = true
		if appPort > 0 {
			proxyPass = fmt.Sprintf("http://127.0.0.1:%d", appPort)
		}
	case "php":
		if appPort == 0 {
			appPort = 9000 // PHP-FPM default
		}
	}

	// Default log paths
	accessLog := fmt.Sprintf("/var/log/nginx/%s_access.log", req.Domain)
	errorLog := fmt.Sprintf("/var/log/nginx/%s_error.log", req.Domain)

	siteName := req.Name
	if siteName == "" {
		siteName = req.Domain
	}

	w := &Website{
		WebServerID:   webServerID,
		Name:          siteName,
		Domain:        req.Domain,
		Port:          port,
		RootPath:      req.RootPath,
		ProjectType:   projectType,
		AppPort:       appPort,
		Status:        "active",
		ProxyEnabled:  proxyEnabled,
		ProxyPass:     proxyPass,
		AccessLog:     accessLog,
		ErrorLog:      errorLog,
		CustomConfig:  req.CustomConfig,
		ConfigOptions: req.ConfigOptions,
		BuildCommand:  req.BuildCommand,
		StartCommand:  req.StartCommand,
	}

	id, err := s.repo.Create(ctx, w)
	if err != nil {
		return nil, err
	}
	w.ID = id

	// Generate and write config file
	if err := s.writeConfigForServer(ctx, webServerID, w); err != nil {
		log.Printf("website: failed to write config for %s: %v", w.Domain, err)
	}

	// Create symlink for Nginx/Apache style
	if ws.SitesAvailable != "" && ws.SitesEnabled != "" {
		confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		_ = os.MkdirAll(ws.SitesEnabled, 0755)
		_ = os.Symlink(confPath, linkPath)
	}

	// Reload web server if running
	if ws.Status == "running" {
		s.reloadWebServer(ctx, ws)
	}

	return w, nil
}

// recalcProxyDefaults 在 project_type 变更时重算 ProxyEnabled / ProxyPass / AppPort，
// 逻辑与 Create 保持完全一致。
func recalcProxyDefaults(w *Website) {
	switch w.ProjectType {
	case "nodejs":
		if w.AppPort == 0 {
			w.AppPort = 3000
		}
		w.ProxyEnabled = true
		w.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d", w.AppPort)
	case "python":
		if w.AppPort == 0 {
			w.AppPort = 8000
		}
		w.ProxyEnabled = true
		w.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d", w.AppPort)
	case "java":
		if w.AppPort == 0 {
			w.AppPort = 8080
		}
		w.ProxyEnabled = true
		w.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d", w.AppPort)
	case "proxy":
		w.ProxyEnabled = true
		if w.ProxyPass == "" && w.AppPort > 0 {
			w.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d", w.AppPort)
		}
	case "php":
		if w.AppPort == 0 {
			w.AppPort = 9000
		}
		w.ProxyEnabled = false
		w.ProxyPass = ""
	default: // static
		w.ProxyEnabled = false
		w.ProxyPass = ""
	}
}

// Update updates a website
func (s *WebsiteService) Update(ctx context.Context, webServerID, id int64, req *UpdateWebsiteRequest) error {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return errx.NotFound("网站不存在")
	}

	oldDomain := w.Domain
	oldProjectType := w.ProjectType

	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Domain != nil && *req.Domain != w.Domain {
		if !domainRegexp.MatchString(*req.Domain) {
			return errx.BadRequest("invalid domain format: must be a valid RFC 1123 hostname")
		}
		// Check new domain uniqueness
		count, _ := s.repo.CountByDomainExcludingID(ctx, *req.Domain, id)
		if count > 0 {
			return errx.Conflict("域名 %s 已存在", *req.Domain)
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
		// Check port conflict (exclude self)
		portCount, _ := s.repo.CountByPortExcludingID(ctx, webServerID, *req.Port, id)
		if portCount > 0 {
			return errx.Conflict("端口 %d 已被其他网站占用", *req.Port)
		}
		w.Port = *req.Port
	}
	if req.ProjectType != nil {
		w.ProjectType = *req.ProjectType
	}
	if req.AppPort != nil {
		w.AppPort = *req.AppPort
	}
	// 项目类型变更时重算 proxy 默认值（与 Create 一致）。否则 static 改 nodejs
	// 后 proxy_pass 仍为空，nginxProxyTemplate 会生成无效的 "proxy_pass ;" 配置。
	if req.ProjectType != nil && *req.ProjectType != oldProjectType {
		recalcProxyDefaults(w)
	}
	if req.CustomConfig != nil {
		w.CustomConfig = *req.CustomConfig
	}
	if req.ConfigOptions != nil {
		w.ConfigOptions = *req.ConfigOptions
	}
	if req.BuildCommand != nil {
		w.BuildCommand = *req.BuildCommand
	}
	if req.StartCommand != nil {
		w.StartCommand = *req.StartCommand
	}
	if req.ProcessID != nil {
		w.ProcessID = *req.ProcessID
	}

	if err := s.repo.Update(ctx, w); err != nil {
		return err
	}

	// If domain changed, remove old config first
	if oldDomain != w.Domain {
		s.removeConfigForServer(ctx, webServerID, oldDomain)
	}

	// Write new config
	if err := s.writeConfigForServer(ctx, webServerID, w); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// If site is active and domain changed, create new symlink
	if w.Status == "active" && oldDomain != w.Domain {
		ws, _ := s.webServerRepo.Get(ctx, webServerID)
		if ws != nil && ws.SitesAvailable != "" && ws.SitesEnabled != "" {
			if domainRegexp.MatchString(w.Domain) {
				confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
				linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
				relConf, errConf := filepath.Rel(ws.SitesAvailable, confPath)
				relLink, errLink := filepath.Rel(ws.SitesEnabled, linkPath)
				if errConf == nil && !strings.HasPrefix(relConf, "..") && errLink == nil && !strings.HasPrefix(relLink, "..") {
					_ = os.MkdirAll(ws.SitesEnabled, 0755)
					_ = os.Symlink(confPath, linkPath)
				}
			}
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
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return errx.NotFound("网站不存在")
	}

	s.removeConfigForServer(ctx, webServerID, w.Domain)
	return s.repo.Delete(ctx, webServerID, id)
}

// Enable enables a website
func (s *WebsiteService) Enable(ctx context.Context, webServerID, id int64) error {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return errx.NotFound("网站不存在")
	}

	// Check web server is running
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil {
		return errx.NotFound("Web 服务器不存在")
	}
	if ws.Status == "not_installed" {
		return errx.BadRequest("无法启用网站：%s 未安装", ws.DisplayName)
	}
	if ws.Status == "stopped" {
		return errx.BadRequest("无法启用网站：%s 已停止，请先启动", ws.DisplayName)
	}

	// Write config
	if err := s.writeConfigForServer(ctx, webServerID, w); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Create symlink (for Nginx/Apache style)
	if ws.SitesAvailable != "" && ws.SitesEnabled != "" {
		confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		if _, err := os.Stat(linkPath); os.IsNotExist(err) {
			_ = os.Symlink(confPath, linkPath)
		}
	}

	// Reload web server
	s.reloadWebServer(ctx, ws)

	return s.repo.UpdateStatus(ctx, webServerID, id, "active")
}

// Disable disables a website
func (s *WebsiteService) Disable(ctx context.Context, webServerID, id int64) error {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return errx.NotFound("网站不存在")
	}

	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws != nil && ws.SitesEnabled != "" {
		linkPath := filepath.Join(ws.SitesEnabled, w.Domain+".conf")
		os.Remove(linkPath)
		s.reloadWebServer(ctx, ws)
	}

	return s.repo.UpdateStatus(ctx, webServerID, id, "disabled")
}

// LinkProcess sets the linked process ID for a website
func (s *WebsiteService) LinkProcess(ctx context.Context, id, processID int64) error {
	return s.repo.UpdateProcessID(ctx, id, processID)
}

// GetLogs returns logs for a website
func (s *WebsiteService) GetLogs(ctx context.Context, webServerID, id int64, logType string, lines int) (string, error) {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return "", err
	}
	if w == nil {
		return "", errx.NotFound("网站不存在")
	}

	logPath := w.AccessLog
	if logType == "error" {
		logPath = w.ErrorLog
	}
	if logType == "app" {
		// 应用启动/运行日志：StartWebsiteProcess 和进程守护把 nohup stdout 重定向到这里
		logPath = fmt.Sprintf("/var/log/easyserver/%s.log", w.Domain)
	}
	if logPath == "" {
		return "", nil
	}
	// 文件不存在时给出友好提示，而不是返回 tail 的 stderr（"tail: cannot open ..."）。
	// nginx 在网站首次有访问/错误时才会创建该日志文件。
	if _, err := os.Stat(logPath); err != nil {
		return fmt.Sprintf("日志文件尚不存在: %s\n（网站产生访问或错误后 nginx 会自动创建该文件）", logPath), nil //nolint:nilerr // 日志文件不存在时返回友好提示
	}
	if lines <= 0 {
		lines = 200
	}

	out, err := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), logPath).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(读取日志失败: %s)", logPath), nil //nolint:nilerr // 读取日志失败时返回友好提示
	}
	return string(out), nil
}

// ApplySSL applies SSL certificate using certbot
func (s *WebsiteService) ApplySSL(ctx context.Context, webServerID, id int64, email string) error {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if w == nil {
		return errx.NotFound("网站不存在")
	}

	// Check web server is running
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil || ws.Status != "running" {
		return errx.BadRequest("无法申请 SSL：Web 服务器未运行")
	}

	if _, err := exec.LookPath("certbot"); err != nil {
		return errx.BadRequest("certbot 未安装，请运行: apt install certbot python3-certbot-nginx")
	}

	args := []string{"--nginx", "-d", w.Domain, "--non-interactive", "--agree-tos"}
	if email != "" {
		args = append(args, "--email", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	out, err := exec.CommandContext(ctx, "certbot", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed: %s", out)
	}

	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", w.Domain)
	keyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", w.Domain)
	return s.repo.UpdateSSL(ctx, id, certPath, keyPath)
}

// UploadSSL 启用用户上传的证书：更新数据库 + 重新生成 nginx 配置(含 SSL) + reload。
// 与 ApplySSL(certbot 申请) 不同，这里接收已写好的证书文件路径。
func (s *WebsiteService) UploadSSL(ctx context.Context, webServerID, id int64, certPath, keyPath string) error {
	if err := s.repo.UpdateSSL(ctx, id, certPath, keyPath); err != nil {
		return err
	}
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil || w == nil {
		return errx.NotFound("网站不存在")
	}
	if err := s.writeConfigForServer(ctx, webServerID, w); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws != nil && ws.Status == "running" {
		s.reloadWebServer(ctx, ws)
	}
	return nil
}

// Internal helpers

// validateDomain validates that a domain name is safe to use in file paths
func validateDomain(domain string) error {
	if domain == "" {
		return errx.BadRequest("域名不能为空")
	}
	// Only allow alphanumeric, hyphens, dots
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*$`)
	if !domainRegex.MatchString(domain) {
		return errx.BadRequest("无效的域名：%s", domain)
	}
	if len(domain) > 253 {
		return errx.BadRequest("域名过长：%d 字符", len(domain))
	}
	return nil
}

func validateRootPath(p string) error {
	if p == "" {
		return errx.BadRequest("根路径不能为空")
	}
	if !strings.HasPrefix(p, "/") {
		return errx.BadRequest("根路径必须是绝对路径（以 / 开头）")
	}
	if strings.Contains(p, "..") {
		return errx.BadRequest("根路径不能包含 '..'")
	}
	// Reject shell metacharacters that could enable injection
	shellMeta := []string{";", "|", "&", "$", "`", "(", ")", "{", "}", "\n", "\r", "\x00"}
	for _, m := range shellMeta {
		if strings.Contains(p, m) {
			return errx.BadRequest("根路径包含无效字符：%q", m)
		}
	}
	return nil
}

func (s *WebsiteService) writeConfigForServer(ctx context.Context, webServerID int64, w *Website) error {
	ws, err := s.webServerRepo.Get(ctx, webServerID)
	if err != nil || ws == nil {
		return errx.NotFound("Web 服务器不存在")
	}

	// Only generate config for Nginx currently
	if ws.Name != "nginx" {
		return nil
	}

	if !domainRegexp.MatchString(w.Domain) {
		return fmt.Errorf("invalid domain: %s", w.Domain)
	}

	_ = os.MkdirAll(ws.SitesAvailable, 0755)
	_ = os.MkdirAll(ws.SitesEnabled, 0755)

	confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
	rel, err := filepath.Rel(ws.SitesAvailable, confPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid domain config path: %s", w.Domain)
	}

	if w.CustomConfig != "" {
		return os.WriteFile(confPath, []byte(w.CustomConfig), 0644)
	}

	// Fetch security config for rate limiting (nil-safe if not wired).
	var secCfg *security.SecurityConfig
	if s.securityRepo != nil {
		secCfg, _ = s.securityRepo.GetConfig(ctx, w.ID)
	}
	rateLimitBlock := nginxRateLimitBlock(secCfg, w.ID)

	// Select template based on project type
	var config string
	switch w.ProjectType {
	case "php":
		config = nginxPHPTemplate(w, rateLimitBlock)
	case "nodejs", "python", "java", "proxy":
		config = nginxProxyTemplate(w, rateLimitBlock)
	default: // static
		config = nginxStaticTemplate(w, rateLimitBlock)
	}

	return os.WriteFile(confPath, []byte(config), 0644)
}

func (s *WebsiteService) removeConfigForServer(ctx context.Context, webServerID int64, domain string) {
	if !domainRegexp.MatchString(domain) {
		return
	}
	ws, _ := s.webServerRepo.Get(ctx, webServerID)
	if ws == nil {
		return
	}
	if ws.SitesEnabled != "" {
		linkPath := filepath.Join(ws.SitesEnabled, domain+".conf")
		if rel, err := filepath.Rel(ws.SitesEnabled, linkPath); err == nil && !strings.HasPrefix(rel, "..") {
			_ = os.Remove(linkPath)
		}
	}
	if ws.SitesAvailable != "" {
		confPath := filepath.Join(ws.SitesAvailable, domain+".conf")
		if rel, err := filepath.Rel(ws.SitesAvailable, confPath); err == nil && !strings.HasPrefix(rel, "..") {
			_ = os.Remove(confPath)
		}
	}
}

func (s *WebsiteService) reloadWebServer(ctx context.Context, ws *WebServer) {
	if ws.ServiceName == "" {
		return
	}
	// Test config first (for Nginx)
	if ws.Name == "nginx" {
		if out, err := exec.CommandContext(ctx, "nginx", "-t").CombinedOutput(); err != nil {
			log.Printf("website: nginx config test failed: %s", out)
			return
		}
	}
	_, _ = exec.CommandContext(ctx, "systemctl", "reload", ws.ServiceName).CombinedOutput()
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

// NginxConfigOptions 是 config_options JSON 的结构化开关
type NginxConfigOptions struct {
	WebSocket     bool `json:"websocket"`      // proxy 类型是否加 WebSocket 头
	Gzip          bool `json:"gzip"`           // 是否开启 gzip
	HTTPSRedirect bool `json:"https_redirect"` // SSL 时 80->443 跳转
	AccessLog     bool `json:"access_log"`     // 是否记访问日志
}

// ParseConfigOptions 解析 config_options JSON，缺失字段用默认值
// （代理类型默认开 WebSocket + AccessLog，其余关）
func ParseConfigOptions(s, projectType string) NginxConfigOptions {
	isProxy := projectType == "nodejs" || projectType == "python" || projectType == "java" || projectType == "proxy"
	opts := NginxConfigOptions{WebSocket: isProxy, Gzip: false, HTTPSRedirect: false, AccessLog: true}
	s = strings.TrimSpace(s)
	if s == "" {
		return opts
	}
	var m map[string]bool
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return opts
	}
	if v, ok := m["websocket"]; ok {
		opts.WebSocket = v
	}
	if v, ok := m["gzip"]; ok {
		opts.Gzip = v
	}
	if v, ok := m["https_redirect"]; ok {
		opts.HTTPSRedirect = v
	}
	if v, ok := m["access_log"]; ok {
		opts.AccessLog = v
	}
	return opts
}

// nginxSSLConfig 返回 SSL 配置：listen 后缀(ssl) + 证书指令块。SSLEnabled 且证书路径存在时生效。
func nginxSSLConfig(w *Website) (listenSuffix, sslBlock string) {
	if !w.SSLEnabled || w.SSLCertPath == "" || w.SSLKeyPath == "" {
		return "", ""
	}
	return " ssl", fmt.Sprintf("    ssl_certificate %s;\n    ssl_certificate_key %s;\n", sanitizeNginxValue(w.SSLCertPath), sanitizeNginxValue(w.SSLKeyPath))
}

// nginxRateLimitBlock 返回限流配置块（server 块外）。
func nginxRateLimitBlock(cfg *security.SecurityConfig, websiteID int64) string {
	if cfg == nil || !cfg.RateLimitEnabled {
		return ""
	}
	rate := cfg.RateLimitRate
	if rate <= 0 {
		rate = 10
	}
	burst := cfg.RateLimitBurst
	if burst <= 0 {
		burst = 20
	}
	conn := cfg.LimitConn
	if conn <= 0 {
		conn = 100
	}
	return fmt.Sprintf(
		"limit_req_zone $binary_remote_addr zone=perip_%d:10m rate=%dr/s;\nlimit_conn_zone $binary_remote_addr zone=perconn_%d:10m;\n",
		websiteID, rate, websiteID) +
		fmt.Sprintf("\n    limit_req zone=perip_%d burst=%d nodelay;\n    limit_conn perconn_%d %d;\n",
			websiteID, burst, websiteID, conn)
}

func nginxStaticTemplate(w *Website, rateLimitBlock string) string {
	opts := ParseConfigOptions(w.ConfigOptions, w.ProjectType)
	listenSuffix, sslBlock := nginxSSLConfig(w)
	gzipBlock := ""
	if opts.Gzip {
		gzipBlock = "    gzip on;\n    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;\n"
	}
	accessLogLine := fmt.Sprintf("    access_log %s;", sanitizeNginxValue(w.AccessLog))
	if !opts.AccessLog {
		accessLogLine = "    access_log off;"
	}
	return fmt.Sprintf(`# EasyServer - Static site: %s
%s
server {
    listen %d%s;
    server_name %s;
    root %s;
    index index.html index.htm;
%s%s
    location / {
        try_files $uri $uri/ /index.html;
    }

    %s
    include /etc/nginx/conf.d/banned_ips.conf;
    error_log %s;
}
`, sanitizeNginxValue(w.Name), rateLimitBlock, w.Port, listenSuffix, sanitizeNginxValue(w.Domain), sanitizeNginxValue(w.RootPath), sslBlock, gzipBlock, accessLogLine, sanitizeNginxValue(w.ErrorLog))
}

func nginxProxyTemplate(w *Website, rateLimitBlock string) string {
	// 防御：proxy_pass 为空时回退到 static 模板，避免生成无效的 "proxy_pass ;"
	// 配置（nginx -t 会失败，导致整站不生效、日志文件不生成）。
	if strings.TrimSpace(w.ProxyPass) == "" {
		return nginxStaticTemplate(w, rateLimitBlock)
	}
	opts := ParseConfigOptions(w.ConfigOptions, w.ProjectType)
	listenSuffix, sslBlock := nginxSSLConfig(w)
	wsHeaders := ""
	if opts.WebSocket {
		wsHeaders = "        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n"
	}
	gzipBlock := ""
	if opts.Gzip {
		gzipBlock = "    gzip on;\n    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;\n"
	}
	accessLogLine := fmt.Sprintf("    access_log %s;", sanitizeNginxValue(w.AccessLog))
	if !opts.AccessLog {
		accessLogLine = "    access_log off;"
	}
	httpsRedirect := ""
	if opts.HTTPSRedirect && w.SSLEnabled {
		// 497 = HTTP 请求发到 SSL 端口时 nginx 内部错误码，用 301 跳到 HTTPS 域名
		httpsRedirect = fmt.Sprintf("    error_page 497 =301 https://%s$request_uri;\n", sanitizeNginxValue(w.Domain))
	}
	return fmt.Sprintf(`# EasyServer - %s proxy: %s
%s
server {
    listen %d%s;
    server_name %s;
%s%s%s
    location / {
        proxy_pass %s;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
%s        proxy_read_timeout 86400;
    }

    %s
    error_log %s;
}
`, sanitizeNginxValue(w.ProjectType), sanitizeNginxValue(w.Name), rateLimitBlock, w.Port, listenSuffix, sanitizeNginxValue(w.Domain), sslBlock, gzipBlock, httpsRedirect, sanitizeNginxValue(w.ProxyPass), wsHeaders, accessLogLine, sanitizeNginxValue(w.ErrorLog))
}

func nginxPHPTemplate(w *Website, rateLimitBlock string) string {
	return fmt.Sprintf(`# EasyServer - PHP site: %s
%s
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
    include /etc/nginx/conf.d/banned_ips.conf;
    error_log %s;
}
`, sanitizeNginxValue(w.Name), rateLimitBlock, w.Port, sanitizeNginxValue(w.Domain), sanitizeNginxValue(w.RootPath), w.AppPort, sanitizeNginxValue(w.AccessLog), sanitizeNginxValue(w.ErrorLog))
}
