package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

func TestRestartPanel_BodyParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	store := config.NewStore(&config.Config{})
	sig := infra.NewSignal()
	h := NewSettingsHandler(store, nil, sig)

	t.Run("empty body succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/settings/restart", bytes.NewBufferString(""))
		c.Request.Header.Set("Content-Type", "application/json")

		res, err := h.RestartPanel(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil response")
		}
	})

	t.Run("valid body succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/settings/restart", bytes.NewBufferString(`{"force":false}`))
		c.Request.Header.Set("Content-Type", "application/json")

		res, err := h.RestartPanel(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil response")
		}
	})

	t.Run("invalid json returns BadRequest", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/settings/restart", bytes.NewBufferString(`{invalid}`))
		c.Request.Header.Set("Content-Type", "application/json")

		_, err := h.RestartPanel(c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})
}
