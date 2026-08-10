package database

import (
	"context"
	"encoding/base64"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"easyserver/internal/infra/executor"
)

type runtimeExecCall struct {
	name string
	args []string
}

type runtimeFakeExecutor struct {
	calls []runtimeExecCall
	out   string
	code  int
}

func (f *runtimeFakeExecutor) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	f.calls = append(f.calls, runtimeExecCall{name: name, args: args})
	return f.out, "", f.code, nil
}
func (f *runtimeFakeExecutor) RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, error) {
	return f.Run(ctx, name, args...)
}
func (f *runtimeFakeExecutor) RunCombined(ctx context.Context, name string, args ...string) (string, int, error) {
	f.calls = append(f.calls, runtimeExecCall{name: name, args: args})
	return f.out, f.code, nil
}
func (f *runtimeFakeExecutor) RunStream(ctx context.Context, onLine func(string), name string, args ...string) (string, int, error) {
	f.calls = append(f.calls, runtimeExecCall{name: name, args: args})
	if onLine != nil {
		for _, l := range strings.Split(f.out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				onLine(l)
			}
		}
	}
	return f.out, f.code, nil
}
func (f *runtimeFakeExecutor) RunWithOptions(ctx context.Context, opts executor.CommandOptions, name string, args ...string) (string, int, error) {
	panic("unused")
}
func (f *runtimeFakeExecutor) Start(context.Context, executor.StartOptions, string, ...string) (executor.Process, error) {
	panic("unused")
}
func (f *runtimeFakeExecutor) Command(context.Context, executor.StartOptions, string, ...string) *exec.Cmd {
	panic("unused")
}
func (f *runtimeFakeExecutor) LookPath(string) (string, error) { return "", nil }

type unusedProcess struct{}

func (*unusedProcess) Pid() int                           { return 0 }
func (*unusedProcess) StdoutPipe() (io.ReadCloser, error) { return nil, nil }
func (*unusedProcess) StderrPipe() (io.ReadCloser, error) { return nil, nil }
func (*unusedProcess) Wait() error                        { return nil }
func (*unusedProcess) Kill() error                        { return nil }
func (*unusedProcess) Signal(syscall.Signal) error        { return nil }
func (*unusedProcess) Cmd() *exec.Cmd                     { return nil }

func TestExecSeparatesStderrFromQueryOutput(t *testing.T) {
	mock := executor.NewMockExecutor()
	// mysql 客户端把警告打到 stderr —— 成功时不得混入查询结果（列表/SQL 输出
	// 是解析目标，必须干净）。
	// MockExecutor 按 `name+空格+args[0]` 匹配：exec 子命令与 pull 子命令分开设置。
	mock.SetResponse("docker exec", executor.MockResponse{
		Stdout:   "testdb\tutf8mb4\n",
		Stderr:   "mysql: [Warning] Using a password on the command line interface can be insecure.",
		ExitCode: 0,
	})
	runtime := NewCLIContainerRuntime(mock)
	out, err := runtime.Exec(context.Background(), "docker", "c1", "mysql", "-N", "-B", "-e", "SHOW DATABASES")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out != "testdb\tutf8mb4\n" {
		t.Fatalf("stderr leaked into query output: %q", out)
	}
}

func TestExecHoistsEnvBeforeContainerName(t *testing.T) {
	// withAdminCredentials 把 MYSQL_PWD 作为 `-e KEY=VAL` 传入；docker exec 的
	// 选项必须位于容器名之前，否则 `-e` 会被当成容器内要执行的命令而失败。
	fake := &runtimeFakeExecutor{}
	runtime := NewCLIContainerRuntime(fake)
	_, err := runtime.Exec(context.Background(), "docker", "c1",
		"-e", "MYSQL_PWD=secret", "mysql", "-uroot", "-N", "-B", "-e", "SHOW DATABASES")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	got := fake.calls[0]
	if got.name != "docker" {
		t.Fatalf("unexpected binary: %q", got.name)
	}
	want := []string{"exec", "-e", "MYSQL_PWD=secret", "c1", "mysql", "-uroot", "-N", "-B", "-e", "SHOW DATABASES"}
	if strings.Join(got.args, " ") != strings.Join(want, " ") {
		t.Fatalf("env not hoisted before container name:\n got %q\nwant %q", got.args, want)
	}
}

func TestLifecycleCommandKeepsCombinedOutput(t *testing.T) {
	mock := executor.NewMockExecutor()
	// 生命周期命令（如拉镜像/启动）stderr 也可能承载进度/诊断，仍须合并 ——
	// 安装日志依赖整条输出流。
	mock.SetResponse("docker pull", executor.MockResponse{
		Stdout:   "pulling image...",
		Stderr:   "extracting layer 3/5",
		ExitCode: 0})
	runtime := NewCLIContainerRuntime(mock)
	out, err := runtime.command(context.Background(), "docker", "pull", "mysql:8.0")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if !strings.Contains(out, "pulling image...") || !strings.Contains(out, "extracting layer 3/5") {
		t.Fatalf("combined output lost: %q", out)
	}
}

func TestContainerRuntimeCreateUsesStableManagedArguments(t *testing.T) {
	fake := &runtimeFakeExecutor{}
	runtime := NewCLIContainerRuntime(fake)
	err := runtime.Create(context.Background(), ContainerSpec{
		ContainerEngine: "podman",
		Name:            "easyserver-db-mysql-8",
		Image:           "mysql:8.0",
		Volume:          "easyserver-db-mysql-8-data",
		DataDir:         "/var/lib/mysql",
		BindAddress:     "127.0.0.1",
		HostPort:        3306,
		ContainerPort:   3306,
		Environment:     map[string]string{"MYSQL_ROOT_PASSWORD": "secret"},
		Labels:          map[string]string{"com.easyserver.dbtype": "mysql"},
		HealthCommand:   "mysqladmin ping -h localhost",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Only one command now — `create`; named volumes are auto-created by the
	// engine on first mount, no explicit `volume create`.
	if len(fake.calls) != 1 {
		t.Fatalf("expected a single container create, got %d calls", len(fake.calls))
	}
	if fake.calls[0].name != "podman" || len(fake.calls[0].args) < 3 || fake.calls[0].args[0] != "create" {
		t.Fatalf("unexpected create call: %#v", fake.calls[0])
	}

	// Assert the structured contract, not the concatenated CLI args.
	spec := runtime.lastSpec
	if spec.Volume != "easyserver-db-mysql-8-data" || spec.DataDir != "/var/lib/mysql" {
		t.Fatalf("data volume mapping lost: volume=%q datadir=%q", spec.Volume, spec.DataDir)
	}
	if spec.BindAddress != "127.0.0.1" {
		t.Fatalf("expected loopback bind by default, got %q", spec.BindAddress)
	}
	if spec.HostPort != 3306 || spec.ContainerPort != 3306 {
		t.Fatalf("unexpected port mapping: %d:%d", spec.HostPort, spec.ContainerPort)
	}
	if spec.Labels["com.easyserver.managed"] != "true" {
		t.Fatalf("managed label not set: %#v", spec.Labels)
	}
}

func TestSeedVolumeFileWritesTemplateIntoVolume(t *testing.T) {
	fake := &runtimeFakeExecutor{}
	runtime := NewCLIContainerRuntime(fake)
	err := runtime.SeedVolumeFile(context.Background(), "podman", "mysql:8.0",
		"easyserver-db-mysql-8-0-config", "easyserver.cnf", "[mysqld]\n")
	if err != nil {
		t.Fatalf("seed volume: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected a single run call, got %d", len(fake.calls))
	}
	got := fake.calls[0]
	if got.name != "podman" || got.args[0] != "run" {
		t.Fatalf("unexpected seed call: %#v", got)
	}
	joined := strings.Join(got.args, " ")
	// 内容走 base64 规避 shell 特殊字符；目标挂在 /easyserver-init —— 一个不遮蔽
	// 镜像配置目录的路径，正式容器把卷挂到配置目录后即可读。
	for _, want := range []string{
		"--rm", "--name", "easyserver-db-mysql-8-0-config-seed",
		"--volume", "easyserver-db-mysql-8-0-config:/easyserver-init",
		"--entrypoint", "/bin/sh", "mysql:8.0", "-c",
		"mkdir -p /easyserver-init/$(dirname easyserver.cnf) &&",
		"| base64 -d > /easyserver-init/easyserver.cnf",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("seed args missing %q:\n%s", want, joined)
		}
	}
	// base64 编码的模板应可解码回原文（含换行与 # 注释）。
	enc := base64.StdEncoding.EncodeToString([]byte("[mysqld]\n"))
	if !strings.Contains(joined, enc) {
		t.Fatalf("template not base64-encoded into command:\n%s", joined)
	}
}

func TestRemoveToleratesAlreadyGone(t *testing.T) {
	mock := executor.NewMockExecutor()
	// 失败安装回滚后重装/卸载会再次删除容器/数据卷 —— 目标资源已不存在时
	// 应视为成功，而不是让重装流程报错。
	mock.SetResponse("podman rm", executor.MockResponse{ExitCode: 1, Stderr: "Error: no such container \"easyserver-db-mysql-8.0\""})
	mock.SetResponse("podman volume", executor.MockResponse{ExitCode: 1, Stderr: "Error: no such volume \"easyserver-db-mysql-8.0-data\""})
	rt := NewCLIContainerRuntime(mock)

	if err := rt.Remove(context.Background(), "podman", "easyserver-db-mysql-8.0"); err != nil {
		t.Fatalf("Remove of already-gone container should be a no-op: %v", err)
	}
	if err := rt.RemoveVolume(context.Background(), "podman", "easyserver-db-mysql-8.0-data"); err != nil {
		t.Fatalf("RemoveVolume of already-gone volume should be a no-op: %v", err)
	}
}

func TestStreamRunFeedsHookLineByLine(t *testing.T) {
	// 流式路径（拉镜像等长耗时命令）必须把 stdout+stderr 逐行实时喂给
	// outputHook，而不是等命令结束一次性回放。用真实 OSExecutor + 无害的
	// sh 命令验证双 pipe 合并。
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	rt := NewCLIContainerRuntime(executor.NewOSExecutor())
	var got []string
	rt.SetOutputHook(func(line string) { got = append(got, line) })

	if _, err := rt.streamRun(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2; sleep 1; echo last-line"); err != nil {
		t.Fatalf("streamRun: %v", err)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"out-line", "err-line", "last-line"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("hook missing %q (stdout+stderr must merge line by line): %v", want, got)
		}
	}
	if len(got) < 3 {
		t.Fatalf("expected all lines streamed, got only %v", got)
	}
}

func TestExistsTreatsNotFoundAsFalse(t *testing.T) {
	// Exists 用 `inspect` 判存在：not-found（docker "No such object" / podman
	// "no such container"）→ false、nil，而不是错误。
	fake := &runtimeFakeExecutor{out: "Error: No such object: easyserver-db-mysql-8", code: 1}
	rt := NewCLIContainerRuntime(fake)

	exists, err := rt.Exists(context.Background(), "podman", "c1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatal("expected not-found container to report exists=false")
	}
}

func TestExistsReportsTrueOnHit(t *testing.T) {
	fake := &runtimeFakeExecutor{out: "c1", code: 0}
	rt := NewCLIContainerRuntime(fake)
	exists, err := rt.Exists(context.Background(), "docker", "c1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected existing container to report exists=true")
	}
}

func TestStatusDoesNotPolluteOutputHook(t *testing.T) {
	// 安装期间 waitForHealthy 每 500ms 轮询一次容器状态；Status 的
	// `podman inspect` 输出（running|starting）绝不能经 outputHook 灌进安装日志，
	// 否则会刷出整屏重复状态行。生命周期命令（start 等）仍须走 hook。
	fake := &runtimeFakeExecutor{out: "running|starting"}
	runtime := NewCLIContainerRuntime(fake)
	var hooked []string
	runtime.SetOutputHook(func(line string) { hooked = append(hooked, line) })

	if _, err := runtime.Status(context.Background(), "podman", "c1"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(hooked) != 0 {
		t.Fatalf("Status output leaked into output hook: %v", hooked)
	}
	if err := runtime.Start(context.Background(), "podman", "c1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(hooked) != 1 || hooked[0] != "running|starting" {
		t.Fatalf("lifecycle command should still fire the hook, got %v", hooked)
	}
}
