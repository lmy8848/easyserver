package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"easyserver/internal/domain/ssh"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

func TestGenerateKeyPair_BodyParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := ssh.NewService()
	h := NewSSHHandler(svc)

	t.Run("invalid json returns BadRequest", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/ssh/keys/generate", bytes.NewBufferString(`{invalid}`))
		c.Request.Header.Set("Content-Type", "application/json")

		_, err := h.GenerateKeyPair(c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errx.KindBadRequest) {
			t.Errorf("expected KindBadRequest, got %v", err)
		}
	})

	t.Run("empty body does not fail json parsing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, "/ssh/keys/generate", bytes.NewBufferString(""))
		c.Request.Header.Set("Content-Type", "application/json")

		// It should proceed past JSON parsing to the service logic
		// (which generates an ed25519 key by default or writes to authorized_keys)
		res, err := h.GenerateKeyPair(c)
		if err != nil && errors.Is(err, errx.KindBadRequest) {
			t.Fatalf("did not expect BadRequest on empty body, got: %v", err)
		}
		if err == nil && res == nil {
			t.Fatal("expected non-nil response on success")
		}
	})
}
