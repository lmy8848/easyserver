package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
)

// 网站自定义图标采用约定式存储：文件存在即生效，无需数据库字段。
// 路径：<DataRoot>/data/site-icons/<domain>.<ext>；删除文件即恢复默认。
// nginx 配置生成时若图标存在会注入 location = /favicon.ico 块（见
// nginxFaviconBlock），上传/删除后重生成配置并 reload。

const maxIconSize = 2 << 20 // 2 MB

// iconExts 是允许的图标扩展名与探测顺序（同域名多扩展互斥，保留一个）。
var iconExts = []string{".ico", ".png", ".svg", ".jpg"}

var iconMime = map[string]string{
	".ico": "image/x-icon",
	".png": "image/png",
	".svg": "image/svg+xml",
	".jpg": "image/jpeg",
}

func siteIconDir() string {
	return filepath.Join(config.DataRoot, "data", "site-icons")
}

// siteIconPath 返回该域名已存图标的路径；ok=false 表示未设置自定义图标。
func siteIconPath(domain string) (string, bool) {
	dir := siteIconDir()
	for _, ext := range iconExts {
		p := filepath.Join(dir, domain+ext)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}

// sniffIconExt 通过魔数识别图片类型并返回规范扩展名。SVG 是文本格式，
// 按 XML/`<svg` 前缀识别。
func sniffIconExt(head []byte) (string, error) {
	switch {
	case len(head) >= 4 && head[0] == 0x89 && head[1] == 'P' && head[2] == 'N' && head[3] == 'G':
		return ".png", nil
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return ".jpg", nil
	case len(head) >= 4 && head[0] == 0 && head[1] == 0 && head[2] == 1 && head[3] == 0:
		return ".ico", nil
	}
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	trimmed = bytes.TrimPrefix(trimmed, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<svg")) {
		return ".svg", nil
	}
	return "", errx.BadRequest("不支持的图标格式，请上传 ico/png/svg/jpg")
}

// removeSiteIcons 删除该域名的所有扩展名变体，返回是否删除了文件。
func removeSiteIcons(domain string) bool {
	removed := false
	for _, ext := range iconExts {
		if err := os.Remove(filepath.Join(siteIconDir(), domain+ext)); err == nil {
			removed = true
		}
	}
	return removed
}

// IconFile 返回已存图标的文件路径；ok=false 表示未设置自定义图标。
func (s *WebsiteService) IconFile(ctx context.Context, webServerID, id int64) (path string, ok bool, err error) {
	w, err := s.getWebsite(ctx, webServerID, id)
	if err != nil {
		return "", false, err
	}
	path, ok = siteIconPath(w.Domain)
	return path, ok, nil
}

// UploadIcon 保存网站自定义图标（覆盖旧图标），并重生成 nginx 配置使
// /favicon.ico 指向新图标。src 最多读取 maxIconSize+1 字节用于超限判断。
func (s *WebsiteService) UploadIcon(ctx context.Context, webServerID, id int64, src io.Reader) error {
	w, err := s.getWebsite(ctx, webServerID, id)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(io.LimitReader(src, maxIconSize+1))
	if err != nil {
		return errx.Internal("读取上传内容失败: %w", err)
	}
	if len(data) > maxIconSize {
		return errx.BadRequest("图标文件超过 2MB 限制")
	}
	ext, err := sniffIconExt(data)
	if err != nil {
		return err
	}

	dir := siteIconDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errx.Internal("创建图标目录失败: %w", err)
	}
	// 同域名多扩展互斥：先清掉旧变体再写入，保证 siteIconPath 探测唯一
	removeSiteIcons(w.Domain)
	iconPath := filepath.Join(dir, w.Domain+ext)
	if err := os.WriteFile(iconPath, data, 0644); err != nil {
		return errx.Internal("写入图标失败: %w", err)
	}

	if err := s.regenerateConfig(ctx, webServerID, w); err != nil {
		return err
	}
	return nil
}

// DeleteIcon 删除网站自定义图标（恢复默认 favicon），并重生成 nginx 配置。
func (s *WebsiteService) DeleteIcon(ctx context.Context, webServerID, id int64) error {
	w, err := s.getWebsite(ctx, webServerID, id)
	if err != nil {
		return err
	}
	if !removeSiteIcons(w.Domain) {
		return nil // 本就没有自定义图标
	}
	return s.regenerateConfig(ctx, webServerID, w)
}

// regenerateConfig 重生成 nginx 配置并 reload，图标增删后调用。
func (s *WebsiteService) regenerateConfig(ctx context.Context, webServerID int64, w *Website) error {
	ws, err := s.webServerRepo.Get(ctx, webServerID)
	if err != nil || ws == nil {
		return errx.NotFound("Web 服务器不存在")
	}
	if err := s.writeConfigForServer(ctx, webServerID, w); err != nil {
		return errx.Internal("重生成站点配置失败: %w", err)
	}
	s.reloadWebServer(ctx, ws)
	return nil
}

// getWebsite 是图标相关操作的公共取站逻辑。
func (s *WebsiteService) getWebsite(ctx context.Context, webServerID, id int64) (*Website, error) {
	w, err := s.repo.Get(ctx, webServerID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errx.NotFound("网站不存在")
	}
	return w, nil
}

// nginxFaviconBlock 返回该域名自定义图标的 nginx location 块；无图标时
// 返回空串。Content-Type 由请求 URI 的 .ico 扩展决定，浏览器对 favicon
// 会按内容嗅探，png/svg 文件同样可正常显示。
func nginxFaviconBlock(domain string) string {
	path, ok := siteIconPath(domain)
	if !ok {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if iconMime[ext] == "" {
		return ""
	}
	return fmt.Sprintf(`
    location = /favicon.ico {
        root %s;
        try_files /%s%s =404;
        expires 7d;
        log_not_found off;
    }
`, siteIconDir(), domain, ext)
}
