package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewareGeneratesWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)

	rid := w.Header().Get("X-Request-ID")
	if len(rid) != 32 {
		t.Fatalf("生成的 request_id 应为 32 位 hex，实际 %q", rid)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(rid) {
		t.Fatalf("request_id 应为纯 hex，实际 %q", rid)
	}
}

func TestRequestIDMiddlewarePassesThroughValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "abc123_XYZ.01")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "abc123_XYZ.01" {
		t.Fatalf("合法上游 ID 应透传，实际 %q", got)
	}
}

func TestRequestIDMiddlewareRejectsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 含换行的注入载荷与超长 ID 都应被替换为新生成的 ID
	for _, bad := range []string{"id\r\nX-Evil: 1", string(make([]byte, 200))} {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
		req.Header.Set("X-Request-ID", bad)
		r.ServeHTTP(w, req)

		got := w.Header().Get("X-Request-ID")
		if got == bad {
			t.Fatalf("非法 ID 不应透传: %q", got)
		}
		if len(got) != 32 {
			t.Fatalf("被替换后应为 32 位 hex，实际 %q", got)
		}
	}
}
