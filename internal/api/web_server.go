package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/web"

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

func (h *WebServerHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	// Refresh status for all servers
	h.webServerService.RefreshAllStatus(ctx)

	servers, err := h.webServerService.List(ctx)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, servers)
}

func (h *WebServerHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	h.webServerService.RefreshStatus(ctx, id)

	server, err := h.webServerService.Get(ctx, id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if server == nil {
		c.Error(ErrNotFound.WithMessage("Web 服务器不存在"))
		return
	}
	Success(c, server)
}

func (h *WebServerHandler) Create(c *gin.Context) {
	var req model.CreateWebServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate Name format: alphanumeric, hyphen, underscore only
	if !nameRegexp.MatchString(req.Name) {
		c.Error(ErrBadRequest.WithMessage("名称只能包含字母、数字、连字符或下划线"))
		return
	}

	// Validate DisplayName if provided
	if req.DisplayName != "" && strings.TrimSpace(req.DisplayName) == "" {
		c.Error(ErrBadRequest.WithMessage("显示名称不能为空白"))
		return
	}

	// Look up the predefined template — only predefined server types are allowed
	predef := model.FindPredefinedWebServer(req.Name)
	if predef == nil {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("未知的服务器类型 '%s'; 允许的类型: %v", req.Name, model.GetPredefinedWebServerNames())))
		return
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
		c.Error(WrapError(err))
		return
	}
	Success(c, ws)
}

func (h *WebServerHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Delete(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) Install(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Install(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已安装"})
}

func (h *WebServerHandler) Uninstall(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Uninstall(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已卸载"})
}

func (h *WebServerHandler) Start(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Start(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *WebServerHandler) Stop(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Stop(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "stopped"})
}

func (h *WebServerHandler) Restart(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Restart(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *WebServerHandler) Status(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	ctx := c.Request.Context()
	h.webServerService.RefreshStatus(ctx, id)

	server, err := h.webServerService.Get(ctx, id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if server == nil {
		c.Error(ErrNotFound.WithMessage("Web 服务器不存在"))
		return
	}
	Success(c, gin.H{"status": server.Status, "version": server.Version})
}

func (h *WebServerHandler) Reload(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.webServerService.Reload(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已重载"})
}

func (h *WebServerHandler) TestConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	ok, msg := h.webServerService.TestConfig(c.Request.Context(), id)
	Success(c, gin.H{"valid": ok, "message": msg})
}

func (h *WebServerHandler) GetConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	content, err := h.webServerService.GetConfig(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"content": content})
}

func (h *WebServerHandler) SaveConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.webServerService.SaveConfig(c.Request.Context(), id, req.Content); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已保存"})
}

func (h *WebServerHandler) GetServiceLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if lines <= 0 {
		lines = 100
	}

	logs, err := h.webServerService.GetServiceLogs(c.Request.Context(), id, lines)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"logs": logs})
}

func (h *WebServerHandler) SetAutoStart(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.webServerService.SetAutoStart(c.Request.Context(), id, req.Enabled); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"auto_start": req.Enabled})
}

func (h *WebServerHandler) GetProcessInfo(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	pid, mem, uptime, err := h.webServerService.GetProcessInfo(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"pid": pid, "memory_bytes": mem, "uptime": uptime})
}

// Website endpoints (nested under web server)

func (h *WebServerHandler) ListWebsites(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}

	sites, err := h.websiteService.List(c.Request.Context(), sid)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, sites)
}

func (h *WebServerHandler) GetWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	site, err := h.websiteService.Get(c.Request.Context(), sid, id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	if site == nil {
		c.Error(ErrNotFound.WithMessage("网站不存在"))
		return
	}
	Success(c, site)
}

func (h *WebServerHandler) CreateWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}

	var req model.CreateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := req.ValidateDomain(); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	site, err := h.websiteService.Create(c.Request.Context(), sid, &req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, site)
}

func (h *WebServerHandler) UpdateWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req model.UpdateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.websiteService.Update(c.Request.Context(), sid, id, &req); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) DeleteWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.websiteService.Delete(c.Request.Context(), sid, id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) EnableWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.websiteService.Enable(c.Request.Context(), sid, id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "active"})
}

func (h *WebServerHandler) DisableWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	if err := h.websiteService.Disable(c.Request.Context(), sid, id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "disabled"})
}

func (h *WebServerHandler) GetWebsiteLogs(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	logType := c.DefaultQuery("type", "access")
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	if lines <= 0 {
		lines = 200
	}

	logs, err := h.websiteService.GetLogs(c.Request.Context(), sid, id, logType, lines)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"logs": logs, "type": logType})
}

func (h *WebServerHandler) ApplyWebsiteSSL(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	c.ShouldBindJSON(&req)

	if err := h.websiteService.ApplySSL(c.Request.Context(), sid, id, req.Email); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "SSL 证书已应用"})
}

// GetProjectTypes returns available project types
func (h *WebServerHandler) GetProjectTypes(c *gin.Context) {
	Success(c, model.GetProjectTypes())
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

func (h *WebServerHandler) BrowseDirs(c *gin.Context) {
	reqPath := c.DefaultQuery("path", "/var/www")

	// Clean and resolve path
	reqPath = filepath.Clean(reqPath)

	// Security: must be under allowed roots
	if !isAllowedPath(reqPath) {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("路径必须在以下目录下: %s", strings.Join(allowedRoots, ", "))))
		return
	}

	// Check directory exists
	info, err := os.Stat(reqPath)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("目录不存在"))
		return
	}
	if !info.IsDir() {
		c.Error(ErrBadRequest.WithMessage("不是目录"))
		return
	}

	entries, err := os.ReadDir(reqPath)
	if err != nil {
		c.Error(ErrInternal.WithMessage("无法读取目录"))
		return
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

	Success(c, gin.H{
		"current": reqPath,
		"entries": dirs,
	})
}

// ValidatePath validates a root path for website creation
func (h *WebServerHandler) ValidatePath(c *gin.Context) {
	reqPath := c.Query("path")
	if reqPath == "" {
		c.Error(ErrBadRequest.WithMessage("路径不能为空"))
		return
	}

	reqPath = filepath.Clean(reqPath)

	// Security check
	if !isAllowedPath(reqPath) {
		Success(c, gin.H{
			"valid":   false,
			"message": fmt.Sprintf("路径必须在以下目录下: %s", strings.Join(allowedRoots, ", ")),
		})
		return
	}

	// Check if exists
	info, err := os.Stat(reqPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Can be created
			Success(c, gin.H{
				"valid":   true,
				"message": "目录将会被创建",
				"exists":  false,
			})
			return
		}
		Success(c, gin.H{
			"valid":   false,
			"message": "无法访问路径",
		})
		return
	}

	if !info.IsDir() {
		Success(c, gin.H{
			"valid":   false,
			"message": "路径不是目录",
		})
		return
	}

	// Check write permission via file mode bits
	writable := info.Mode().Perm()&0200 != 0
	if writable {
		Success(c, gin.H{
			"valid":    true,
			"message":  "目录已就绪",
			"exists":   true,
			"writable": true,
			"project":  detectProjectType(reqPath),
		})
		return
	}

	// Readable but not writable
	Success(c, gin.H{
		"valid":    true,
		"message":  "目录存在但可能不可写",
		"exists":   true,
		"writable": false,
		"project":  detectProjectType(reqPath),
	})
}

// isAllowedPath checks if a path is under allowed root directories
func isAllowedPath(p string) bool {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	for _, root := range allowedRoots {
		if strings.HasPrefix(absPath, root) {
			return true
		}
	}
	return false
}

// hasProjectFiles checks if a directory contains project indicator files
func hasProjectFiles(dir string) bool {
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
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// detectProjectType detects the project type in a directory
func detectProjectType(dir string) string {
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
		if _, err := os.Stat(filepath.Join(dir, c.file)); err == nil {
			return c.project
		}
	}
	return ""
}
