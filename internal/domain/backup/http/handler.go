// Package backuphttp 提供面板自身数据的导出（配置/SQLite/站点/插件目录），
// 用于换机迁移与灾难恢复。导出为 tar.gz，SQLite 用 VACUUM INTO 做一致性快照，
// 避免直接拷贝 WAL 中的半写状态。
//
// 归档先在临时文件中完整生成，成功后才写响应头：任何中途失败都不会产生
// "看起来正常但缺数据库"的截断包（HTTP 一旦开始输出就无法再改状态码）。
package backuphttp

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	store *config.Store
	db    *sql.DB
}

func RegisterRoutes(protected *gin.RouterGroup, store *config.Store, db *sql.DB) {
	h := &Handler{store: store, db: db}
	// 导出大包耗时远超默认 30s 读写超时，单独放宽。
	slow := protected.Group("",
		middleware.WriteTimeout(10*time.Minute),
		middleware.ReadTimeout(10*time.Minute),
	)
	slow.GET("/backup/export", httpx.H(h.Export))
	slow.POST("/backup/inspect", httpx.H(h.Inspect))
}

type manifest struct {
	Version   string   `json:"version"`
	CreatedAt string   `json:"created_at"`
	Includes  []string `json:"includes"`
	Warnings  []string `json:"warnings,omitempty"`
}

func parseIncludes(raw string) map[string]bool {
	set := map[string]bool{"config": true, "db": true}
	if raw == "" {
		return set
	}
	out := map[string]bool{}
	for part := range strings.SplitSeq(raw, ",") {
		switch strings.TrimSpace(part) {
		case "config", "db", "sites", "plugins":
			out[strings.TrimSpace(part)] = true
		}
	}
	if len(out) == 0 {
		return set
	}
	return out
}

// Export 打包导出。查询参数 include=config,db,sites,plugins（默认 config,db）。
func (h *Handler) Export(c *gin.Context) (any, error) {
	includes := parseIncludes(c.Query("include"))
	middleware.AuditSummary(c, "导出面板备份")

	// 归档先在权限收紧的临时目录里生成，避免快照（含密钥/密码）被本机其他用户读取。
	tmpDir, err := os.MkdirTemp("", "easyserver-backup-")
	if err != nil {
		return nil, errx.Internal("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.Chmod(tmpDir, 0o700); err != nil {
		return nil, errx.Internal("设置临时目录权限失败: %w", err)
	}

	archivePath := filepath.Join(tmpDir, "backup.tar.gz")
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, errx.Internal("创建备份文件失败: %w", err)
	}
	warnings, buildErr := h.buildArchive(c.Request.Context(), f, tmpDir, includes)
	closeErr := f.Close()
	if buildErr != nil {
		return nil, errx.Internal("生成备份失败: %w", buildErr)
	}
	if closeErr != nil {
		return nil, errx.Internal("写入备份文件失败: %w", closeErr)
	}

	name := fmt.Sprintf("easyserver-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", httpx.FormatContentDisposition("attachment", name))
	c.Header("Cache-Control", "no-store")
	if len(warnings) > 0 {
		// 非致命缺项（如站点目录不存在）通过响应头告知，前端可提示。
		c.Header("X-Backup-Warnings", strings.Join(warnings, " | "))
	}
	c.File(archivePath)
	return nil, nil
}

// buildArchive 把内容写入 out（tar.gz），返回非致命告警。
func (h *Handler) buildArchive(ctx context.Context, out io.Writer, tmpDir string, includes map[string]bool) ([]string, error) {
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	warnings := []string{}

	m := manifest{Version: infra.Version, CreatedAt: time.Now().Format(time.RFC3339), Includes: keysOf(includes)}
	if err := writeJSONEntry(tw, "manifest.json", m); err != nil {
		return nil, fmt.Errorf("写入 manifest: %w", err)
	}

	if includes["config"] {
		path := h.store.Get().Path
		if path == "" {
			warnings = append(warnings, "配置路径未知，已跳过 config.toml")
		} else if err := addFile(tw, "config.toml", path); err != nil {
			warnings = append(warnings, "配置读取失败: "+err.Error())
		}
	}
	if includes["db"] {
		if err := h.addDatabase(ctx, tw, tmpDir); err != nil {
			return nil, fmt.Errorf("备份数据库: %w", err)
		}
	}
	if includes["sites"] {
		root := filepath.Join(config.DataRoot, "data")
		if _, err := os.Stat(root); err != nil {
			warnings = append(warnings, "站点目录不存在: "+root)
		} else if err := addDir(tw, "data", root, func(rel string) bool {
			base := filepath.Base(rel)
			return strings.HasPrefix(base, "easyserver.db") || strings.HasSuffix(base, ".log")
		}); err != nil {
			return nil, fmt.Errorf("备份站点目录: %w", err)
		}
	}
	if includes["plugins"] {
		root := filepath.Join(config.DataRoot, "plugins")
		if _, err := os.Stat(root); err != nil {
			warnings = append(warnings, "插件目录不存在: "+root)
		} else if err := addDir(tw, "plugins", root, nil); err != nil {
			return nil, fmt.Errorf("备份插件目录: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return warnings, nil
}

// addDatabase 用 VACUUM INTO 生成一致快照后写入归档。
// 复用面板现有连接池（带 busy_timeout），避免新连接立刻撞 "database is locked"。
func (h *Handler) addDatabase(ctx context.Context, tw *tar.Writer, tmpDir string) error {
	snapshot := filepath.Join(tmpDir, "snapshot.db")
	if _, err := h.db.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		return err
	}
	return addFile(tw, "data/easyserver.db", snapshot)
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out) // 顺序稳定，manifest 可复现
	return out
}

func writeJSONEntry(tw *tar.Writer, name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(tw, name, data, 0o600)
}

func addFile(tw *tar.Writer, name, path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s 是目录", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o600,
		Size:    st.Size(),
		ModTime: st.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func writeBytes(tw *tar.Writer, name string, data []byte, mode int64) error {
	hdr := &tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: time.Now()}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// addDir 递归写入目录；skip 返回 true 表示跳过该相对路径。
func addDir(tw *tar.Writer, prefix, root string, skip func(rel string) bool) error {
	if _, err := os.Stat(root); err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// 跳过不可读条目（权限/竞态删除），备份继续；缺项由调用方决定是否告警。
			return nil //nolint:nilerr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // 相对路径算不出来就跳过该条目，不影响整体备份
		}
		if rel == "." {
			return nil
		}
		if skip != nil && skip(rel) {
			return nil
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.Size() > 256<<20 { // 单文件上限 256MB
			log.Printf("backup: skip large file %s (%d bytes)", rel, info.Size())
			return nil
		}
		return addFile(tw, filepath.ToSlash(filepath.Join(prefix, rel)), path)
	})
}
