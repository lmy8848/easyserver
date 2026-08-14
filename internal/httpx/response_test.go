package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestH_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())

	r.GET("/test-success", H(func(c *gin.Context) (any, error) {
		return gin.H{"id": 1, "name": "test"}, nil
	}))

	r.GET("/test-paginated", H(func(c *gin.Context) (any, error) {
		return PaginatedData{
			Total: 100,
			Items: []string{"a", "b"},
		}, nil
	}))

	t.Run("normal success returns {code:0, message:ok, data}", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-success", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Code)
		assert.Equal(t, "ok", resp.Message)
		dataMap, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 1, dataMap["id"])
		assert.Equal(t, "test", dataMap["name"])
	})

	t.Run("paginated data serialization matches wire format", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-paginated", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Code)
		dataMap, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 100, dataMap["total"])
	})
}

func TestH_ErrorAndAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())

	r.GET("/test-error", H(func(c *gin.Context) (any, error) {
		return nil, errx.Forbidden("无权访问该资源")
	}))

	r.GET("/test-written-skip", H(func(c *gin.Context) (any, error) {
		c.String(http.StatusOK, "raw bytes stream")
		return nil, nil
	}))

	t.Run("returns error via ErrorHandler", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-error", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		var resp Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 40300, resp.Code)
		assert.Equal(t, "无权访问该资源", resp.Message)
	})

	t.Run("written skips double serialization", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-written-skip", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "raw bytes stream", w.Body.String())
	})
}
