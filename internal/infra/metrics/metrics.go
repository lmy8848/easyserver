// Package metrics 提供最小化的 Prometheus 文本指标：HTTP 请求计数与耗时直方图，
// 以及面板关键 gauge（活跃会话等）。刻意不引入 prometheus/client_golang，
// 保持依赖面最小；文本格式与 Prometheus/Grafana 完全兼容。
package metrics

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var buckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500} // 毫秒

type seriesKey struct {
	method, route, status string
}

type Registry struct {
	mu       sync.RWMutex
	counts   map[seriesKey]uint64
	histo    map[seriesKey][]uint64 // 每个 bucket 的累计计数（最后一项为 +Inf）
	sumMs    map[seriesKey]float64
	sessions int64
}

var Default = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		counts: make(map[seriesKey]uint64),
		histo:  make(map[seriesKey][]uint64),
		sumMs:  make(map[seriesKey]float64),
	}
}

// Middleware 记录每个请求的状态码与耗时（按路由模板分组，避免 path 高基数）。
func (r *Registry) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if c.Request.URL.Path == "/metrics" {
			return
		}
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		k := seriesKey{c.Request.Method, route, strconv.Itoa(c.Writer.Status())}
		ms := float64(time.Since(start).Microseconds()) / 1000.0

		r.mu.Lock()
		r.counts[k]++
		h := r.histo[k]
		if h == nil {
			h = make([]uint64, len(buckets)+1)
		}
		idx := len(buckets)
		for i, b := range buckets {
			if ms <= b {
				idx = i
				break
			}
		}
		h[idx]++
		r.histo[k] = h
		r.sumMs[k] += ms
		r.mu.Unlock()
	}
}

// SetActiveSessions 由会话服务定期写入，作为 gauge 暴露。
func (r *Registry) SetActiveSessions(n int64) {
	r.mu.Lock()
	r.sessions = n
	r.mu.Unlock()
}

// Handler 输出 Prometheus 文本格式。鉴权规则：
//   - token 为空：仅允许 loopback 访问（本机 curl/Prometheus）；
//   - token 非空：要求 Authorization: Bearer <token>，可来自任意地址。
func (r *Registry) Handler(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			if !isLoopback(c.ClientIP()) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		} else {
			got := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		}
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.String(http.StatusOK, r.Render())
	}
}

// Render 生成文本指标（便于测试）。
func (r *Registry) Render() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var b strings.Builder
	b.WriteString("# HELP easyserver_http_requests_total HTTP 请求总数\n")
	b.WriteString("# TYPE easyserver_http_requests_total counter\n")
	keys := make([]seriesKey, 0, len(r.counts))
	for k := range r.counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		return keys[i].status < keys[j].status
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "easyserver_http_requests_total{method=%q,route=%q,status=%q} %d\n",
			k.method, k.route, k.status, r.counts[k])
	}

	b.WriteString("# HELP easyserver_http_request_duration_milliseconds HTTP 请求耗时直方图\n")
	b.WriteString("# TYPE easyserver_http_request_duration_milliseconds histogram\n")
	for _, k := range keys {
		h := r.histo[k]
		var cum uint64
		for i, bound := range buckets {
			cum += h[i]
			fmt.Fprintf(&b, "easyserver_http_request_duration_milliseconds_bucket{method=%q,route=%q,status=%q,le=%q} %d\n",
				k.method, k.route, k.status, strconv.FormatFloat(bound, 'f', -1, 64), cum)
		}
		cum += h[len(buckets)]
		fmt.Fprintf(&b, "easyserver_http_request_duration_milliseconds_bucket{method=%q,route=%q,status=%q,le=\"+Inf\"} %d\n",
			k.method, k.route, k.status, cum)
		fmt.Fprintf(&b, "easyserver_http_request_duration_milliseconds_sum{method=%q,route=%q,status=%q} %s\n",
			k.method, k.route, k.status, strconv.FormatFloat(r.sumMs[k], 'f', 3, 64))
		fmt.Fprintf(&b, "easyserver_http_request_duration_milliseconds_count{method=%q,route=%q,status=%q} %d\n",
			k.method, k.route, k.status, cum)
	}

	b.WriteString("# HELP easyserver_sessions_active 当前有效会话数\n")
	b.WriteString("# TYPE easyserver_sessions_active gauge\n")
	fmt.Fprintf(&b, "easyserver_sessions_active %d\n", r.sessions)
	return b.String()
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
