package backuphttp

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// restoreResult 是恢复操作的受理结果。
type restoreResult struct {
	Applied        []string `json:"applied"`
	Skipped        []string `json:"skipped"`
	PreSnapshot    string   `json:"pre_snapshot"`
	RestartPlanned bool     `json:"restart_planned"`
}

// Restore 从上传的备份包恢复配置与数据库。危险操作，要求 confirm=RESTORE。
//
// 保护措施：
//  1. 先只读解包（严格校验路径/链接、解压总量上限），内容进权限收紧的临时目录；
//  2. 生成"恢复前快照"（当前配置+数据库）留档，任何异常都能人工回滚；
//  3. 配置/数据库用 rename 原子替换，旧文件保留为 *.bak-<ts>；
//  4. 替换后请求热重启（进程内 Signal），避免运行中的连接池继续持有被替换的旧库。
//
// 站点/插件目录本次不覆盖（体量大且牵扯权限），会在 skipped 中列出，供人工确认后处理。
func (h *Handler) Restore(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "恢复面板备份")

	if c.PostForm("confirm") != "RESTORE" {
		return nil, errx.BadRequest("恢复操作需要二次确认：confirm 必须为 RESTORE")
	}
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

	stage, err := os.MkdirTemp(filepath.Join(config.DataRoot, "data"), ".restore-")
	if err != nil {
		return nil, errx.Internal("创建恢复临时目录失败: %w", err)
	}
	defer os.RemoveAll(stage)
	_ = os.Chmod(stage, 0o700)

	var m manifest
	var hasConfig, hasDB bool
	var skipped []string
	var unpacked int64
	tr := tar.NewReader(gz)
	for {
		hdr, nerr := tr.Next()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			return nil, errx.BadRequest("解析备份包失败: %w", nerr)
		}
		unpacked += hdr.Size
		if unpacked > maxUnpackedSize {
			return nil, errx.BadRequest("备份包解压后超过 4GB，拒绝恢复")
		}
		clean := filepath.Clean(hdr.Name)
		if filepath.IsAbs(hdr.Name) || strings.HasPrefix(clean, "..") ||
			hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return nil, errx.BadRequest("备份包包含非法条目: %s", hdr.Name)
		}
		switch hdr.Name {
		case "manifest.json":
			data, rerr := io.ReadAll(io.LimitReader(tr, 1<<20))
			if rerr != nil {
				return nil, errx.Internal("读取 manifest 失败: %w", rerr)
			}
			if jerr := json.Unmarshal(data, &m); jerr != nil {
				return nil, errx.BadRequest("manifest.json 解析失败: %w", jerr)
			}
		case "config.toml":
			if werr := writeEntry(tr, filepath.Join(stage, "config.toml")); werr != nil {
				return nil, errx.Internal("解包配置失败: %w", werr)
			}
			hasConfig = true
		case "data/easyserver.db":
			if werr := writeEntry(tr, filepath.Join(stage, "easyserver.db")); werr != nil {
				return nil, errx.Internal("解包数据库失败: %w", werr)
			}
			hasDB = true
		default:
			if strings.HasPrefix(hdr.Name, "data/") || strings.HasPrefix(hdr.Name, "plugins/") {
				skipped = append(skipped, hdr.Name)
			}
		}
	}
	if m.Version == "" {
		return nil, errx.BadRequest("备份包缺少 manifest.json 或版本信息")
	}
	if !hasConfig && !hasDB {
		return nil, errx.BadRequest("备份包既不含配置也不含数据库，无可恢复内容")
	}

	// 恢复前快照：先留一份当前状态，任何问题都可回滚。
	preSnapshot, serr := h.makePreRestoreSnapshot(c.Request.Context())
	if serr != nil {
		return nil, errx.Internal("生成恢复前快照失败（已中止恢复）: %w", serr)
	}

	ts := time.Now().Format("20060102-150405")
	cfgPath := h.store.Get().Path
	applied := []string{}
	if hasConfig && cfgPath != "" {
		if rerr := replaceFile(filepath.Join(stage, "config.toml"), cfgPath, ts); rerr != nil {
			return nil, errx.Internal("替换配置失败: %w", rerr)
		}
		applied = append(applied, "config.toml")
	}
	if hasDB {
		if rerr := replaceFile(filepath.Join(stage, "easyserver.db"), database.DBPath, ts); rerr != nil {
			return nil, errx.Internal("替换数据库失败: %w", rerr)
		}
		// 旧库的 WAL/SHM 属于被替换的 inode，必须清掉，否则新库可能读到旧日志。
		for _, suffix := range []string{"-wal", "-shm"} {
			_ = os.Remove(database.DBPath + suffix)
		}
		applied = append(applied, "data/easyserver.db")
	}

	// 热重启：让进程用新配置/新数据库重新初始化（连接池不能继续持有被替换的旧库）。
	infra.Go(func() {
		time.Sleep(800 * time.Millisecond) // 先把响应交给调用方
		h.sig.Request(infra.RestartOpts{ConfigPath: cfgPath, Force: true})
	})

	return restoreResult{Applied: applied, Skipped: skipped, PreSnapshot: preSnapshot, RestartPlanned: true}, nil
}

// writeEntry 把当前 tar 条目写入目标文件（0600）。
func writeEntry(tr *tar.Reader, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, tr)
	return err
}

// replaceFile 用 src 原子替换 dst，旧文件保留为 dst.bak-<ts>。
func replaceFile(src, dst, ts string) error {
	if _, err := os.Stat(dst); err == nil {
		if rerr := os.Rename(dst, dst+".bak-"+ts); rerr != nil {
			return fmt.Errorf("备份旧文件失败: %w", rerr)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("替换文件失败: %w", err)
	}
	return os.Chmod(dst, 0o600)
}

// makePreRestoreSnapshot 用当前配置与数据库生成一份快照，落到 backups 目录。
func (h *Handler) makePreRestoreSnapshot(ctx context.Context) (string, error) {
	dir := filepath.Join(config.DataRoot, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "pre-restore-"+time.Now().Format("20060102-150405")+".tar.gz")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	tmp, terr := os.MkdirTemp("", "easyserver-restore-")
	if terr != nil {
		f.Close()
		_ = os.Remove(path)
		return "", terr
	}
	defer os.RemoveAll(tmp)

	_, berr := h.buildArchive(ctx, f, tmp, map[string]bool{"config": true, "db": true})
	cerr := f.Close()
	if berr != nil {
		_ = os.Remove(path)
		return "", berr
	}
	if cerr != nil {
		_ = os.Remove(path)
		return "", cerr
	}
	return path, nil
}
