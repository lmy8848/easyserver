package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServiceTokenHeader 是面板服务级调用的鉴权头（panel.api.proxy 自请求携带）。
// token 由面板启动时随机生成，仅存于面板进程内，不对外暴露。
const ServiceTokenHeader = "X-Panel-Service-Token"

// ServiceTokenMiddleware 校验面板服务 token：插件经 Host API panel.api.proxy
// 自请求面板 API 时携带该头，命中则标记服务身份并跳过用户 JWT（服务级调用，
// 非用户态）；普通用户请求不受影响，透传给 JWTMiddleware。
// 必须挂载在 JWTMiddleware 之前。
func ServiceTokenMiddleware(serviceToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(ServiceTokenHeader)
		if token == "" {
			c.Next() // 普通请求，走用户 JWT
			return
		}
		if token != serviceToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40100, "message": "invalid service token", "data": nil})
			return
		}
		c.Set("service_authenticated", true)
		c.Set("username", "panel-api-proxy") // 审计与日志里体现为面板服务代理
		c.Next()
	}
}
