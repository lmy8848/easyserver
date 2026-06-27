package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"time"

	"easyserver/internal/audit"

	"github.com/gin-gonic/gin"
)

// maxAuditBodySize is the maximum request body size read for audit logging (64KB)
const maxAuditBodySize = 64 * 1024

// AuditMiddleware logs non-GET requests using the batched writer
func AuditMiddleware(auditService *audit.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Read request body for logging (limited to prevent OOM on large uploads)
		var bodyBytes []byte
		if c.Request.Body != nil {
			var readErr error
			bodyBytes, readErr = io.ReadAll(io.LimitReader(c.Request.Body, maxAuditBodySize))
			if readErr != nil {
				log.Printf("audit: failed to read request body: %v", readErr)
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		c.Next()

		// Skip audit for GET requests (except login attempts)
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

		detail := map[string]interface{}{
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"status":      c.Writer.Status(),
			"duration_ms": time.Since(start).Milliseconds(),
		}
		detailJSON, _ := json.Marshal(detail)

		auditService.LogOperation(c.Request.Context(), uid, uname, c.Request.Method, c.Request.URL.Path, string(detailJSON), c.ClientIP(), c.Request.UserAgent())
	}
}
