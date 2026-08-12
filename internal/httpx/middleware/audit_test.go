package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVerbFor(t *testing.T) {
	cases := []struct {
		method   string
		fullPath string
		want     ActionCategory
	}{
		{"POST", "/api/auth/login", ActionAuth},
		{"POST", "/api/auth/change-password", ActionAuth},
		{"DELETE", "/api/containers/:id", ActionDelete},
		{"DELETE", "/api/db-servers/versions/:vid", ActionDelete},
		{"POST", "/api/runtime/uninstall", ActionDelete},
		{"POST", "/api/containers/:id/exec", ActionExecute},
		{"POST", "/api/db-servers/databases/:did/execute", ActionExecute},
		{"POST", "/api/containers", ActionCreate},           // POST 到无参数集合根
		{"POST", "/api/runtime/install", ActionCreate},      // install 段
		{"POST", "/api/images/pull", ActionCreate},          // pull 段
		{"POST", "/api/containers/:id/start", ActionUpdate}, // 有参数，start 非创建/执行段
		{"POST", "/api/docker/start", ActionUpdate},         // 无参但 start 是状态变更，非创建
		{"POST", "/api/compose/down", ActionUpdate},         // down 是状态变更，非创建
		{"PUT", "/api/containers/:id/update", ActionUpdate},
		{"PATCH", "/api/firewall/rules/:id", ActionUpdate},
		{"GET", "/api/containers", ActionOther}, // GET 不参与写审计，分类兜底
	}
	for _, c := range cases {
		got := verbFor(c.method, c.fullPath)
		if got != c.want {
			t.Errorf("verbFor(%q, %q) = %q, want %q", c.method, c.fullPath, got, c.want)
		}
	}
}

func TestCategoryFor(t *testing.T) {
	cases := []struct {
		path string
		want ResourceCategory
	}{
		{"/api/db-servers/versions/3", ResourceDatabase},
		{"/api/runtime/install", ResourceRuntime},
		{"/api/containers/123/start", ResourceContainer},
		{"/api/docker/start", ResourceContainer},
		{"/api/cron/tasks", ResourceCron},
		{"/api/firewall/rules", ResourceFirewall},
		{"/api/auth/login", ResourceAuth},
		{"/api/unknown-thing", ResourceOther},
	}
	for _, c := range cases {
		got := categoryFor(c.path)
		if got != c.want {
			t.Errorf("categoryFor(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

type mockRequestLogger struct {
	mu            sync.Mutex
	requestLogs   []reqLogEntry
	operationLogs []opLogEntry
}

type reqLogEntry struct {
	userID                                            int64
	username, action, resource, detail, ip, userAgent string
}

type opLogEntry struct {
	userID                     int64
	username, action, resource string
	extra                      map[string]any
	ip, userAgent              string
}

func (m *mockRequestLogger) LogRequest(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLogs = append(m.requestLogs, reqLogEntry{userID, username, action, resource, detail, ip, userAgent})
}

func (m *mockRequestLogger) LogOperation(ctx context.Context, userID int64, username, action, resource string, extra map[string]any, ip, userAgent string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationLogs = append(m.operationLogs, opLogEntry{userID, username, action, resource, extra, ip, userAgent})
}

func TestAuditMiddleware_OperationLoggedWhenSummarySet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := &mockRequestLogger{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("username", "admin")
		c.Next()
	}, AuditMiddleware(logger))
	r.POST("/api/containers/:id", func(c *gin.Context) {
		AuditSummary(c, "删除容器 nginx-web")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/containers/5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if len(logger.requestLogs) != 1 {
		t.Fatalf("expected 1 request log, got %d", len(logger.requestLogs))
	}
	if len(logger.operationLogs) != 1 {
		t.Fatalf("expected 1 operation log, got %d", len(logger.operationLogs))
	}

	op := logger.operationLogs[0]
	if op.action != "修改" {
		t.Errorf("operation action = %q, want 修改", op.action)
	}
	if op.resource != "容器" {
		t.Errorf("operation resource = %q, want 容器", op.resource)
	}
	if op.extra["summary"] != "删除容器 nginx-web" {
		t.Errorf("summary = %v, want 删除容器 nginx-web", op.extra["summary"])
	}
	if op.extra["success"] != true {
		t.Errorf("success = %v, want true", op.extra["success"])
	}
}

func TestAuditMiddleware_RequestOnlyWhenNoSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := &mockRequestLogger{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("username", "admin")
		c.Next()
	}, AuditMiddleware(logger))
	r.POST("/api/containers/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/containers/5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if len(logger.requestLogs) != 1 {
		t.Fatalf("expected 1 request log, got %d", len(logger.requestLogs))
	}
	if len(logger.operationLogs) != 0 {
		t.Fatalf("expected 0 operation logs, got %d", len(logger.operationLogs))
	}

	reqDetail := logger.requestLogs[0].detail
	var d map[string]any
	if err := json.Unmarshal([]byte(reqDetail), &d); err != nil {
		t.Fatal(err)
	}
	if d["status"] != float64(200) {
		t.Errorf("request detail status = %v, want 200", d["status"])
	}
}

func TestAuditMiddleware_SkipsGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := &mockRequestLogger{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Next()
	}, AuditMiddleware(logger))
	r.GET("/api/containers", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/containers", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if len(logger.requestLogs) != 0 {
		t.Fatalf("expected 0 request logs for GET, got %d", len(logger.requestLogs))
	}
	if len(logger.operationLogs) != 0 {
		t.Fatalf("expected 0 operation logs for GET, got %d", len(logger.operationLogs))
	}
}
