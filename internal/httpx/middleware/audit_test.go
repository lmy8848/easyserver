package middleware

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/audit"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestVerbFor(t *testing.T) {
	cases := []struct {
		method   string
		fullPath string
		want     audit.ActionCategory
	}{
		{"POST", "/api/auth/login", audit.ActionAuth},
		{"POST", "/api/auth/change-password", audit.ActionAuth},
		{"DELETE", "/api/containers/:id", audit.ActionDelete},
		{"DELETE", "/api/db-servers/versions/:vid", audit.ActionDelete},
		{"POST", "/api/runtime/uninstall", audit.ActionDelete},
		{"POST", "/api/containers/:id/exec", audit.ActionExecute},
		{"POST", "/api/db-servers/databases/:did/execute", audit.ActionExecute},
		{"POST", "/api/containers", audit.ActionCreate},           // POST 到无参数集合根
		{"POST", "/api/runtime/install", audit.ActionCreate},      // install 段
		{"POST", "/api/images/pull", audit.ActionCreate},          // pull 段
		{"POST", "/api/containers/:id/start", audit.ActionUpdate}, // 有参数，start 非创建/执行段
		{"POST", "/api/docker/start", audit.ActionUpdate},         // 无参但 start 是状态变更，非创建
		{"POST", "/api/compose/down", audit.ActionUpdate},         // down 是状态变更，非创建
		{"PUT", "/api/containers/:id/update", audit.ActionUpdate},
		{"PATCH", "/api/firewall/rules/:id", audit.ActionUpdate},
		{"GET", "/api/containers", audit.ActionOther}, // GET 不参与写审计，分类兜底
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
		want audit.ResourceCategory
	}{
		{"/api/db-servers/versions/3", audit.ResourceDatabase},
		{"/api/runtime/install", audit.ResourceRuntime},
		{"/api/containers/123/start", audit.ResourceContainer},
		{"/api/docker/start", audit.ResourceContainer},
		{"/api/cron/tasks", audit.ResourceCron},
		{"/api/firewall/rules", audit.ResourceFirewall},
		{"/api/auth/login", audit.ResourceAuth},
		{"/api/unknown-thing", audit.ResourceOther},
	}
	for _, c := range cases {
		got := categoryFor(c.path)
		if got != c.want {
			t.Errorf("categoryFor(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// newTestService builds a real audit.Service over an in-memory sqlite audit_logs table.
func newTestService(t *testing.T) (*audit.Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER DEFAULT 0,
		username TEXT NOT NULL,
		action TEXT NOT NULL,
		resource TEXT DEFAULT '',
		detail TEXT DEFAULT '',
		ip TEXT DEFAULT '',
		user_agent TEXT DEFAULT '',
		type TEXT NOT NULL DEFAULT 'operation',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	return audit.NewService(db, audit.NewSQLiteRepository(db), 90), db
}

// TestAuditMiddleware_OperationLoggedWhenSummarySet: a POST whose handler sets AuditSummary
// records both a request log and an operation log with the derived verb/category.
func TestAuditMiddleware_OperationLoggedWhenSummarySet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := newTestService(t)
	defer db.Close()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("username", "admin")
		c.Next()
	}, AuditMiddleware(svc))
	r.POST("/api/containers/:id", func(c *gin.Context) {
		AuditSummary(c, "删除容器 nginx-web")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/containers/5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	svc.Close()

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 logs (request + operation), got %d", n)
	}

	var opAction, opResource, detail string
	err := db.QueryRow("SELECT action, resource, detail FROM audit_logs WHERE type='operation'").
		Scan(&opAction, &opResource, &detail)
	if err != nil {
		t.Fatal(err)
	}
	// POST /api/containers/:id has a param and no exec/create segment -> 修改
	if opAction != "修改" {
		t.Errorf("operation action = %q, want 修改", opAction)
	}
	if opResource != "容器" {
		t.Errorf("operation resource = %q, want 容器", opResource)
	}
	var d map[string]interface{}
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		t.Fatal(err)
	}
	if d["summary"] != "删除容器 nginx-web" {
		t.Errorf("summary = %v, want 删除容器 nginx-web", d["summary"])
	}
	if d["success"] != true {
		t.Errorf("success = %v, want true", d["success"])
	}
}

// TestAuditMiddleware_RequestOnlyWhenNoSummary: a POST whose handler sets no summary
// records only the request log, no operation log.
func TestAuditMiddleware_RequestOnlyWhenNoSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := newTestService(t)
	defer db.Close()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("username", "admin")
		c.Next()
	}, AuditMiddleware(svc))
	r.POST("/api/containers/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/containers/5", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	svc.Close()

	var n, opCount int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&n)
	db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE type='operation'").Scan(&opCount)
	if n != 1 {
		t.Fatalf("expected 1 request log only, got %d", n)
	}
	if opCount != 0 {
		t.Fatalf("expected 0 operation logs, got %d", opCount)
	}

	// request detail must store status at the top level so Stats/alerts
	// (json_extract(detail,'$.status')) can read it directly — not nested under "detail".
	var reqStatus int
	if err := db.QueryRow(`SELECT CAST(json_extract(detail,'$.status') AS INTEGER) FROM audit_logs WHERE type='request'`).Scan(&reqStatus); err != nil {
		t.Fatal(err)
	}
	if reqStatus != 200 {
		t.Errorf("request detail $.status = %d, want 200 (must be top-level)", reqStatus)
	}
}

// TestAuditMiddleware_SkipsGET: GET requests are not audited at all.
func TestAuditMiddleware_SkipsGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := newTestService(t)
	defer db.Close()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Next()
	}, AuditMiddleware(svc))
	r.GET("/api/containers", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/containers", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	svc.Close()

	var n int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&n)
	if n != 0 {
		t.Fatalf("expected 0 logs for GET, got %d", n)
	}
}
