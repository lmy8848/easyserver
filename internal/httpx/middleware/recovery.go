package middleware

import (
	"net/http"
	"runtime/debug"

	"easyserver/internal/infra/logger"

	"github.com/gin-gonic/gin"
)

// PanicRecovery 替代 gin.Recovery()：捕获 handler panic 后记录到全局运行日志
// （slog → 文件 + stderr，含堆栈，便于在 easyserver.log 排查崩溃），并返回
// 统一 500 JSON（与 ErrorHandler 的错误码约定一致）。
// gin.Recovery() 默认写 os.Stdout（仅 journald），运行日志文件不可见。
func PanicRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"ip", c.ClientIP(),
					"err", r,
					"stack", string(debug.Stack()),
				)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"code":    50000,
						"message": "internal server error",
						"data":    nil,
					})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
