package middleware

import (
	"encoding/json"
	"strings"
	"time"

	"easyserver/internal/audit"

	"github.com/gin-gonic/gin"
)

// AuditSummary lets a handler declare the human-readable summary of what it did.
// When set, the audit middleware records an operation log on top of the request log.
func AuditSummary(c *gin.Context, summary string) {
	c.Set("audit_summary", summary)
}

// AuditDetail attaches a structured key/value to the operation log detail.
func AuditDetail(c *gin.Context, key string, value any) {
	v, _ := c.Get("audit_detail")
	m, _ := v.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	m[key] = value
	c.Set("audit_detail", m)
}

// categoryFor derives a coarse resource category from the URL path (longest-prefix match).
func categoryFor(path string) string {
	for _, p := range categoryPrefixes {
		if strings.HasPrefix(path, p.prefix) {
			return p.category
		}
	}
	return "其他"
}

var categoryPrefixes = []struct{ prefix, category string }{
	{"/api/db-servers", "数据库"},
	{"/api/runtime", "运行环境"},
	{"/api/packages", "软件包"},
	{"/api/containers", "容器"},
	{"/api/docker", "容器"},
	{"/api/images", "容器"},
	{"/api/compose", "容器"},
	{"/api/volumes", "容器"},
	{"/api/networks", "容器"},
	{"/api/cloud", "云服务器"},
	{"/api/cron", "定时任务"},
	{"/api/firewall", "防火墙"},
	{"/api/ssh", "SSH"},
	{"/api/terminal", "终端"},
	{"/api/process", "守护进程"},
	{"/api/files", "文件"},
	{"/api/websites", "网站"},
	{"/api/web-servers", "Web服务"},
	{"/api/deploy", "发布"},
	{"/api/settings", "面板设置"},
	{"/api/env-config", "环境变量"},
	{"/api/global-config", "环境变量"},
	{"/api/notifications", "通知"},
	{"/api/audit-logs", "审计"},
	{"/api/systemd", "系统服务"},
	{"/api/services", "系统服务"},
	{"/api/auth", "认证"},
}

// verbFor maps method + route template to one of six coarse verbs:
// 创建/删除/修改/执行/认证/其他. Route params appear as ":id" in FullPath.
func verbFor(method, fullPath string) string {
	if strings.HasPrefix(fullPath, "/api/auth") {
		return "认证"
	}
	segs := strings.Split(strings.TrimPrefix(fullPath, "/api/"), "/")
	for _, s := range segs {
		if execSegs[s] {
			return "执行"
		}
	}
	// State-change verbs (start/stop/restart/...) before create heuristics, so
	// POST /docker/start (no param) is not misclassified as 创建.
	for _, s := range segs {
		if controlSegs[s] {
			return "修改"
		}
	}
	if method == "DELETE" {
		return "删除"
	}
	for _, s := range segs {
		if createSegs[s] {
			return "创建"
		}
	}
	if method == "POST" {
		hasParam := false
		for _, s := range segs {
			if strings.HasPrefix(s, ":") {
				hasParam = true
				break
			}
		}
		if !hasParam {
			return "创建"
		}
	}
	if method == "POST" || method == "PUT" || method == "PATCH" {
		return "修改"
	}
	return "其他"
}

var execSegs = map[string]bool{
	"exec": true, "execute": true, "query": true,
	"copy-to": true, "copy-from": true, "run": true,
}

var createSegs = map[string]bool{
	"create": true, "install": true, "pull": true, "up": true,
	"new": true, "add": true, "upload": true, "mkdir": true,
	"import": true, "apply": true, "grant": true,
}

// controlSegs are state-change verbs that mutate an existing resource's state
// (start/stop/restart/...) rather than creating one — classified as 修改.
var controlSegs = map[string]bool{
	"start": true, "stop": true, "restart": true, "pause": true,
	"unpause": true, "down": true, "reload": true, "enable": true,
	"disable": true, "restore": true,
}

// AuditMiddleware logs every non-GET request (request log) and, when the handler
// declared an AuditSummary, an operation log on top.
func AuditMiddleware(auditService *audit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// GET requests are not audited (read-only).
		if c.Request.Method == "GET" {
			return
		}

		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		var uid int64
		var uname string
		if v, ok := userID.(int64); ok {
			uid = v
		}
		if v, ok := username.(string); ok {
			uname = v
		}

		status := c.Writer.Status()
		ip := c.ClientIP()
		ua := c.Request.UserAgent()
		path := c.Request.URL.Path
		ctx := c.Request.Context()

		// 1. Request log: every non-GET request. action = HTTP method, resource = path.
		// detail is a single flat JSON layer so Stats/alerts can json_extract('$.status') directly.
		reqDetail, _ := json.Marshal(map[string]any{
			"method":      c.Request.Method,
			"path":        path,
			"status":      status,
			"duration_ms": time.Since(start).Milliseconds(),
			"timestamp":   time.Now().Format(time.RFC3339),
		})
		auditService.LogRequest(ctx, uid, uname, c.Request.Method, path, string(reqDetail), ip, ua)

		// 2. Operation log: only when the handler declared a summary.
		if summary, ok := c.Get("audit_summary"); ok {
			s, _ := summary.(string)
			extra := map[string]any{
				"summary": s,
				"success": status < 400 && len(c.Errors) == 0,
			}
			if d, ok := c.Get("audit_detail"); ok {
				if dm, ok := d.(map[string]any); ok {
					for k, v := range dm {
						extra[k] = v
					}
				}
			}
			if status >= 400 || len(c.Errors) > 0 {
				extra["error"] = c.Errors.String()
			}
			verb := verbFor(c.Request.Method, c.FullPath())
			// Operation logs do not record IP/user-agent (request-log concern); pass empty.
			auditService.LogOperation(ctx, uid, uname, verb, categoryFor(path), extra, "", "")
		}
	}
}
