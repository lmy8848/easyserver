package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapKind(t *testing.T) {
	tests := []struct {
		kind       errx.Kind
		wantStatus int
		wantCode   int
	}{
		{errx.KindBadRequest, http.StatusBadRequest, apperror.CodeBadRequest},
		{errx.KindUnauthorized, http.StatusUnauthorized, apperror.CodeUnauthorized},
		{errx.KindForbidden, http.StatusForbidden, apperror.CodeForbidden},
		{errx.KindNotFound, http.StatusNotFound, apperror.CodeNotFound},
		{errx.KindConflict, http.StatusConflict, apperror.CodeConflict},
		{errx.KindRateLimit, http.StatusTooManyRequests, apperror.CodeRateLimit},
		{errx.KindInternal, http.StatusInternalServerError, apperror.CodeInternalError},
		{errx.Kind(99), http.StatusInternalServerError, apperror.CodeInternalError},
	}

	for _, tt := range tests {
		status, code := mapKind(tt.kind)
		assert.Equal(t, tt.wantStatus, status, "kind %v status mismatch", tt.kind)
		assert.Equal(t, tt.wantCode, code, "kind %v code mismatch", tt.kind)
	}
}

func setupErrorTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler())
	return r
}

func TestErrorHandler_DualChannel(t *testing.T) {
	r := setupErrorTestRouter()

	// 1. errx channel
	r.GET("/test-errx-notfound", func(c *gin.Context) {
		c.Error(errx.NotFound("资源 %s 不存在", "user-1"))
	})

	r.GET("/test-errx-sentinel", func(c *gin.Context) {
		c.Error(errx.ErrDockerNotInstalled)
	})

	// 2. apperror legacy channel
	r.GET("/test-apperror-conflict", func(c *gin.Context) {
		c.Error(apperror.ErrConflict.WithMessage("邮箱已存在"))
	})

	// 3. unknown generic error
	r.GET("/test-unknown-error", func(c *gin.Context) {
		c.Error(errors.New("raw db connection broke"))
	})

	t.Run("errx.NotFound", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-errx-notfound", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp errorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 40400, resp.Code)
		assert.Equal(t, "资源 user-1 不存在", resp.Message)
		assert.Nil(t, resp.Data)
	})

	t.Run("errx.ErrDockerNotInstalled (sentinel code override)", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-errx-sentinel", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var resp errorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 50001, resp.Code)
		assert.Equal(t, "Docker 未安装或未启动", resp.Message)
	})

	t.Run("apperror.ErrConflict (legacy fallback)", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-apperror-conflict", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
		var resp errorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 40900, resp.Code)
		assert.Equal(t, "邮箱已存在", resp.Message)
	})

	t.Run("unknown generic error (500 fallback)", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-unknown-error", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var resp errorResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 50000, resp.Code)
		assert.Equal(t, "internal server error", resp.Message)
	})
}
