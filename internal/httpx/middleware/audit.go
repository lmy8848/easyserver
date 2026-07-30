package middleware

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ActionCategory string
type ResourceCategory string

const (
	ActionCreate  ActionCategory = "创建"
	ActionDelete  ActionCategory = "删除"
	ActionUpdate  ActionCategory = "修改"
	ActionExecute ActionCategory = "执行"
	ActionAuth    ActionCategory = "认证"
	ActionOther   ActionCategory = "其他"

	ResourceDatabase     ResourceCategory = "数据库"
	ResourceRuntime      ResourceCategory = "运行环境"
	ResourcePackage      ResourceCategory = "软件包"
	ResourceContainer    ResourceCategory = "容器"
	ResourceCloud        ResourceCategory = "云服务器"
	ResourceCron         ResourceCategory = "定时任务"
	ResourceFirewall     ResourceCategory = "防火墙"
	ResourceSSH          ResourceCategory = "SSH"
	ResourceTerminal     ResourceCategory = "终端"
	ResourceDaemon       ResourceCategory = "守护进程"
	ResourceFile         ResourceCategory = "文件"
	ResourceWebsite      ResourceCategory = "网站"
	ResourceWebServer    ResourceCategory = "Web服务"
	ResourceDeploy       ResourceCategory = "发布"
	ResourceSetting      ResourceCategory = "面板设置"
	ResourceEnvVar       ResourceCategory = "环境变量"
	ResourceNotification ResourceCategory = "通知"
	ResourceAudit        ResourceCategory = "审计"
	ResourceSystem       ResourceCategory = "系统服务"
	ResourceAuth         ResourceCategory = "认证"
	ResourceOther        ResourceCategory = "其他"
)

// RequestLogger is the audit capability needed by the HTTP middleware.
// *audit.Service satisfies this interface implicitly.
type RequestLogger interface {
	LogRequest(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string)
	LogOperation(ctx context.Context, userID int64, username, action, resource string, extra map[string]interface{}, ip, userAgent string)
}

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
func categoryFor(path string) ResourceCategory {
	for _, p := range categoryPrefixes {
		if strings.HasPrefix(path, p.prefix) {
			return p.category
		}
	}
	return ResourceOther
}

var categoryPrefixes = []struct {
	prefix   string
	category ResourceCategory
}{
	{"/api/db-servers", ResourceDatabase},
	{"/api/runtime", ResourceRuntime},
	{"/api/packages", ResourcePackage},
	{"/api/containers", ResourceContainer},
	{"/api/docker", ResourceContainer},
	{"/api/images", ResourceContainer},
	{"/api/compose", ResourceContainer},
	{"/api/volumes", ResourceContainer},
	{"/api/networks", ResourceContainer},
	{"/api/cloud", ResourceCloud},
	{"/api/cron", ResourceCron},
	{"/api/firewall", ResourceFirewall},
	{"/api/ssh", ResourceSSH},
	{"/api/terminal", ResourceTerminal},
	{"/api/process", ResourceDaemon},
	{"/api/files", ResourceFile},
	{"/api/websites", ResourceWebsite},
	{"/api/web-servers", ResourceWebServer},
	{"/api/deploy", ResourceDeploy},
	{"/api/settings", ResourceSetting},
	{"/api/env-config", ResourceEnvVar},
	{"/api/notifications", ResourceNotification},
	{"/api/audit-logs", ResourceAudit},
	{"/api/systemd", ResourceSystem},
	{"/api/services", ResourceSystem},
	{"/api/auth", ResourceAuth},
}

// verbFor maps method + route template to one of six coarse verbs:
// 创建/删除/修改/执行/认证/其他. Route params appear as ":id" in FullPath.
func verbFor(method, fullPath string) ActionCategory {
	if strings.HasPrefix(fullPath, "/api/auth") {
		return ActionAuth
	}
	segs := strings.Split(strings.TrimPrefix(fullPath, "/api/"), "/")
	for _, s := range segs {
		if execSegs[s] {
			return ActionExecute
		}
	}
	// State-change verbs (start/stop/restart/...) before create heuristics, so
	// POST /docker/start (no param) is not misclassified as 创建.
	for _, s := range segs {
		if controlSegs[s] {
			return ActionUpdate
		}
	}
	if method == "DELETE" {
		return ActionDelete
	}
	for _, s := range segs {
		if deleteSegs[s] {
			return ActionDelete
		}
	}
	for _, s := range segs {
		if createSegs[s] {
			return ActionCreate
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
			return ActionCreate
		}
	}
	if method == "POST" || method == "PUT" || method == "PATCH" {
		return ActionUpdate
	}
	return ActionOther
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

var deleteSegs = map[string]bool{
	"uninstall": true, "remove": true, "clear": true, "prune": true,
	"revoke": true, "delete": true, "drop": true, "destroy": true,
}

// AuditMiddleware logs every non-GET request (request log) and, when the handler
// declared an AuditSummary, an operation log on top.
func AuditMiddleware(logger RequestLogger) gin.HandlerFunc {
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
		logger.LogRequest(ctx, uid, uname, c.Request.Method, path, string(reqDetail), ip, ua)

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
			logger.LogOperation(ctx, uid, uname, string(verb), string(categoryFor(path)), extra, "", "")
		}
	}
}
