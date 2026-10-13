package middleware

import (
	"net/http"
	"time"

	"easyserver/internal/infra/logger"

	"github.com/gin-gonic/gin"
)

// RequestLoggerMiddleware 记录每次 HTTP 请求到全局运行日志（slog → 文件 + stderr）。
// 替代 gin.Logger()（其写 os.Stdout → journald，不进 easyserver.log 文件），
// 使面板运行日志能反映实际请求活动（方法/路径/状态/耗时/客户端 IP）。
// 带请求 ID（RequestIDMiddleware 生成）便于关联日志。
func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		// GET 健康检查与静态资源高频，降为 debug 避免刷屏（面板运行日志仍记录业务请求）
		path := c.Request.URL.Path
		if c.Request.Method == http.MethodGet && (path == "/health" || path == "/favicon.svg") {
			return
		}

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"dur", time.Since(start),
			"ip", c.ClientIP(),
		}
		if rid := c.GetString("request_id"); rid != "" {
			attrs = append(attrs, "rid", rid)
		}

		// 4xx/5xx 用 warn/error 级别（更易在文件里筛出问题），其余 info。
		switch {
		case c.Writer.Status() >= 500:
			logger.Error("http", attrs...)
		case c.Writer.Status() >= 400:
			logger.Warn("http", attrs...)
		default:
			logger.Info("http", attrs...)
		}
	}
}
