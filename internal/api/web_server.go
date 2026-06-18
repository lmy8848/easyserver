package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type WebServerHandler struct {
	webServerService *service.WebServerService
	websiteService   *service.WebsiteService
}

func NewWebServerHandler(webServerService *service.WebServerService, websiteService *service.WebsiteService) *WebServerHandler {
	return &WebServerHandler{
		webServerService: webServerService,
		websiteService:   websiteService,
	}
}

// Web Server endpoints

func (h *WebServerHandler) List(c *gin.Context) {
	// Refresh status for all servers
	h.webServerService.RefreshAllStatus()

	servers, err := h.webServerService.List()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, servers)
}

func (h *WebServerHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	h.webServerService.RefreshStatus(id)

	server, err := h.webServerService.Get(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if server == nil {
		NotFound(c, "web server not found")
		return
	}
	Success(c, server)
}

func (h *WebServerHandler) Create(c *gin.Context) {
	var ws model.WebServer
	if err := c.ShouldBindJSON(&ws); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.webServerService.Create(&ws); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, ws)
}

func (h *WebServerHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Delete(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) Install(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Install(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "installed"})
}

func (h *WebServerHandler) Uninstall(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Uninstall(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "uninstalled"})
}

func (h *WebServerHandler) Start(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Start(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *WebServerHandler) Stop(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Stop(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "stopped"})
}

func (h *WebServerHandler) Restart(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Restart(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *WebServerHandler) Status(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	h.webServerService.RefreshStatus(id)

	server, err := h.webServerService.Get(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if server == nil {
		NotFound(c, "web server not found")
		return
	}
	Success(c, gin.H{"status": server.Status, "version": server.Version})
}

func (h *WebServerHandler) Reload(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.webServerService.Reload(id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "reloaded"})
}

func (h *WebServerHandler) TestConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	ok, msg := h.webServerService.TestConfig(id)
	Success(c, gin.H{"valid": ok, "message": msg})
}

func (h *WebServerHandler) GetConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	content, err := h.webServerService.GetConfig(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"content": content})
}

func (h *WebServerHandler) SaveConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.webServerService.SaveConfig(id, req.Content); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "saved"})
}

func (h *WebServerHandler) GetServiceLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if lines <= 0 {
		lines = 100
	}

	logs, err := h.webServerService.GetServiceLogs(id, lines)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"logs": logs})
}

func (h *WebServerHandler) SetAutoStart(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.webServerService.SetAutoStart(id, req.Enabled); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"auto_start": req.Enabled})
}

func (h *WebServerHandler) GetProcessInfo(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	pid, mem, uptime, err := h.webServerService.GetProcessInfo(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"pid": pid, "memory_bytes": mem, "uptime": uptime})
}

// Website endpoints (nested under web server)

func (h *WebServerHandler) ListWebsites(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	sites, err := h.websiteService.List(sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, sites)
}

func (h *WebServerHandler) GetWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	site, err := h.websiteService.Get(sid, id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if site == nil {
		NotFound(c, "website not found")
		return
	}
	Success(c, site)
}

func (h *WebServerHandler) CreateWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	var req model.CreateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	site, err := h.websiteService.Create(sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, site)
}

func (h *WebServerHandler) UpdateWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	var req model.UpdateWebsiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.websiteService.Update(sid, id, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) DeleteWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.websiteService.Delete(sid, id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

func (h *WebServerHandler) EnableWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.websiteService.Enable(sid, id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "active"})
}

func (h *WebServerHandler) DisableWebsite(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	if err := h.websiteService.Disable(sid, id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "disabled"})
}

func (h *WebServerHandler) GetWebsiteLogs(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	logType := c.DefaultQuery("type", "access")
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	if lines <= 0 {
		lines = 200
	}

	logs, err := h.websiteService.GetLogs(sid, id, logType, lines)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"logs": logs, "type": logType})
}

func (h *WebServerHandler) ApplyWebsiteSSL(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	id, err := strconv.ParseInt(c.Param("wid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	c.ShouldBindJSON(&req)

	if err := h.websiteService.ApplySSL(sid, id, req.Email); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "SSL certificate applied"})
}

// GetProjectTypes returns available project types
func (h *WebServerHandler) GetProjectTypes(c *gin.Context) {
	Success(c, model.GetProjectTypes())
}

// Directory browser

// allowedRoots defines safe base directories for website root paths
var allowedRoots = []string{"/var/www", "/home", "/opt", "/srv", "/usr/share"}

type DirEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	HasItems bool   `json:"has_items"` // has package.json, index.php, etc.
	Project  string `json:"project"`  // detected project type
}

func (h *WebServerHandler) BrowseDirs(c *gin.Context) {
	reqPath := c.DefaultQuery("path", "/var/www")

	// Clean and resolve path
	reqPath = filepath.Clean(reqPath)

	// Security: must be under allowed roots
	if !isAllowedPath(reqPath) {
		BadRequest(c, fmt.Sprintf("path must be under: %s", strings.Join(allowedRoots, ", ")))
		return
	}

	// Check directory exists
	info, err := os.Stat(reqPath)
	if err != nil {
		NotFound(c, "directory not found")
		return
	}
	if !info.IsDir() {
		BadRequest(c, "not a directory")
		return
	}

	entries, err := os.ReadDir(reqPath)
	if err != nil {
		InternalError(c, "cannot read directory")
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
		BadRequest(c, "path is required")
		return
	}

	reqPath = filepath.Clean(reqPath)

	// Security check
	if !isAllowedPath(reqPath) {
		Success(c, gin.H{
			"valid":   false,
			"message": fmt.Sprintf("path must be under: %s", strings.Join(allowedRoots, ", ")),
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
				"message": "directory will be created",
				"exists":  false,
			})
			return
		}
		Success(c, gin.H{
			"valid":   false,
			"message": "cannot access path",
		})
		return
	}

	if !info.IsDir() {
		Success(c, gin.H{
			"valid":   false,
			"message": "path is not a directory",
		})
		return
	}

	// Check readable
	testFile := filepath.Join(reqPath, ".easyserver_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
		os.Remove(testFile)
		Success(c, gin.H{
			"valid":    true,
			"message":  "directory is ready",
			"exists":   true,
			"writable": true,
			"project":  detectProjectType(reqPath),
		})
		return
	}

	// Readable but not writable
	Success(c, gin.H{
		"valid":    true,
		"message":  "directory exists but may not be writable",
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
		"go.mod", // Go
		"Gemfile", // Ruby
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
