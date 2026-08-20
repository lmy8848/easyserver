package httpx

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type sampleReq struct {
	Name string `json:"name" form:"name" binding:"required"`
	Age  int    `json:"age" form:"age"`
}

func init() {
	gin.SetMode(gin.TestMode)
}

func TestBindJSON(t *testing.T) {
	ctx := context.Background()

	t.Run("valid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(`{"name":"alice","age":30}`))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := BindJSON[sampleReq](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Name != "alice" || req.Age != 30 {
			t.Errorf("got %+v, want alice/30", req)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(`{invalid}`))
		c.Request.Header.Set("Content-Type", "application/json")

		_, err := BindJSON[sampleReq](c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(`{"age":30}`))
		c.Request.Header.Set("Content-Type", "application/json")

		_, err := BindJSON[sampleReq](c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})
}

func TestBindQuery(t *testing.T) {
	ctx := context.Background()

	t.Run("valid query", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/?name=bob&age=25", nil)

		req, err := BindQuery[sampleReq](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Name != "bob" || req.Age != 25 {
			t.Errorf("got %+v, want bob/25", req)
		}
	})

	t.Run("missing required query", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/?age=25", nil)

		_, err := BindQuery[sampleReq](c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})
}
