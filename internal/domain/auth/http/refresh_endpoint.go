package http

import (
	"easyserver/internal/domain/auth"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterRefreshRoute 注册移动端刷新接口（公开路由，自带设备绑定与限流）。
// 刷新令牌通过请求体传递（绝不放到 URL），失败统一 401 不暴露原因。
func RegisterRefreshRoute(public *gin.RouterGroup, refreshSvc *auth.RefreshService, jwtSecret string, sessionSvc *auth.SessionService, sessionTTL int64, rateLimit int, rateInterval int64) {
	h := &refreshHandler{
		refresh:  refreshSvc,
		jwt:      jwtSecret,
		sessions: sessionSvc,
		ttl:      sessionTTL,
	}
	// 挂 login 档限流：凭证端点不共享 api 档的宽松配额（参数此前被丢弃未用）
	limited := public.Group("/auth", middleware.RateLimitMiddleware("login", rateLimit, time.Duration(rateInterval)*time.Second))
	limited.POST("/refresh", httpx.H(h.Refresh))
}

// bearerToken 从 Authorization 头提取 Bearer 令牌（refresh 为公开路由，
// JWT 中间件不在此运行，令牌由客户端拦截器自动附带）。
func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		return h[len(prefix):]
	}
	return ""
}

type refreshHandler struct {
	refresh  *auth.RefreshService
	jwt      string
	sessions *auth.SessionService
	ttl      int64
}

func (h *refreshHandler) Refresh(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
		DeviceID     string `json:"device_id" binding:"required"`
	}](c)
	if err != nil {
		return nil, err
	}
	user, err := h.refresh.Rotate(c.Request.Context(), req.RefreshToken, req.DeviceID)
	if err != nil {
		return nil, err
	}
	// 新 access：从 DB 取用户名（refresh 服务只回 user_id，避免在维护两处映射）
	uid, uname, gerr := h.refresh.Identity(c.Request.Context(), req.RefreshToken)
	if gerr != nil {
		return nil, gerr
	}
	access, terr := auth.GenerateToken(h.jwt, uid, uname, time.Duration(h.ttl)*time.Second)
	if terr != nil {
		return nil, errx.Internal("生成令牌失败")
	}
	if h.sessions != nil {
		// 原位替换：请求自带旧 access（拦截器自动附加），替换其会话行——
		// 一台设备始终一行；此前每次 refresh 新建行，旧行还要活满 24h，
		// 会话列表随之堆积同设备的"在线设备"条目。旧令牌无效则退化为新建。
		oldToken := bearerToken(c)
		expires := time.Now().Add(time.Duration(h.ttl) * time.Second)
		var serr error
		if oldToken != "" {
			serr = h.sessions.ReplaceSession(c.Request.Context(), oldToken, access, uid, c.ClientIP(), c.Request.UserAgent(), "mobile", req.DeviceID, expires)
		} else {
			serr = h.sessions.CreateSession(c.Request.Context(), access, uid, c.ClientIP(), c.Request.UserAgent(), "mobile", req.DeviceID, expires)
		}
		if serr != nil {
			return nil, errx.Internal("创建会话失败")
		}
	}
	middleware.AuditSummary(c, "刷新移动端令牌")
	return gin.H{
		"token":              access,
		"refresh_token":      user.Token,
		"refresh_expires_at": user.ExpiresAt,
	}, nil
}
