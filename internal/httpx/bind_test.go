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
	Name string `json:"name" form:"name" uri:"name" binding:"required"`
	Age  int    `json:"age" form:"age" uri:"age"`
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

func TestBindOptionalJSON(t *testing.T) {
	ctx := context.Background()

	t.Run("valid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(`{"name":"alice","age":30}`))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := BindOptionalJSON[sampleReq](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Name != "alice" || req.Age != 30 {
			t.Errorf("got %+v, want alice/30", req)
		}
	})

	t.Run("empty body returns zero value and nil error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(""))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := BindOptionalJSON[sampleReq](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Name != "" || req.Age != 0 {
			t.Errorf("got %+v, want zero value", req)
		}
	})

	t.Run("invalid json returns BadRequest", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewBufferString(`{invalid}`))
		c.Request.Header.Set("Content-Type", "application/json")

		_, err := BindOptionalJSON[sampleReq](c)
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

func TestBindURI(t *testing.T) {
	ctx := context.Background()

	t.Run("valid uri parameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/users/charlie/28", nil)
		c.Params = gin.Params{
			{Key: "name", Value: "charlie"},
			{Key: "age", Value: "28"},
		}

		req, err := BindURI[sampleReq](c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Name != "charlie" || req.Age != 28 {
			t.Errorf("got %+v, want charlie/28", req)
		}
	})

	t.Run("missing required uri parameter", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/users", nil)
		c.Params = gin.Params{
			{Key: "age", Value: "28"},
		}

		_, err := BindURI[sampleReq](c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})

	t.Run("invalid uri parameter type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/users/charlie/not-an-int", nil)
		c.Params = gin.Params{
			{Key: "name", Value: "charlie"},
			{Key: "age", Value: "not-an-int"},
		}

		_, err := BindURI[sampleReq](c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})
}
