package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"easyserver/internal/infra/errx"
)

// 安装/升级的安全上限。
const (
	maxPluginArchiveSize = 50 << 20  // 单个插件包大小上限（50MB，含二进制）
	maxPluginEntries     = 10000     // zip entry 数量上限（防超大包拖垮解压）
	maxPluginTotalSize   = 150 << 20 // 总解压量上限（3×包上限，防 zip bomb 写满磁盘）
)

// InstallFromZip 安装上传的插件包（.zip）：
// 解压到 staging 目录 → 校验 manifest/可执行架构 → 原子移动到插件目录 → 注册。
// 同名插件已存在时走升级路径（保留配置/密钥/已审批权限，按需自动重启）。
// zip slip（路径穿越）、zip bomb（entry 数/总解压量/单文件大小）在解压时拦截。
// 安装/升级成败均写入插件管理日志（/var/log/easyserver/plugin-manager.log）。
// 返回 upgraded=true 表示这是一次覆盖升级而非新装。
func (m *Manager) InstallFromZip(ctx context.Context, data []byte) (mft *Manifest, upgraded bool, err error) {
	if len(data) == 0 {
		logManagerf(" 安装插件失败: 插件包为空")
		return nil, false, errx.BadRequest("插件包为空")
	}
	if len(data) > maxPluginArchiveSize {
		logManagerf(" 安装插件失败: 插件包超过 50MB 限制")
		return nil, false, errx.BadRequest("插件包超过 50MB 限制")
	}

	zr, err := zip.NewReader(readerAt{data}, int64(len(data)))
	if err != nil {
		logManagerf(" 安装插件失败: 无效的 zip 插件包: %v", err)
		return nil, false, errx.BadRequest("无效的 zip 插件包: %v", err)
	}
	if len(zr.File) > maxPluginEntries {
		logManagerf(" 安装插件失败: 插件包文件数超过 %d", maxPluginEntries)
		return nil, false, errx.BadRequest("插件包文件数超过 %d", maxPluginEntries)
	}

	// staging 目录（插件目录下 .staging-<rand>）
	staging, err := os.MkdirTemp(m.pluginsDir, ".staging-*")
	if err != nil {
		logManagerf(" 安装插件失败: 创建临时目录失败: %v", err)
		return nil, false, errx.Internal("创建临时目录失败: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	// 解压（zip slip 防护：每个 entry 目标必须落在 staging 内；
	// zip bomb 防护：单文件与总解压量上限；权限按 zip 内可执行位规范化）
	var manifestPath string
	var total uint64
	for _, f := range zr.File {
		clean := filepath.Clean("/" + f.Name) // 锚定后 Clean 吞掉 ..
		target := filepath.Join(staging, clean)
		if !strings.HasPrefix(target, staging+string(filepath.Separator)) {
			logManagerf(" 安装插件失败: 插件包包含非法路径: %s", f.Name)
			return nil, false, errx.BadRequest("插件包包含非法路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				logManagerf(" 安装插件失败: 创建目录 %s 失败: %v", f.Name, err)
				return nil, false, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			logManagerf(" 安装插件失败: 创建目录 %s 失败: %v", f.Name, err)
			return nil, false, err
		}
		// 单文件解压大小上限（防 zip bomb）
		if f.UncompressedSize64 > maxPluginArchiveSize {
			logManagerf(" 安装插件失败: 插件包内文件过大: %s", f.Name)
			return nil, false, errx.BadRequest("插件包内文件过大: %s", f.Name)
		}
		total += f.UncompressedSize64
		if total > maxPluginTotalSize {
			logManagerf(" 安装插件失败: 插件包总解压量超过 %dMB", maxPluginTotalSize>>20)
			return nil, false, errx.BadRequest("插件包总解压量超过上限（%dMB）", maxPluginTotalSize>>20)
		}
		// 权限规范化：zip 内带可执行位 → 0755，其余 0644（此前一律 0755）
		perm := os.FileMode(0o644)
		if f.Mode()&0o111 != 0 {
			perm = 0o755
		}
		rc, err := f.Open()
		if err != nil {
			logManagerf(" 安装插件失败: 打开包内文件 %s 失败: %v", f.Name, err)
			return nil, false, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
		if err != nil {
			_ = rc.Close()
			logManagerf(" 安装插件失败: 写入 %s 失败: %v", f.Name, err)
			return nil, false, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			logManagerf(" 安装插件失败: 解压 %s 失败: %v", f.Name, err)
			return nil, false, err
		}
		_ = rc.Close()
		_ = out.Close()
		if strings.HasSuffix(f.Name, "plugin.yaml") {
			manifestPath = target
		}
	}

	if manifestPath == "" {
		logManagerf(" 安装插件失败: 插件包缺少 plugin.yaml")
		return nil, false, errx.BadRequest("插件包缺少 plugin.yaml")
	}

	// 校验 manifest（含 exec 存在性）
	mft, err = LoadManifest(manifestPath, filepath.Dir(manifestPath))
	if err != nil {
		logManagerf(" 安装插件失败: 插件 manifest 无效: %v", err)
		return nil, false, errx.BadRequest("插件 manifest 无效: %v", err)
	}

	// 架构校验：ELF 二进制须匹配当前系统架构（脚本等非 ELF 放行）
	if err := checkExecutableArch(filepath.Join(staging, mft.Runtime.Exec)); err != nil {
		logManagerf(" 安装插件 %s 失败: %v", mft.Name, err)
		return nil, false, errx.BadRequest("插件可执行文件架构不兼容: %v", err)
	}

	targetDir := filepath.Join(m.pluginsDir, mft.Name)
	if _, statErr := os.Stat(targetDir); statErr == nil {
		return m.upgradePlugin(ctx, mft, staging, targetDir)
	} else if !os.IsNotExist(statErr) {
		logManagerf(" 安装插件失败: 检查插件目录失败: %v", statErr)
		return nil, false, errx.Internal("检查插件目录失败: %w", statErr)
	}

	// ── 新装路径 ──
	if err := os.Rename(staging, targetDir); err != nil {
		logManagerf(" 安装插件失败: 移动插件目录失败: %v", err)
		return nil, false, errx.Internal("安装插件失败: %w", err)
	}
	if err := m.repo.UpsertPlugin(ctx, &PluginRecord{
		Name:                mft.Name,
		Version:             mft.Version,
		APIVersion:          mft.APIVersion,
		ManifestPath:        filepath.Join(targetDir, "plugin.yaml"),
		Enabled:             false,
		Status:              StateRegistered,
		PermissionsApproved: mft.Permissions,
	}); err != nil {
		_ = os.RemoveAll(targetDir) // 回滚已移入的目录，避免半装状态
		logManagerf(" 安装插件失败: 注册 %s 失败: %v", mft.Name, err)
		return nil, false, err
	}
	m.mu.Lock()
	m.manifests[mft.Name] = mft
	m.mu.Unlock()

	logManagerf(" 已安装插件 %s（%s）", mft.Name, mft.Version)
	return mft, false, nil
}

// upgradePlugin 升级安装：保留配置 KV/密钥/已审批权限，停旧进程 → 旧目录
// 改名备份 → 新目录移入 → 更新注册；原启用则按原权限自动重启；失败回滚。
func (m *Manager) upgradePlugin(ctx context.Context, mft *Manifest, staging, targetDir string) (*Manifest, bool, error) {
	name := mft.Name
	old, recErr := m.repo.GetPlugin(ctx, name)
	if recErr != nil || old == nil {
		return nil, false, errx.Internal("读取现有插件记录失败，无法升级")
	}

	// 1. 停止旧进程（不动 DB 的 enabled/approved——与 Disable 语义不同）
	if m.sup != nil {
		if err := m.sup.StopPlugin(ctx, name); err != nil {
			return nil, false, errx.Internal("停止旧插件失败: %w", err)
		}
	}

	// 2. 旧目录备份（回滚保护）
	bak := targetDir + ".bak-upgrade-" + time.Now().Format("20060102-150405")
	if err := os.Rename(targetDir, bak); err != nil {
		return nil, false, errx.Internal("备份旧插件目录失败: %w", err)
	}

	rollback := func() {
		_ = os.RemoveAll(targetDir)
		_ = os.Rename(bak, targetDir)
		_ = m.repo.UpsertPlugin(ctx, &PluginRecord{
			Name:                old.Name,
			Version:             old.Version,
			APIVersion:          old.APIVersion,
			ManifestPath:        old.ManifestPath,
			Enabled:             old.Enabled,
			Status:              StateStopped,
			PermissionsApproved: old.PermissionsApproved,
		})
		// 恢复内存中的旧 manifest（从回滚后的目录重载）
		if om, lerr := LoadManifest(old.ManifestPath, targetDir); lerr == nil {
			m.mu.Lock()
			m.manifests[name] = om
			m.mu.Unlock()
		}
	}

	// 3. 新目录移入 + 更新注册（保留 enabled/approved）
	if err := os.Rename(staging, targetDir); err != nil {
		rollback()
		logManagerf(" 升级插件 %s 失败: 移动新目录失败: %v", name, err)
		return nil, false, errx.Internal("升级插件失败: %w", err)
	}
	if err := m.repo.UpsertPlugin(ctx, &PluginRecord{
		Name:                name,
		Version:             mft.Version,
		APIVersion:          mft.APIVersion,
		ManifestPath:        filepath.Join(targetDir, "plugin.yaml"),
		Enabled:             old.Enabled,
		Status:              StateStopped,
		PermissionsApproved: old.PermissionsApproved,
	}); err != nil {
		rollback()
		logManagerf(" 升级插件 %s 失败: 更新注册失败: %v", name, err)
		return nil, false, err
	}
	m.mu.Lock()
	m.manifests[name] = mft
	m.mu.Unlock()

	logManagerf(" 升级安装插件 %s（%s → %s）", name, old.Version, mft.Version)

	// 4. 原启用则按原审批权限自动重启（失败不回滚文件——已升级成功，仅提示重启失败）
	if old.Enabled {
		if err := m.StartWithApproved(ctx, name, old.PermissionsApproved); err != nil {
			logManagerf(" 升级后重启插件 %s 失败: %v（可手动启动）", name, err)
		}
	}

	// 5. 清理备份（升级已成功且自动重启无致命错误）
	_ = os.RemoveAll(bak)
	return mft, true, nil
}

// elfMachineByArch 映射 runtime.GOARCH → ELF e_machine 值。
var elfMachineByArch = map[string]uint16{
	"amd64": 62,  // EM_X86_64
	"arm64": 183, // EM_AARCH64
	"386":   3,   // EM_386
	"arm":   40,  // EM_ARM
}

// checkExecutableArch 校验插件可执行文件与当前系统架构兼容：
// ELF 二进制要求 e_machine 匹配 runtime.GOARCH（上传 Windows/macOS 二进制
// 在安装时即报错，而不是装完启动失败）；#! 脚本等非 ELF 放行（任意语言
// 可执行文件合法）。
func checkExecutableArch(path string) error {
	want, ok := elfMachineByArch[runtime.GOARCH]
	if !ok {
		return nil // 未知架构不校验
	}
	f, err := os.Open(path) //nolint:gosec // 已由 LoadManifest 校验路径在插件目录内
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	hdr := make([]byte, 20)
	n, err := io.ReadFull(f, hdr)
	if n < 20 {
		return nil // 文件头不完整：无法判断，放行（后续执行自然报错）
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(hdr[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return nil // 非 ELF（脚本/其他格式）放行
	}
	machine := binary.LittleEndian.Uint16(hdr[18:20])
	if machine != want {
		return fmt.Errorf("ELF 架构标识 %d 与当前系统 %s 不匹配", machine, runtime.GOARCH)
	}
	return nil
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
