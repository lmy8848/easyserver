package plugin

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperPlugin 是自执行 fake 插件：测试以
// exec(测试二进制, -test.run=TestHelperPlugin) + GO_FAKE_PLUGIN=1 拉起，
// 在 FAKE_SOCK 上提供 /health（GO_FAKE_UNHEALTHY=1 时返回 500，
// GO_FAKE_EXIT=1 时启动 300ms 后自行退出）。
func TestHelperPlugin(t *testing.T) {
	if f, err := os.OpenFile("/tmp/fake-child.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		fmt.Fprintf(f, "child pid=%d GO_FAKE_PLUGIN=%q GO_FAKE_UNHEALTHY=%q GO_FAKE_EXIT=%q FAKE_SOCK=%q\n",
			os.Getpid(), os.Getenv("GO_FAKE_PLUGIN"), os.Getenv("GO_FAKE_UNHEALTHY"),
			os.Getenv("GO_FAKE_EXIT"), os.Getenv("FAKE_SOCK"))
		_ = f.Close()
	}
	if os.Getenv("GO_FAKE_PLUGIN") == "" {
		return
	}
	sock := os.Getenv("FAKE_SOCK")
	log.Printf("fake 插件启动: sock=%s unhealthy=%s exit=%s pid=%d",
		sock, os.Getenv("GO_FAKE_UNHEALTHY"), os.Getenv("GO_FAKE_EXIT"), os.Getpid())
	_ = os.Remove(sock)
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", sock)
	if err != nil {
		os.Exit(3)
	}
	if os.Getenv("GO_FAKE_EXIT") == "1" {
		// 立即退出且不监听：启动期就能确定性地判定 startup-exit
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if os.Getenv("GO_FAKE_UNHEALTHY") == "1" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	_ = srv.Serve(ln)
}

// setupFakePlugin 把测试二进制拷贝为标准插件目录布局
// <pluginsDir>/fake-plugin/fake-bin，并返回指向它的 manifest。
func setupFakePlugin(t *testing.T, pluginsDir, socket string) Manifest {
	t.Helper()
	exec, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "fake-" + strings.NewReplacer("/", "_", "Test", "").Replace(t.Name())
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "fake-bin")
	data, err := os.ReadFile(exec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		t.Fatal(err)
	}

	m := validManifest()
	m.Name = name
	m.Runtime.Exec = "fake-bin"
	m.Runtime.Socket = socket
	m.Runtime.Args = []string{"-test.run=TestHelperPlugin"}
	m.Runtime.StartupTimeout = 5 * time.Second
	m.Health.Interval = 200 * time.Millisecond
	return m
}

// registerFake 预注册插件行（PluginManager 扫描的等价物）；
// SetPluginStatus 是 UPDATE 语义，无行时静默空转，测试需先注册。
func registerFake(t *testing.T, repo PluginRepository, m Manifest, pluginsDir string) {
	t.Helper()
	err := repo.UpsertPlugin(context.Background(), &PluginRecord{
		Name: m.Name, Version: m.Version, APIVersion: m.APIVersion,
		ManifestPath: filepath.Join(pluginsDir, m.Name, "plugin.yaml"),
		Enabled:      true, Status: StateRegistered,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorStartStop(t *testing.T) {
	if os.Getenv("GO_FAKE_PLUGIN") == "1" {
		TestHelperPlugin(t)
		return
	}
	t.Setenv("GO_FAKE_PLUGIN", "1")
	ctx := context.Background()
	repo, _ := testRepo(t)
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	socketDir := filepath.Join(root, "run")
	sup := NewSupervisor(repo, filepath.Join(socketDir, "host.sock"), socketDir, pluginsDir)

	m := setupFakePlugin(t, pluginsDir, "fake-plugin.sock")
	t.Setenv("FAKE_SOCK", filepath.Join(socketDir, "fake-plugin.sock"))
	registerFake(t, repo, m, pluginsDir)

	if err := sup.EnsureRunning(ctx, &m, "token-abc"); err != nil {
		t.Fatalf("拉起失败: %v", err)
	}
	got, _ := repo.GetPlugin(ctx, m.Name)
	if got == nil || got.Status != StateRunning {
		t.Fatalf("状态应为 running: %+v", got)
	}
	// 幂等：重复拉起不报错
	if err := sup.EnsureRunning(ctx, &m, "token-abc"); err != nil {
		t.Fatalf("幂等拉起失败: %v", err)
	}
	if err := sup.StopPlugin(ctx, m.Name); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	got, err2 := repo.GetPlugin(ctx, m.Name)
	if err2 != nil {
		t.Fatalf("停止后查询失败: %v", err2)
	}
	if got == nil {
		t.Fatal("停止后插件行不应消失")
	}
	if got.Status != StateDisabled {
		t.Fatalf("停止后状态应为 disabled: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(socketDir, "fake-plugin.sock")); !os.IsNotExist(err) {
		t.Fatal("停止后 socket 应被清理")
	}
}

func TestSupervisorStartupExit(t *testing.T) {
	if os.Getenv("GO_FAKE_PLUGIN") == "1" {
		t.Setenv("GO_FAKE_EXIT", "1")
		TestHelperPlugin(t)
		return
	}
	t.Setenv("GO_FAKE_PLUGIN", "1")
	t.Setenv("GO_FAKE_EXIT", "1")
	ctx := context.Background()
	repo, _ := testRepo(t)
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	socketDir := filepath.Join(root, "run")
	sup := NewSupervisor(repo, filepath.Join(socketDir, "host.sock"), socketDir, pluginsDir)

	m := setupFakePlugin(t, pluginsDir, "crash.sock")
	t.Setenv("FAKE_SOCK", filepath.Join(socketDir, "crash.sock"))
	registerFake(t, repo, m, pluginsDir)

	if err := sup.EnsureRunning(ctx, &m, "token"); err == nil {
		t.Fatal("启动即退出的插件应返回错误")
	}
	got, _ := repo.GetPlugin(ctx, m.Name)
	if got == nil || got.Status != StateError {
		t.Fatalf("状态应为 error: %+v", got)
	}
	// 阻止 1s 后的退避重启污染后续测试（重启会以当时环境变量拉起子进程）
	_ = sup.StopPlugin(ctx, m.Name)
}

func TestSupervisorUnhealthyRejected(t *testing.T) {
	if os.Getenv("GO_FAKE_PLUGIN") == "1" {
		t.Setenv("GO_FAKE_UNHEALTHY", "1")
		TestHelperPlugin(t)
		return
	}
	t.Setenv("GO_FAKE_PLUGIN", "1")
	t.Setenv("GO_FAKE_UNHEALTHY", "1")
	ctx := context.Background()
	repo, _ := testRepo(t)
	root := t.TempDir()
	pluginsDir := filepath.Join(root, "plugins")
	socketDir := filepath.Join(root, "run")
	sup := NewSupervisor(repo, filepath.Join(socketDir, "host.sock"), socketDir, pluginsDir)

	m := setupFakePlugin(t, pluginsDir, "unhealthy.sock")
	t.Setenv("FAKE_SOCK", filepath.Join(socketDir, "unhealthy.sock"))
	registerFake(t, repo, m, pluginsDir)

	err := sup.EnsureRunning(ctx, &m, "token")
	t.Logf("EnsureRunning err: %v", err)
	if err == nil {
		t.Fatal("健康检查不过的插件不应进入 running")
	}
	got, gerr := repo.GetPlugin(ctx, m.Name)
	if gerr != nil {
		t.Fatalf("查询失败: %v", gerr)
	}
	if got == nil {
		t.Fatal("插件行不应消失")
	}
	t.Logf("最终状态: %s last_error=%q", got.Status, got.LastError)
	if got.Status == StateRunning {
		t.Fatal("不健康插件不应标记 running")
	}
}
