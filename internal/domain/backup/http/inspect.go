package backuphttp

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

const (
	maxInspectSize  = 200 << 20 // 200MB（压缩包大小）
	maxUnpackedSize = 4 << 30   // 4GB（解压总量，防 gzip 炸弹）
)

type inspectEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type inspectResult struct {
	Manifest  manifest       `json:"manifest"`
	Entries   []inspectEntry `json:"entries"`
	TotalSize int64          `json:"total_size"`
	Warnings  []string       `json:"warnings"`
}

// Inspect 对上传的备份包做只读预检：解析 manifest、列出条目、校验路径安全，
// 不做任何落盘。恢复前先用它确认包内容（版本/大小/是否含站点与插件）。
func (h *Handler) Inspect(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "预检面板备份包")

	fh, err := c.FormFile("file")
	if err != nil {
		return nil, errx.BadRequest("请上传备份文件（字段名 file）")
	}
	if fh.Size > maxInspectSize {
		return nil, errx.BadRequest("备份包超过 200MB 上限")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errx.Internal("读取上传文件失败: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, errx.BadRequest("不是有效的 tar.gz 备份包")
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	res := inspectResult{Warnings: []string{}}
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errx.BadRequest("解析备份包失败: %w", err)
		}
		name := hdr.Name
		if filepath.IsAbs(name) || strings.HasPrefix(filepath.Clean(name), "..") {
			res.Warnings = append(res.Warnings, "包含非法路径已忽略: "+name)
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			res.Warnings = append(res.Warnings, "包含链接条目已忽略: "+name)
			continue
		}
		total += hdr.Size
		if total > maxUnpackedSize {
			return nil, errx.BadRequest("备份包解压后超过 4GB，疑似异常包或被压缩放大")
		}
		res.TotalSize = total
		if len(res.Entries) < 200 {
			res.Entries = append(res.Entries, inspectEntry{Name: name, Size: hdr.Size})
		}
		if name == "manifest.json" {
			data, rerr := io.ReadAll(io.LimitReader(tr, 1<<20))
			if rerr == nil {
				if jerr := json.Unmarshal(data, &res.Manifest); jerr != nil {
					res.Warnings = append(res.Warnings, "manifest.json 解析失败")
				}
			}
		}
	}
	if res.Manifest.Version == "" {
		return nil, errx.BadRequest("备份包缺少 manifest.json 或版本信息")
	}
	if !strings.Contains(strings.Join(res.Manifest.Includes, ","), "db") {
		res.Warnings = append(res.Warnings, "备份包未包含数据库，恢复后数据不会还原")
	}
	return res, nil
}
