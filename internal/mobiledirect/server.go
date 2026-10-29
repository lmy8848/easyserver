package mobiledirect

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/logger"

	"github.com/gin-gonic/gin"
)

// Server 是移动端直连通道的独立 TLS 监听器：设备签名校验 → 反向代理到面板
// 自身 loopback 端口，并以"服务级 + 用户身份"的头部传递，复用全部已有业务路由。
type Server struct {
	cfg      config.MobileDirectConfig
	store    *Store
	upstream *url.URL
	// mint 为通过签名的设备铸造短期 JWT（由 api.Setup 注入，写入会话表）。
	mint   func(ctx context.Context, userID int64) (string, time.Time, error)
	replay *replayGuard
	proxy  *httputil.ReverseProxy
	srv    *http.Server

	tokMu  sync.Mutex
	tokens map[string]cachedToken
}

// NewServer 构造直连通道。upstreamBase 形如 https://127.0.0.1:443，
// token 是面板进程内的服务令牌（与插件 Host API 自请求同一个）。
func NewServer(cfg config.MobileDirectConfig, store *Store, upstreamBase string,
	mint func(ctx context.Context, userID int64) (string, time.Time, error)) (*Server, error) {
	up, err := url.Parse(upstreamBase)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream %q: %w", upstreamBase, err)
	}
	s := &Server{
		cfg:      cfg,
		store:    store,
		upstream: up,
		mint:     mint,
		replay:   newReplayGuard(cfg.ClockSkew.Duration() * 2),
		tokens:   make(map[string]cachedToken),
	}

	proxy := &httputil.ReverseProxy{}
	// Rewrite 取代已弃用的 Director（Go 1.26+，SA1019）：SetURL 承担
	// 单目标改写，SetXForwarded 补回 Director 路径自带的 X-Forwarded-*。
	proxy.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetURL(up)
		if cfg.Hostname != "" {
			req.Out.Host = cfg.Hostname
		}
		// 身份以中间件铸造的短期 JWT Cookie 传给主引擎；清除客户端自带凭据，
		// 保证身份只来自设备签名。
		req.Out.Header.Del("Authorization")
		req.SetXForwarded()
	}
	// loopback 自请求，使用面板自己的自签/内部证书。
	proxy.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Warn("mobile-direct", "proxy error", "path", r.URL.Path, "err", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":50200,"message":"upstream unavailable","data":null}`))
	}
	s.proxy = proxy
	return s, nil
}

// Start 在独立端口上启动 TLS 监听。失败返回错误（由调用方决定是否禁用）。
func (s *Server) Start() error {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.Use(gin.Recovery())
	e.Use(s.pathGuard)
	e.Use(middleware.RateLimitMiddleware("mobile_direct", 600, time.Minute))
	e.Use(s.deviceAuth)
	e.Any("/*any", func(c *gin.Context) {
		s.proxy.ServeHTTP(c.Writer, c.Request)
	})

	s.srv = &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		logger.Info("mobile-direct", "listening", "addr", s.cfg.Listen, "host", s.cfg.Hostname)
		if err := s.srv.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile); err != nil && err != http.ErrServerClosed {
			logger.Error("mobile-direct", "serve failed", "err", err)
		}
	}()
	return nil
}

// Shutdown 优雅关闭直连监听。
func (s *Server) Shutdown(ctx context.Context) {
	if s.srv != nil {
		_ = s.srv.Shutdown(ctx)
	}
}

// pathGuard 限制直连通道只提供 /api/ 业务接口，并屏蔽密码登录/校验类入口
// 与插件宿主 API（这些只走 CF 通道，直连端不提供，减少暴露面）。
func (s *Server) pathGuard(c *gin.Context) {
	p := c.Request.URL.Path
	blocked := []string{
		"/api/auth/login",
		"/api/auth/verify-totp",
		"/api/auth/verify-backup",
		"/api/auth/qr/",
		"/api/host",
		"/api/plugins",
	}
	if !strings.HasPrefix(p, "/api/") {
		abortNotFound(c)
		return
	}
	for _, b := range blocked {
		if strings.HasPrefix(p, b) {
			abortNotFound(c)
			return
		}
	}
	c.Next()
}

// abortNotFound 与真实 404 一致，避免暴露直连端存在。
func abortNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 40400, "message": "not found", "data": nil})
}
