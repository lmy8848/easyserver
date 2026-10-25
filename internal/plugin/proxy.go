package plugin

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// transportCache 按 socket 路径缓存 Unix socket Transport：代理每个请求
// 新建 Transport 会泄漏连接（不关 idle conn），缓存后跨请求复用连接池。
var transportCache sync.Map // sockPath → *http.Transport

func transportFor(sockPath string) *http.Transport {
	if v, ok := transportCache.Load(sockPath); ok {
		return v.(*http.Transport)
	}
	t := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		},
		MaxIdleConns:        4,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     8,
		MaxIdleConnsPerHost: 4,
	}
	transportCache.Store(sockPath, t)
	return t
}

// newPluginProxy 创建拨向插件 socket 的反向代理（文档 §3.2：超时/限流由
// 面板侧强制，超时经 TimeoutHandler 实现）。
//
// stripPrefix 是 /api/plugins/<id> 前缀：代理将其剥离后再转发给插件，
// 使插件业务路由以插件自身视角（如 /api/status）注册即可被浏览器经
// /api/plugins/<id>/api/status 访问——否则插件收到带前缀的路径全部 404。
//
// 安全：入站的 Authorization/Cookie（面板用户 JWT 与会话）绝不透传给
// 插件进程——插件不应获得面板用户凭据（插件经自身的 plugin token 走
// Host API，代理请求本身无需认证语义）。
func newPluginProxy(socketPath string, requestTimeout time.Duration, stripPrefix string) http.Handler {
	target := &url.URL{Scheme: "http", Host: "plugin"} // host 仅占位，拨号走 socket
	// 用裸 ReverseProxy + Rewrite：NewSingleHostReverseProxy 自带 Director，
	// 与 Rewrite 冲突（ReverseProxy 要求二选一），会导致代理报错不走 transport
	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		path := strings.TrimPrefix(pr.In.URL.Path, stripPrefix)
		if path == "" || !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		pr.Out.URL.Path = path
		pr.Out.URL.RawPath = "" // 让 Out 按 path 重新转义
		pr.Out.Host = ""
		// 剥离面板凭据：插件不可收割用户 JWT/会话 Cookie
		pr.Out.Header.Del("Authorization")
		pr.Out.Header.Del("Cookie")
	}
	proxy.Transport = transportFor(socketPath)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		logManagerf(" 代理 %s 失败: %v", socketPath, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":50200,"message":"插件不可达","data":null}`))
	}
	return http.TimeoutHandler(proxy, requestTimeout, `{"code":50400,"message":"插件响应超时","data":null}`)
}
