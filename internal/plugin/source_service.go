package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 插件源协议相关约束（文档 §插件源）。
const (
	// sourceFetchTimeout 拉取源 index.json 的超时。
	sourceFetchTimeout = 5 * time.Second
	// sourceIndexMaxBytes index.json 响应体上限（防恶意大响应）。
	sourceIndexMaxBytes = 10 << 20
	// sourceDownloadTimeout 从源下载插件 zip 的超时。
	sourceDownloadTimeout = 30 * time.Second
)

var sourceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// SourceService 是插件源（市场入口）的业务逻辑：
// 管理已配置源、拉取源插件清单、从源下载安装。
type SourceService struct {
	repo SourceRepository
	mgr  *Manager
}

// NewSourceService 创建插件源服务；mgr 用于复用 InstallFromZip 安装。
func NewSourceService(repo SourceRepository, mgr *Manager) *SourceService {
	return &SourceService{repo: repo, mgr: mgr}
}

// List 列出已配置的插件源。
func (s *SourceService) List(ctx context.Context) ([]PluginSource, error) {
	return s.repo.ListSources(ctx)
}

// Add 添加插件源；校验 name 与 http(s) URL。
func (s *SourceService) Add(ctx context.Context, name, rawURL string) (*PluginSource, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimSpace(rawURL)
	if !sourceNameRe.MatchString(name) {
		return nil, errors.New("源名称无效：仅字母/数字/下划线/横线，≤32 字符")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("源地址无效：需 http(s):// 完整 URL")
	}
	return s.repo.AddSource(ctx, name, strings.TrimRight(rawURL, "/"))
}

// Delete 删除插件源。
func (s *SourceService) Delete(ctx context.Context, id int64) error {
	return s.repo.DeleteSource(ctx, id)
}

// FetchCatalog 拉取指定源的插件清单（GET {url}/index.json）。
func (s *SourceService) FetchCatalog(ctx context.Context, src *PluginSource) (*SourceCatalog, error) {
	indexURL := src.URL + "/index.json"
	reqCtx, cancel := context.WithTimeout(ctx, sourceFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "EasyServer/plugin-source")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取源 %s 失败: %w", indexURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("拉取源 %s 失败: HTTP %d", indexURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, sourceIndexMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取源响应失败: %w", err)
	}
	if len(body) > sourceIndexMaxBytes {
		return nil, errors.New("源响应过大（超过 10MB）")
	}
	var cat SourceCatalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("源 index.json 解析失败: %w", err)
	}
	return &cat, nil
}

// InstallFromURL 从指定 zip 下载地址安装插件：下载 → 复用 InstallFromZip。
// 仅允许 http(s) URL，大小上限 50MB、30s 超时。
func (s *SourceService) InstallFromURL(ctx context.Context, rawURL string) (*Manifest, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("下载地址无效：需 http(s):// 完整 URL")
	}
	reqCtx, cancel := context.WithTimeout(ctx, sourceDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("构造下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "EasyServer/plugin-source")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载插件包失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载插件包失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginArchiveSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取插件包失败: %w", err)
	}
	if len(data) > maxPluginArchiveSize {
		return nil, errors.New("插件包超过 50MB 限制")
	}
	// 复用 InstallFromZip：zip slip / 单文件 bomb / manifest 校验 / 原子安装/注册/日志 全部生效
	return s.mgr.InstallFromZip(ctx, data)
}
