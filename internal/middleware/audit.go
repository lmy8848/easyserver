package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"database/sql"

	"github.com/gin-gonic/gin"
)

func AuditMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Read request body for logging
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
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
		if userID != nil {
			uid = userID.(int64)
		}
		if username != nil {
			uname = username.(string)
		}

		detail := map[string]interface{}{
			"method":      c.Request.Method,
			"path":        c.Request.URL.Path,
			"status":      c.Writer.Status(),
			"duration_ms": time.Since(start).Milliseconds(),
		}
		detailJSON, _ := json.Marshal(detail)

		go func() {
			db.Exec(
				`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				uid, uname, c.Request.Method, c.Request.URL.Path, string(detailJSON), c.ClientIP(), c.Request.UserAgent(),
			)
		}()
	}
}
