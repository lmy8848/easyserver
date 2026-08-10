package database

import (
	"context"
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

func TestLifecycleCommandKeepsCombinedOutput(t *testing.T) {
	mock := executor.NewMockExecutor()
	// 生命周期命令（如拉镜像/启动）stderr 也可能承载进度/诊断，仍须合并 ——
	// 安装日志依赖整条输出流。
	mock.SetResponse("docker pull", executor.MockResponse{
		Stdout:   "pulling image...",
		Stderr:   "extracting layer 3/5",
		ExitCode: 0,
	})
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
		Labels:          map[string]string{"com.easyserver.engine": "mysql"},
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
