package plugin

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// newPluginProxy 创建拨向插件 socket 的反向代理（文档 §3.2：超时/限流由
// 面板侧强制，插件慢不拖垮主面板）。
func newPluginProxy(socketPath string, requestTimeout time.Duration) http.Handler {
	target := &url.URL{Scheme: "http", Host: "plugin"} // host 仅占位，拨号走 socket
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"message":"插件响应超时"}`))
	}
	return http.TimeoutHandler(proxy, requestTimeout, `{"message":"插件响应超时"}`)
}
