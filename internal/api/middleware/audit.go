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

// SetAction is a declarative middleware to set the operation action name
func SetAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("audit_action", action)
		c.Next()
	}
}

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

			// Restore the body: combine the bytes we just read with the rest of the unread body
			c.Request.Body = struct {
				io.Reader
				io.Closer
			}{
				io.MultiReader(bytes.NewReader(bodyBytes), c.Request.Body),
				c.Request.Body,
			}
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

		// 1. Log Request
		detail := map[string]interface{}{
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"status":      c.Writer.Status(),
			"duration_ms": time.Since(start).Milliseconds(),
		}
		detailJSON, _ := json.Marshal(detail)
		auditService.LogRequest(c.Request.Context(), uid, uname, c.Request.Method, c.Request.URL.Path, string(detailJSON), c.ClientIP(), c.Request.UserAgent())

		// 2. Declarative Operation Log
		action := c.GetString("audit_action")
		if action != "" {
			extra := make(map[string]interface{})
			if len(bodyBytes) > 0 {
				_ = json.Unmarshal(bodyBytes, &extra)
			}
			for _, p := range c.Params {
				extra[p.Key] = p.Value
			}

			status := c.Writer.Status()
			extra["status"] = status

			// Record success/failure explicitly based on HTTP status and errors
			if status >= 400 || len(c.Errors) > 0 {
				extra["success"] = false
				if len(c.Errors) > 0 {
					extra["error"] = c.Errors.String()
				}
			} else {
				extra["success"] = true
			}

			auditService.LogOperation(c.Request.Context(), uid, uname, action, c.Request.URL.Path, extra, c.ClientIP(), c.Request.UserAgent())
		}
	}
}
