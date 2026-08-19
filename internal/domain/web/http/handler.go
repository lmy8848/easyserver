package http

import (
	"crypto/tls"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"easyserver/internal/domain/web"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type WebServerHandler struct {
	webServerService *web.Service
	websiteService   *web.WebsiteService
}

func NewWebServerHandler(webServerService *web.Service, websiteService *web.WebsiteService) *WebServerHandler {
	return &WebServerHandler{
		webServerService: webServerService,
		websiteService:   websiteService,
	}
}

// Web Server endpoints

func (h *WebServerHandler) List(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	// Refresh status for all servers
	h.webServerService.RefreshAllStatus(ctx)

	servers, err := h.webServerService.List(ctx)
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(servers, httpx.ParsePagination(c, 50, 200)), nil
}

func (h *WebServerHandler) Get(c *gin.Context) (any, error) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	_ = h.webServerService.RefreshStatus(ctx, id)

	server, err := h.webServerService.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, errx.NotFound("Web 服务器不存在")
	}
	return server, nil
}

func (h *WebServerHandler) Create(c *gin.Context) (any, error) {
	var req web.CreateWebServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "创建 Web服务 "+req.Name)

	// Validate Name format: alphanumeric, hyphen, underscore only
	if !nameRegexp.MatchString(req.Name) {
		return nil, errx.BadRequest("名称只能包含字母、数字、连字符或下划线")
	}

	// Validate DisplayName if provided
	if req.DisplayName != "" && strings.TrimSpace(req.DisplayName) == "" {
		return nil, errx.BadRequest("显示名称不能为空白")
	}

	// Look up the predefined template — only predefined server types are allowed
	predef := web.FindPredefinedWebServer(req.Name)
	if predef == nil {
		return nil, errx.BadRequest("未知的服务器类型 '%s'; 允许的类型: %v", req.Name, web.GetPredefinedWebServerNames())
	}

	// Build the WebServer from the trusted template, with optional display overrides
	ws := *predef // copy all safe fields from template
	if req.DisplayName != "" {
		ws.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		ws.Description = req.Description
	}

	if err := h.webServerService.Create(c.Request.Context(), &ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (h *WebServerHandler) Delete(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "删除 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Delete(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *WebServerHandler) Install(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "安装 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Install(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "已安装"}, nil
}

func (h *WebServerHandler) Uninstall(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "卸载 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Uninstall(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "已卸载"}, nil
}

func (h *WebServerHandler) Start(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "启动 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Start(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"status": "running"}, nil
}

func (h *WebServerHandler) Stop(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "停止 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Stop(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"status": "stopped"}, nil
}

func (h *WebServerHandler) Restart(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "重启 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Restart(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"status": "running"}, nil
}

func (h *WebServerHandler) Status(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	ctx := c.Request.Context()
	_ = h.webServerService.RefreshStatus(ctx, id)

	server, err := h.webServerService.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, errx.NotFound("Web 服务器不存在")
	}
	return gin.H{"status": server.Status, "version": server.Version}, nil
}

func (h *WebServerHandler) Reload(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "重载 Web服务 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.Reload(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "已重载"}, nil
}

func (h *WebServerHandler) TestConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	middleware.AuditSummary(c, "测试 Web服务配置 "+strconv.FormatInt(id, 10))
	ok, msg := h.webServerService.TestConfig(c.Request.Context(), id)
	return gin.H{"valid": ok, "message": msg}, nil
}

func (h *WebServerHandler) GetConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	content, err := h.webServerService.GetConfig(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	return gin.H{"content": content}, nil
}

func (h *WebServerHandler) SaveConfig(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "保存 Web服务配置 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.SaveConfig(c.Request.Context(), id, req.Content); err != nil {
		return nil, err
	}
	return gin.H{"message": "已保存"}, nil
}

func (h *WebServerHandler) GetServiceLogs(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	lines := httpx.QueryInt(c, "lines", 100)
	if lines <= 0 {
		lines = 100
	}

	logs, err := h.webServerService.GetServiceLogs(c.Request.Context(), id, lines)
	if err != nil {
		return nil, err
	}
	return gin.H{"logs": logs}, nil
}

func (h *WebServerHandler) SetAutoStart(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "设置 Web服务自启 #"+strconv.FormatInt(id, 10))

	if err := h.webServerService.SetAutoStart(c.Request.Context(), id, req.Enabled); err != nil {
		return nil, err
	}
	return gin.H{"auto_start": req.Enabled}, nil
}

func (h *WebServerHandler) GetProcessInfo(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	pid, mem, uptime, err := h.webServerService.GetProcessInfo(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	return gin.H{"pid": pid, "memory_bytes": mem, "uptime": uptime}, nil
}

// Website endpoints (nested under web server)

func (h *WebServerHandler) ListWebsites(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}

	sites, err := h.websiteService.List(c.Request.Context(), sid)
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(sites, httpx.ParsePagination(c, 50, 200)), nil
}

func (h *WebServerHandler) GetWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	site, err := h.websiteService.Get(c.Request.Context(), sid, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, errx.NotFound("网站不存在")
	}
	return site, nil
}

func (h *WebServerHandler) CreateWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}

	var req web.CreateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "创建网站 "+req.Domain)

	if err := req.ValidateDomain(); err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	site, err := h.websiteService.Create(c.Request.Context(), sid, &req)
	if err != nil {
		return nil, err
	}

	return site, nil
}

func (h *WebServerHandler) UpdateWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req web.UpdateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "更新网站 #"+strconv.FormatInt(id, 10))

	if err := h.websiteService.Update(c.Request.Context(), sid, id, &req); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *WebServerHandler) DeleteWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "删除网站 #"+strconv.FormatInt(id, 10))

	if err := h.websiteService.Delete(c.Request.Context(), sid, id); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *WebServerHandler) EnableWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "启用网站 #"+strconv.FormatInt(id, 10))

	if err := h.websiteService.Enable(c.Request.Context(), sid, id); err != nil {
		return nil, err
	}

	return gin.H{"status": "active"}, nil
}

func (h *WebServerHandler) DisableWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	middleware.AuditSummary(c, "禁用网站 #"+strconv.FormatInt(id, 10))

	if err := h.websiteService.Disable(c.Request.Context(), sid, id); err != nil {
		return nil, err
	}

	return gin.H{"status": "disabled"}, nil
}

func (h *WebServerHandler) GetWebsiteLogs(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	logType := c.DefaultQuery("type", "access")
	lines := httpx.QueryInt(c, "lines", 200)
	if lines <= 0 {
		lines = 200
	}

	logs, err := h.websiteService.GetLogs(c.Request.Context(), sid, id, logType, lines)
	if err != nil {
		return nil, err
	}
	return gin.H{"logs": logs, "type": logType}, nil
}

func (h *WebServerHandler) ApplyWebsiteSSL(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "应用网站SSL证书 #"+strconv.FormatInt(id, 10))

	if err := h.websiteService.ApplySSL(c.Request.Context(), sid, id, req.Email); err != nil {
		return nil, err
	}
	return gin.H{"message": "SSL 证书已应用"}, nil
}

// UploadWebsiteSSL 接收用户上传的 PEM 证书+私钥，校验后写文件并启用 SSL
func (h *WebServerHandler) UploadWebsiteSSL(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	var req struct {
		CertContent string `json:"cert_content" binding:"required"`
		KeyContent  string `json:"key_content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "上传网站SSL证书 #"+strconv.FormatInt(id, 10))

	// 校验 PEM 格式且 cert 与 key 匹配
	certPEM := []byte(strings.TrimSpace(req.CertContent))
	keyPEM := []byte(strings.TrimSpace(req.KeyContent))
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, errx.BadRequest("证书与私钥不匹配或格式错误: %w", err)
	}

	sslDir := "/etc/nginx/ssl"
	if err := os.MkdirAll(sslDir, 0700); err != nil {
		return nil, errx.Internal("创建 SSL 目录失败")
	}
	certPath := filepath.Join(sslDir, fmt.Sprintf("site_%d.crt", id))
	keyPath := filepath.Join(sslDir, fmt.Sprintf("site_%d.key", id))
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, errx.Internal("写证书文件失败")
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, errx.Internal("写私钥文件失败")
	}

	if err := h.websiteService.UploadSSL(c.Request.Context(), sid, id, certPath, keyPath); err != nil {
		return nil, err
	}
	return gin.H{"message": "证书已上传并启用"}, nil
}

func (h *WebServerHandler) BuildWebsite(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}

	site, err := h.websiteService.Get(c.Request.Context(), sid, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, errx.NotFound("网站不存在")
	}
	if site.BuildCommand == "" {
		return nil, errx.BadRequest("该网站未设置编译命令")
	}

	middleware.AuditSummary(c, "编译网站 "+site.Domain)

	// Run build command in project root directory
	buildCmd := exec.CommandContext(c.Request.Context(), "sh", "-c", site.BuildCommand)
	buildCmd.Dir = site.RootPath
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		return gin.H{"success": false, "output": string(out) + "\n" + err.Error()}, nil //nolint:nilerr // Build failure output is returned in JSON payload
	}
	return gin.H{"success": true, "output": string(out)}, nil
}

// GetProjectTypes returns available project types
func (h *WebServerHandler) GetProjectTypes(c *gin.Context) (any, error) {
	return web.GetProjectTypes(), nil
}

// Directory browser

// nameRegexp validates web server Name: alphanumeric, hyphen, underscore only.
var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// allowedRoots defines safe base directories for website root paths
var allowedRoots = []string{"/var/www", "/home", "/opt", "/srv", "/usr/share"}

type DirEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	HasItems bool   `json:"has_items"` // has package.json, index.php, etc.
	Project  string `json:"project"`   // detected project type
}

func (h *WebServerHandler) BrowseDirs(c *gin.Context) (any, error) {
	reqPath := c.DefaultQuery("path", "/var/www")

	// Security: must be under allowed roots
	safePath, ok := sanitizeAllowedPath(reqPath)
	if !ok {
		return nil, errx.BadRequest("路径必须在以下目录下: %s", strings.Join(allowedRoots, ", "))
	}
	reqPath = safePath

	// Check directory exists
	info, err := os.Stat(reqPath)
	if err != nil {
		return nil, errx.NotFound("目录不存在")
	}
	if !info.IsDir() {
		return nil, errx.BadRequest("不是目录")
	}

	entries, err := os.ReadDir(reqPath)
	if err != nil {
		return nil, errx.Internal("无法读取目录")
	}

	var dirs []DirEntry
	for _, e := range entries {
		// Skip hidden files and system directories
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(reqPath, e.Name())

		if e.IsDir() {
			dirs = append(dirs, DirEntry{
				Name:     e.Name(),
				Path:     fullPath,
				IsDir:    true,
				HasItems: hasProjectFiles(fullPath),
				Project:  detectProjectType(fullPath),
			})
		}
	}

	// Add parent directory
	parent := filepath.Dir(reqPath)
	if parent != reqPath && isAllowedPath(parent) {
		dirs = append([]DirEntry{{Name: "..", Path: parent, IsDir: true}}, dirs...)
	}

	return gin.H{
		"current": reqPath,
		"entries": dirs,
	}, nil
}

// ValidatePath validates a root path for website creation
func (h *WebServerHandler) ValidatePath(c *gin.Context) (any, error) {
	reqPath := c.Query("path")
	if reqPath == "" {
		return nil, errx.BadRequest("路径不能为空")
	}

	// Security check
	safePath, ok := sanitizeAllowedPath(reqPath)
	if !ok {
		return gin.H{
			"valid":   false,
			"message": "路径必须在以下目录下: " + strings.Join(allowedRoots, ", "),
		}, nil
	}
	reqPath = safePath

	// Check if exists
	info, err := os.Stat(reqPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Can be created
			return gin.H{
				"valid":   true,
				"message": "目录将会被创建",
				"exists":  false,
			}, nil
		}
		return gin.H{
			"valid":   false,
			"message": "无法访问路径",
		}, nil
	}

	if !info.IsDir() {
		return gin.H{
			"valid":   false,
			"message": "路径不是目录",
		}, nil
	}

	// Check write permission via file mode bits
	writable := info.Mode().Perm()&0200 != 0
	if writable {
		return gin.H{
			"valid":    true,
			"message":  "目录已就绪",
			"exists":   true,
			"writable": true,
			"project":  detectProjectType(reqPath),
		}, nil
	}

	// Readable but not writable
	return gin.H{
		"valid":    true,
		"message":  "目录存在但可能不可写",
		"exists":   true,
		"writable": false,
		"project":  detectProjectType(reqPath),
	}, nil
}

// sanitizeAllowedPath checks if a path is under allowed root directories
func sanitizeAllowedPath(p string) (string, bool) {
	if p == "" || strings.Contains(p, "\x00") || strings.Contains(p, "..") {
		return "", false
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		return "", false
	}
	absPath, err := filepath.Abs(clean)
	if err != nil {
		return "", false
	}
	realPath := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		realPath = resolved
	}
	for _, root := range allowedRoots {
		cleanRoot := filepath.Clean(root)
		realRoot := cleanRoot
		if resolved, err := filepath.EvalSymlinks(cleanRoot); err == nil {
			realRoot = resolved
		}
		rel, err := filepath.Rel(realRoot, realPath)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
			return realPath, true
		}
	}
	return "", false
}

// isAllowedPath checks if a path is under allowed root directories
func isAllowedPath(p string) bool {
	_, ok := sanitizeAllowedPath(p)
	return ok
}

// hasProjectFiles checks if a directory contains project indicator files
func hasProjectFiles(dir string) bool {
	safeDir, ok := sanitizeAllowedPath(dir)
	if !ok {
		return false
	}
	indicators := []string{
		"package.json", "index.js", "app.js", "server.js", // Node.js
		"index.php", "composer.json", // PHP
		"app.py", "manage.py", "requirements.txt", // Python
		"pom.xml", "build.gradle", // Java
		"go.mod",                  // Go
		"Gemfile",                 // Ruby
		"index.html", "index.htm", // Static
	}
	for _, f := range indicators {
		baseName := filepath.Base(filepath.Clean(f))
		target := filepath.Join(safeDir, baseName)
		rel, err := filepath.Rel(safeDir, target)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			continue
		}
		if fi, err := os.Lstat(target); err == nil && fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// detectProjectType detects the project type in a directory
func detectProjectType(dir string) string {
	safeDir, ok := sanitizeAllowedPath(dir)
	if !ok {
		return ""
	}
	checks := []struct {
		file    string
		project string
	}{
		{"package.json", "nodejs"},
		{"index.php", "php"},
		{"composer.json", "php"},
		{"manage.py", "django"},
		{"app.py", "python"},
		{"requirements.txt", "python"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
		{"go.mod", "go"},
		{"Gemfile", "ruby"},
		{"index.html", "static"},
	}
	for _, c := range checks {
		baseName := filepath.Base(filepath.Clean(c.file))
		target := filepath.Join(safeDir, baseName)
		rel, err := filepath.Rel(safeDir, target)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			continue
		}
		if fi, err := os.Lstat(target); err == nil && fi.Mode().IsRegular() {
			return c.project
		}
	}
	return ""
}

// GetWebsiteSSL returns parsed SSL certificate detail.
func (h *WebServerHandler) GetWebsiteSSL(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	info, err := h.websiteService.GetSSL(c.Request.Context(), sid, id)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// GetWebsiteConfig returns the generated nginx config file content.
func (h *WebServerHandler) GetWebsiteConfig(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	cfg, err := h.websiteService.GetConfig(c.Request.Context(), sid, id)
	if err != nil {
		return nil, err
	}
	return gin.H{"config": cfg}, nil
}

// GetWebsiteParsedLogs returns structured access/error log entries.
func (h *WebServerHandler) GetWebsiteParsedLogs(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	logType := c.DefaultQuery("type", "access")
	lines := httpx.QueryInt(c, "lines", 500)
	if lines <= 0 {
		lines = 500
	}
	entries, err := h.websiteService.GetParsedLogs(c.Request.Context(), sid, id, logType, lines)
	if err != nil {
		return nil, err
	}
	return gin.H{"entries": entries, "type": logType}, nil
}

// GetWebsiteStats returns access-log statistics.
func (h *WebServerHandler) GetWebsiteStats(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	stats, err := h.websiteService.GetStats(c.Request.Context(), sid, id)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// ProbeWebsiteHealth performs an HTTP health probe.
func (h *WebServerHandler) ProbeWebsiteHealth(c *gin.Context) (any, error) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的服务器ID")
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的 ID")
	}
	port := httpx.QueryInt(c, "port", 0)
	res, err := h.websiteService.ProbeHealth(c.Request.Context(), sid, id, port)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// RegisterRoutes registers web server and website management routes
func RegisterRoutes(protected *gin.RouterGroup, webServerService *web.Service, websiteService *web.WebsiteService) {
	handler := NewWebServerHandler(webServerService, websiteService)

	// Utilities (must be before /:id to avoid conflict)
	protected.GET("/web-servers/project-types", httpx.H(handler.GetProjectTypes))
	protected.GET("/web-servers/browse", httpx.H(handler.BrowseDirs))
	protected.GET("/web-servers/validate-path", httpx.H(handler.ValidatePath))

	// Web server CRUD
	protected.GET("/web-servers", httpx.H(handler.List))
	protected.GET("/web-servers/:id", httpx.H(handler.Get))
	protected.POST("/web-servers", httpx.H(handler.Create))
	protected.DELETE("/web-servers/:id", httpx.H(handler.Delete))
	protected.POST("/web-servers/:id/install", httpx.H(handler.Install))
	protected.POST("/web-servers/:id/uninstall", httpx.H(handler.Uninstall))
	protected.POST("/web-servers/:id/start", httpx.H(handler.Start))
	protected.POST("/web-servers/:id/stop", httpx.H(handler.Stop))
	protected.POST("/web-servers/:id/restart", httpx.H(handler.Restart))
	protected.GET("/web-servers/:id/status", httpx.H(handler.Status))
	protected.POST("/web-servers/:id/reload", httpx.H(handler.Reload))
	protected.GET("/web-servers/:id/test-config", httpx.H(handler.TestConfig))
	protected.GET("/web-servers/:id/config", httpx.H(handler.GetConfig))
	protected.PUT("/web-servers/:id/config", httpx.H(handler.SaveConfig))
	protected.GET("/web-servers/:id/logs", httpx.H(handler.GetServiceLogs))
	protected.POST("/web-servers/:id/auto-start", httpx.H(handler.SetAutoStart))
	protected.GET("/web-servers/:id/process", httpx.H(handler.GetProcessInfo))

	// Websites nested under web server (:id = server, :wid = website)
	protected.GET("/web-servers/:id/websites", httpx.H(handler.ListWebsites))
	protected.GET("/web-servers/:id/websites/:wid", httpx.H(handler.GetWebsite))
	protected.POST("/web-servers/:id/websites", httpx.H(handler.CreateWebsite))
	protected.PUT("/web-servers/:id/websites/:wid", httpx.H(handler.UpdateWebsite))
	protected.DELETE("/web-servers/:id/websites/:wid", httpx.H(handler.DeleteWebsite))
	protected.POST("/web-servers/:id/websites/:wid/enable", httpx.H(handler.EnableWebsite))
	protected.POST("/web-servers/:id/websites/:wid/disable", httpx.H(handler.DisableWebsite))
	protected.GET("/web-servers/:id/websites/:wid/logs", httpx.H(handler.GetWebsiteLogs))
	protected.POST("/web-servers/:id/websites/:wid/ssl", httpx.H(handler.ApplyWebsiteSSL))
	protected.POST("/web-servers/:id/websites/:wid/ssl/upload", httpx.H(handler.UploadWebsiteSSL))

	// Website build
	protected.POST("/web-servers/:id/websites/:wid/build", httpx.H(handler.BuildWebsite))

	// Website detail (Drawer)
	protected.GET("/web-servers/:id/websites/:wid/ssl", httpx.H(handler.GetWebsiteSSL))
	protected.GET("/web-servers/:id/websites/:wid/config", httpx.H(handler.GetWebsiteConfig))
	protected.GET("/web-servers/:id/websites/:wid/logs/parse", httpx.H(handler.GetWebsiteParsedLogs))
	protected.GET("/web-servers/:id/websites/:wid/stats", httpx.H(handler.GetWebsiteStats))
	protected.POST("/web-servers/:id/websites/:wid/health/probe", httpx.H(handler.ProbeWebsiteHealth))
}
