package dbserver

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

func TestContainerRuntimeCreateUsesStableManagedArguments(t *testing.T) {
	fake := &runtimeFakeExecutor{}
	runtime := NewCLIContainerRuntime(fake)
	err := runtime.Create(context.Background(), ContainerSpec{
		Runtime:       "podman",
		Name:          "easyserver-db-mysql-8",
		Image:         "mysql:8.0",
		Volume:        "easyserver-db-mysql-8-data",
		DataDir:       "/var/lib/mysql",
		BindAddress:   "127.0.0.1",
		HostPort:      3306,
		ContainerPort: 3306,
		Environment:   map[string]string{"MYSQL_ROOT_PASSWORD": "secret"},
		Labels:        map[string]string{"com.easyserver.engine": "mysql"},
		HealthCommand: "mysqladmin ping -h localhost",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected volume create and container create, got %d calls", len(fake.calls))
	}
	if fake.calls[0].name != "podman" || len(fake.calls[0].args) < 3 || fake.calls[0].args[0] != "volume" {
		t.Fatalf("unexpected volume call: %#v", fake.calls[0])
	}
	joined := strings.Join(fake.calls[1].args, " ")
	for _, want := range []string{"create", "--name easyserver-db-mysql-8", "--label com.easyserver.managed=true", "--publish 127.0.0.1:3306:3306", "--volume easyserver-db-mysql-8-data:/var/lib/mysql", "mysql:8.0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("container args missing %q: %s", want, joined)
		}
	}
}
