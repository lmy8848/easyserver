package plugin

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// newPluginProxy 创建拨向插件 socket 的反向代理（文档 §3.2：超时/限流由
// 面板侧强制，插件慢不拖垮主面板）。
//
// stripPrefix 是 /api/plugins/<id> 前缀：代理将其剥离后再转发给插件，
// 使插件业务路由以插件自身视角（如 /api/status）注册即可被浏览器经
// /api/plugins/<id>/api/status 访问——否则插件收到带前缀的路径全部 404。
func newPluginProxy(socketPath string, requestTimeout time.Duration, stripPrefix string) http.Handler {
	target := &url.URL{Scheme: "http", Host: "plugin"} // host 仅占位，拨号走 socket
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		path := strings.TrimPrefix(pr.In.URL.Path, stripPrefix)
		if path == "" || !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		pr.Out.URL.Path = path
		pr.Out.URL.RawPath = "" // 让 Out 按 path 重新转义
		pr.Out.Host = ""
	}
	proxy.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("plugin[proxy]: 插件代理失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"插件不可达"}`))
	}
	return http.TimeoutHandler(proxy, requestTimeout, `{"message":"插件响应超时"}`)
}
