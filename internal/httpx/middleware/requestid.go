package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// requestIDPattern 限制透传的 X-Request-ID 字符集，防止日志注入。
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestIDMiddleware 为每个请求建立关联 ID：透传上游（网关/CDN）带来的
// X-Request-ID，缺失时生成 128bit 随机 hex；写入 gin 上下文（key: request_id）
// 并回写到响应头，便于日志与审计跨服务关联。
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if !requestIDPattern.MatchString(rid) {
			rid = generateRequestID()
		}
		c.Set("request_id", rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见；退化为纳秒时间戳，保证请求仍有关联 ID
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
