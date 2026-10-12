package plugin

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/errx"
)

// maxPluginArchiveSize 插件包大小上限（50MB，含二进制）。
const maxPluginArchiveSize = 50 << 20

// InstallFromZip 安装上传的插件包（.zip）：
// 解压到 staging 目录 → 校验 manifest 与可执行文件 → 原子移动到
// 插件目录 → 注册。zip slip（路径穿越）在解压时拦截。
func (m *Manager) InstallFromZip(ctx context.Context, data []byte) (*Manifest, error) {
	if len(data) == 0 {
		return nil, errx.BadRequest("插件包为空")
	}
	if len(data) > maxPluginArchiveSize {
		return nil, errx.BadRequest("插件包超过 50MB 限制")
	}

	zr, err := zip.NewReader(readerAt{data}, int64(len(data)))
	if err != nil {
		return nil, errx.BadRequest("无效的 zip 插件包: %v", err)
	}

	// staging 目录（插件目录下 .staging-<rand>）
	staging, err := os.MkdirTemp(m.pluginsDir, ".staging-*")
	if err != nil {
		return nil, errx.Internal("创建临时目录失败: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	// 解压（zip slip 防护：每个 entry 目标必须落在 staging 内）
	var manifestPath string
	for _, f := range zr.File {
		clean := filepath.Clean("/" + f.Name) // 锚定后 Clean 吞掉 ..
		target := filepath.Join(staging, clean)
		if !strings.HasPrefix(target, staging+string(filepath.Separator)) {
			return nil, errx.BadRequest("插件包包含非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		// 单文件解压大小上限（防 zip bomb）
		if f.UncompressedSize64 > maxPluginArchiveSize {
			return nil, errx.BadRequest("插件包内文件过大: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			_ = rc.Close()
			return nil, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return nil, err
		}
		_ = rc.Close()
		_ = out.Close()
		if strings.HasSuffix(f.Name, "plugin.yaml") {
			manifestPath = target
		}
	}

	if manifestPath == "" {
		return nil, errx.BadRequest("插件包缺少 plugin.yaml")
	}

	// 校验 manifest（含 exec 存在性）
	mft, err := LoadManifest(manifestPath, filepath.Dir(manifestPath))
	if err != nil {
		return nil, errx.BadRequest("插件 manifest 无效: %v", err)
	}

	// 目标目录：已存在则拒绝（提示先卸载）；否则原子移动
	targetDir := filepath.Join(m.pluginsDir, mft.Name)
	if _, err := os.Stat(targetDir); err == nil {
		return nil, errx.Conflict("插件 %s 已存在，请先卸载或删除目录", mft.Name)
	} else if !os.IsNotExist(err) {
		return nil, errx.Internal("检查插件目录失败: %w", err)
	}
	if err := os.Rename(staging, targetDir); err != nil {
		return nil, errx.Internal("安装插件失败: %w", err)
	}

	// 注册（保持未启用）
	if err := m.repo.UpsertPlugin(ctx, &PluginRecord{
		Name:                mft.Name,
		Version:             mft.Version,
		APIVersion:          mft.APIVersion,
		ManifestPath:        filepath.Join(targetDir, "plugin.yaml"),
		Enabled:             false,
		Status:              StateRegistered,
		PermissionsApproved: mft.Permissions,
	}); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.manifests[mft.Name] = mft
	m.mu.Unlock()

	logManagerf(" 已安装插件 %s（%s）", mft.Name, mft.Version)
	return mft, nil
}

// readerAt 把 []byte 适配为 io.ReaderAt（zip.NewReader 需要）。
type readerAt struct{ b []byte }

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n := copy(p, r.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
