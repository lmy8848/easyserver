package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 安装/升级链路的安全与功能测试。

// makePluginZip 生成内存插件 zip：plugin.yaml + exec 文件。
func makePluginZip(t *testing.T, name, version string, execContent []byte, execMode uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mf := "api_version: 1\nmanifest_schema_version: 1\nname: " + name + "\nversion: " + version +
		"\nruntime:\n  exec: " + name + "\n  socket: " + name + ".sock\nhealth:\n  path: /health\npermissions:\n  - log.write\n"
	w, err := zw.Create("plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(mf)); err != nil {
		t.Fatal(err)
	}

	h := &zip.FileHeader{Name: name}
	h.SetMode(os.FileMode(execMode))
	ew, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ew.Write(execContent); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newInstallManager 构造安装测试用 Manager（内存 repo + 临时插件目录）。
func newInstallManager(t *testing.T) (*Manager, string) {
	t.Helper()
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, _ := testRepo(t)
	sup := NewSupervisor(repo, "/tmp/es-test-host.sock", filepath.Join(t.TempDir(), "socks"), pluginsDir)
	m := NewManager(ManagerOptions{
		Repo:       repo,
		Supervisor: sup,
		Storage:    NewPluginStorage(filepath.Join(t.TempDir(), "storage")),
		PluginsDir: pluginsDir,
	})
	return m, pluginsDir
}

func TestInstallRejectsZipSlip(t *testing.T) {
	m, _ := newInstallManager(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()

	if _, _, err := m.InstallFromZip(context.Background(), buf.Bytes()); err == nil {
		t.Fatal("zip slip 路径应被拒绝")
	}
}

func TestInstallRejectsMissingManifest(t *testing.T) {
	m, _ := newInstallManager(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("readme.txt")
	_, _ = w.Write([]byte("no manifest here"))
	_ = zw.Close()

	if _, _, err := m.InstallFromZip(context.Background(), buf.Bytes()); err == nil {
		t.Fatal("缺少 plugin.yaml 应被拒绝")
	}
}

func TestInstallFresh(t *testing.T) {
	m, pluginsDir := newInstallManager(t)
	script := []byte("#!/bin/sh\nsleep 100\n")
	data := makePluginZip(t, "fresh-plugin", "0.1.0", script, 0o755)

	mft, upgraded, err := m.InstallFromZip(context.Background(), data)
	if err != nil {
		t.Fatalf("新装失败: %v", err)
	}
	if upgraded {
		t.Fatal("首次安装 upgraded 应为 false")
	}
	if mft.Name != "fresh-plugin" {
		t.Fatalf("manifest 名称异常: %s", mft.Name)
	}
	// 目录落地 + exec 可执行位保留（Windows 无权限位，跳过该断言）
	info, err := os.Stat(filepath.Join(pluginsDir, "fresh-plugin", "fresh-plugin"))
	if err != nil {
		t.Fatalf("插件目录未落地: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatal("exec 文件应保留可执行位")
	}
	// 注册为 registered 且未启用
	rec, err := m.repo.GetPlugin(context.Background(), "fresh-plugin")
	if err != nil || rec == nil {
		t.Fatalf("未注册: %v", err)
	}
	if rec.Enabled || rec.Status != StateRegistered {
		t.Fatalf("新装应为 registered/未启用: %+v", rec)
	}
}

func TestInstallNonExecFileGets0644(t *testing.T) {
	m, pluginsDir := newInstallManager(t)
	// exec 用脚本（放行），另带一个无执行位的数据文件验证 0644
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mf := "api_version: 1\nmanifest_schema_version: 1\nname: perm-plugin\nversion: 0.1.0\nruntime:\n  exec: perm-plugin\n  socket: p.sock\n"
	w, _ := zw.Create("plugin.yaml")
	_, _ = w.Write([]byte(mf))
	h1 := &zip.FileHeader{Name: "perm-plugin"}
	h1.SetMode(0o755)
	e1, _ := zw.CreateHeader(h1)
	_, _ = e1.Write([]byte("#!/bin/sh\nsleep 100\n"))
	h2 := &zip.FileHeader{Name: "data.txt"}
	h2.SetMode(0o644)
	e2, _ := zw.CreateHeader(h2)
	_, _ = e2.Write([]byte("data"))
	_ = zw.Close()

	if _, _, err := m.InstallFromZip(context.Background(), buf.Bytes()); err != nil {
		t.Fatalf("安装失败: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows 无 unix 权限位，0644 断言在 Linux CI 验证")
	}
	info, err := os.Stat(filepath.Join(pluginsDir, "perm-plugin", "data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("非可执行文件应为 0644，got %o", info.Mode().Perm())
	}
}

func TestUpgradeKeepsConfigAndApproval(t *testing.T) {
	m, pluginsDir := newInstallManager(t)
	ctx := context.Background()
	script := []byte("#!/bin/sh\nsleep 100\n")

	// 首装 v0.1.0 并写入配置
	if _, _, err := m.InstallFromZip(ctx, makePluginZip(t, "up-plugin", "0.1.0", script, 0o755)); err != nil {
		t.Fatalf("首装失败: %v", err)
	}
	if err := m.ConfigSet(ctx, "up-plugin", map[string]string{"token": "abc"}); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	// 模拟管理员已批准子集权限
	if err := m.repo.SetPluginEnabled(ctx, "up-plugin", false, []string{"log.write"}); err != nil {
		t.Fatal(err)
	}

	// 同名 v0.2.0 → 升级路径
	mft, upgraded, err := m.InstallFromZip(ctx, makePluginZip(t, "up-plugin", "0.2.0", script, 0o755))
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}
	if !upgraded {
		t.Fatal("同名安装应走升级路径")
	}
	if mft.Version != "0.2.0" {
		t.Fatalf("升级后 manifest 版本应更新: %s", mft.Version)
	}
	// 配置保留
	cfg, err := m.repo.GetConfig(ctx, "up-plugin", "token")
	if err != nil || cfg != "abc" {
		t.Fatalf("升级应保留配置 KV: %q %v", cfg, err)
	}
	// 审批权限保留
	rec, _ := m.repo.GetPlugin(ctx, "up-plugin")
	if rec == nil || len(rec.PermissionsApproved) != 1 || rec.PermissionsApproved[0] != "log.write" {
		t.Fatalf("升级应保留已审批权限: %+v", rec)
	}
	if rec.Version != "0.2.0" {
		t.Fatalf("注册版本应更新为 0.2.0: %+v", rec)
	}
	// 新目录内容生效（旧目录备份被清理）
	if _, err := os.Stat(filepath.Join(pluginsDir, "up-plugin", "plugin.yaml")); err != nil {
		t.Fatalf("升级后目录异常: %v", err)
	}
	// 无残留 .bak 目录
	entries, _ := os.ReadDir(pluginsDir)
	for _, e := range entries {
		if bytes.Contains([]byte(e.Name()), []byte(".bak-")) {
			t.Fatalf("升级后不应残留备份目录: %s", e.Name())
		}
	}
}

func TestCheckExecutableArch(t *testing.T) {
	dir := t.TempDir()

	// 脚本：放行
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkExecutableArch(script); err != nil {
		t.Fatalf("脚本应放行: %v", err)
	}

	// ELF 架构匹配当前系统：放行；错误架构：拒绝
	cur, wrong := uint16(62), uint16(183)
	if runtime.GOARCH == "arm64" {
		cur, wrong = 183, 62
	}
	elfOK := filepath.Join(dir, "ok.elf")
	hdr := make([]byte, 20)
	copy(hdr, []byte{0x7f, 'E', 'L', 'F'})
	binary.LittleEndian.PutUint16(hdr[18:20], cur)
	if err := os.WriteFile(elfOK, hdr, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkExecutableArch(elfOK); err != nil {
		t.Fatalf("匹配架构 ELF 应放行: %v", err)
	}

	elfBad := filepath.Join(dir, "bad.elf")
	binary.LittleEndian.PutUint16(hdr[18:20], wrong)
	if err := os.WriteFile(elfBad, hdr, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkExecutableArch(elfBad); err == nil {
		t.Fatal("架构不匹配的 ELF 应被拒绝")
	}
}

func TestInstallRejectsWrongArchELF(t *testing.T) {
	m, _ := newInstallManager(t)
	// 伪造架构错误的 ELF 作为 exec
	cur, wrong := uint16(62), uint16(183)
	if runtime.GOARCH == "arm64" {
		cur, wrong = 183, 62
	}
	_ = cur
	hdr := make([]byte, 64)
	copy(hdr, []byte{0x7f, 'E', 'L', 'F'})
	binary.LittleEndian.PutUint16(hdr[18:20], wrong)

	data := makePluginZip(t, "wrongarch", "0.1.0", hdr, 0o755)
	if _, _, err := m.InstallFromZip(context.Background(), data); err == nil {
		t.Fatal("架构不匹配的 ELF 插件应被拒绝")
	}
}
