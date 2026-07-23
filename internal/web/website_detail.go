package web

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"easyserver/internal/infra/apperror"
)

// SSLCertInfo is the parsed SSL certificate detail for a website.
type SSLCertInfo struct {
	Enabled       bool     `json:"enabled"`
	Subject       string   `json:"subject"`
	Issuer        string   `json:"issuer"`
	NotBefore     string   `json:"not_before"`
	NotAfter      string   `json:"not_after"`
	DaysRemaining int      `json:"days_remaining"`
	Serial        string   `json:"serial"`
	DNSNames      []string `json:"dns_names"`
	SigAlg        string   `json:"sig_algo"`
	CertPath      string   `json:"cert_path"`
	KeyPath       string   `json:"key_path"`
}

// LogEntry is one parsed nginx access-log line.
type LogEntry struct {
	Time   string `json:"time"`
	IP     string `json:"ip"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Bytes  string `json:"bytes"`
	UA     string `json:"ua"`
	Raw    string `json:"raw,omitempty"`
}

// KV is a count bucket (top IPs / paths).
type KV struct {
	Key   string `json:"key"`
	Value int    `json:"value"`
}

// WebsiteStats aggregates access-log statistics.
type WebsiteStats struct {
	TotalRequests int    `json:"total_requests"`
	TotalBytes    int64  `json:"total_bytes"`
	Status2xx     int    `json:"status_2xx"`
	Status3xx     int    `json:"status_3xx"`
	Status4xx     int    `json:"status_4xx"`
	Status5xx     int    `json:"status_5xx"`
	TopIPs        []KV   `json:"top_ips"`
	TopPaths      []KV   `json:"top_paths"`
	Window        string `json:"window"`
}

// HealthResult is a single HTTP health probe result.
type HealthResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

// GetSSL reads and parses the website's SSL certificate.
func (s *WebsiteService) GetSSL(ctx context.Context, webServerID, id int64) (*SSLCertInfo, error) {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	if w == nil {
		return nil, apperror.ErrNotFound.WithMessage("网站不存在")
	}
	info := &SSLCertInfo{CertPath: w.SSLCertPath, KeyPath: w.SSLKeyPath}
	if w.SSLCertPath == "" {
		return info, nil // enabled=false
	}
	info.Enabled = true
	data, err := os.ReadFile(w.SSLCertPath)
	if err != nil {
		return nil, apperror.ErrInternal.WithMessage("读取证书失败: " + err.Error())
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, apperror.ErrInternal.WithMessage("证书 PEM 格式无效")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, apperror.ErrInternal.WithMessage("解析证书失败: " + err.Error())
	}
	info.Subject = cert.Subject.CommonName
	info.Issuer = cert.Issuer.CommonName
	info.NotBefore = cert.NotBefore.Format("2006-01-02 15:04")
	info.NotAfter = cert.NotAfter.Format("2006-01-02 15:04")
	info.DaysRemaining = int(cert.NotAfter.Sub(time.Now()).Hours() / 24)
	info.Serial = cert.SerialNumber.String()
	info.DNSNames = cert.DNSNames
	info.SigAlg = cert.SignatureAlgorithm.String()
	return info, nil
}

// GetConfig reads the generated nginx config file for the website.
func (s *WebsiteService) GetConfig(ctx context.Context, webServerID, id int64) (string, error) {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return "", apperror.WrapError(err)
	}
	if w == nil {
		return "", apperror.ErrNotFound.WithMessage("网站不存在")
	}
	ws, err := s.webServerRepo.Get(ctx, webServerID)
	if err != nil || ws == nil {
		return "", apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	confPath := filepath.Join(ws.SitesAvailable, w.Domain+".conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		return "", apperror.ErrNotFound.WithMessage("配置文件不存在: " + confPath)
	}
	return string(data), nil
}

// GetParsedLogs returns structured access/error log entries.
func (s *WebsiteService) GetParsedLogs(ctx context.Context, webServerID, id int64, logType string, lines int) ([]LogEntry, error) {
	raw, err := s.GetLogs(ctx, webServerID, id, logType, lines)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	return parseNginxLogs(raw), nil
}

// GetStats aggregates access-log statistics from the last N lines.
func (s *WebsiteService) GetStats(ctx context.Context, webServerID, id int64) (*WebsiteStats, error) {
	raw, err := s.GetLogs(ctx, webServerID, id, "access", 5000)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	return computeStats(raw), nil
}

// ProbeHealth performs an HTTP probe against the website and returns the result.
func (s *WebsiteService) ProbeHealth(ctx context.Context, webServerID, id int64) (*HealthResult, error) {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	if w == nil {
		return nil, apperror.ErrNotFound.WithMessage("网站不存在")
	}
	scheme := "http"
	if w.SSLEnabled {
		scheme = "https"
	}
	// 探活走对外标准端口（80/443），不用 nginx 监听端口：经 CDN/反代代理的网站
	// 对外端口与 nginx 监听端口通常不同（如 nginx 听 8080，CDN 对外 443）。
	url := fmt.Sprintf("%s://%s", scheme, w.Domain)
	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	latency := time.Since(start).Milliseconds()
	res := &HealthResult{CheckedAt: time.Now().Format(time.RFC3339), LatencyMs: latency}
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode
	res.OK = resp.StatusCode < 400
	return res, nil
}

// nginxLogRe matches the nginx combined log format.
var nginxLogRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) [^"]*" (\d+) (\S+) "[^"]*" "([^"]*)"`)

func parseNginxLogs(raw string) []LogEntry {
	var entries []LogEntry
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "日志文件尚不存在") || strings.HasPrefix(line, "(读取日志失败") {
			continue
		}
		m := nginxLogRe.FindStringSubmatch(line)
		if m == nil {
			entries = append(entries, LogEntry{Raw: line})
			continue
		}
		entries = append(entries, LogEntry{
			IP:     m[1],
			Time:   m[2],
			Method: m[3],
			Path:   m[4],
			Status: m[5],
			Bytes:  m[6],
			UA:     m[7],
		})
	}
	return entries
}

func computeStats(raw string) *WebsiteStats {
	st := &WebsiteStats{Window: "最近 5000 行"}
	ipCount := map[string]int{}
	pathCount := map[string]int{}
	for _, e := range parseNginxLogs(raw) {
		if e.Status == "" {
			continue
		}
		st.TotalRequests++
		var b int64
		fmt.Sscanf(e.Bytes, "%d", &b)
		st.TotalBytes += b
		switch {
		case e.Status[0] == '2':
			st.Status2xx++
		case e.Status[0] == '3':
			st.Status3xx++
		case e.Status[0] == '4':
			st.Status4xx++
		case e.Status[0] == '5':
			st.Status5xx++
		}
		if e.IP != "" {
			ipCount[e.IP]++
		}
		if e.Path != "" {
			pathCount[e.Path]++
		}
	}
	st.TopIPs = topN(ipCount, 10)
	st.TopPaths = topN(pathCount, 10)
	return st
}

func topN(m map[string]int, n int) []KV {
	kvs := make([]KV, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, KV{Key: k, Value: v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Value > kvs[j].Value })
	if len(kvs) > n {
		kvs = kvs[:n]
	}
	return kvs
}
