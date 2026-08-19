package database

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

type runtimeExecCall struct {
	name string
	args []string
}

// fakeRunner 实现 runCmdFunc seam：记录调用参数、返回固定输出/退出码。
// 流式路径（streamRun）不走此 seam，直接用真实命令（见 stream 测试）。
type fakeRunner struct {
	calls  []runtimeExecCall
	out    string
	stderr string
	code   int
}

func (f *fakeRunner) run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	f.calls = append(f.calls, runtimeExecCall{name: name, args: args})
	return f.out, f.stderr, f.code, nil
}

func TestExecSeparatesStderrFromQueryOutput(t *testing.T) {
	// mysql 客户端把警告打到 stderr —— 成功时不得混入查询结果（列表/SQL 输出
	// 是解析目标，必须干净）。
	fake := &fakeRunner{out: "testdb\tutf8mb4\n", stderr: "mysql: [Warning] Using a password on the command line interface can be insecure."}
	runtime := NewCLIContainerRuntime(fake.run)
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
	fake := &fakeRunner{}
	runtime := NewCLIContainerRuntime(fake.run)
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
	// 生命周期命令（如拉镜像/启动）stderr 也可能承载进度/诊断，仍须合并 ——
	// 安装日志依赖整条输出流。
	fake := &fakeRunner{out: "pulling image...", stderr: "extracting layer 3/5"}
	runtime := NewCLIContainerRuntime(fake.run)
	out, err := runtime.command(context.Background(), "docker", "pull", "mysql:8.0")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if !strings.Contains(out, "pulling image...") || !strings.Contains(out, "extracting layer 3/5") {
		t.Fatalf("combined output lost: %q", out)
	}
}

func TestContainerRuntimeCreateUsesStableManagedArguments(t *testing.T) {
	fake := &fakeRunner{}
	runtime := NewCLIContainerRuntime(fake.run)
	err := runtime.Create(context.Background(), ContainerSpec{
		ContainerEngine: "podman",
		Name:            "easyserver-db-mysql-8",
		Image:           "mysql:8.0",
		Volume:          "/opt/easyserver/db/mysql-8.0/data",
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
	// Only one command now — `create`; host dirs are prepared by the panel before
	// create, so no engine-side `volume create` is needed.
	if len(fake.calls) != 1 {
		t.Fatalf("expected a single container create, got %d calls", len(fake.calls))
	}
	if fake.calls[0].name != "podman" || len(fake.calls[0].args) < 3 || fake.calls[0].args[0] != "create" {
		t.Fatalf("unexpected create call: %#v", fake.calls[0])
	}
	// 宿主绝对路径直接挂载（不是命名卷名）。
	joined := strings.Join(fake.calls[0].args, " ")
	if !strings.Contains(joined, "--volume /opt/easyserver/db/mysql-8.0/data:/var/lib/mysql") {
		t.Fatalf("host path volume mount missing:\n%s", joined)
	}

	// Assert the structured contract, not the concatenated CLI args.
	spec := runtime.lastSpec
	if spec.Volume != "/opt/easyserver/db/mysql-8.0/data" || spec.DataDir != "/var/lib/mysql" {
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

func TestRemoveToleratesAlreadyGone(t *testing.T) {
	// 失败安装回滚后重装/卸载会再次删除容器 —— 目标资源已不存在时应视为成功，
	// 而不是让重装流程报错。
	fake := &fakeRunner{code: 1, stderr: `Error: no such container "easyserver-db-mysql-8.0"`}
	rt := NewCLIContainerRuntime(fake.run)

	if err := rt.Remove(context.Background(), "podman", "easyserver-db-mysql-8.0"); err != nil {
		t.Fatalf("Remove of already-gone container should be a no-op: %v", err)
	}
}

func TestStreamRunFeedsHookLineByLine(t *testing.T) {
	// 流式路径（拉镜像等长耗时命令）必须把 stdout+stderr 逐行实时喂给
	// outputHook，而不是等命令结束一次性回放。用真实命令（sh）验证双 pipe 合并。
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	rt := NewCLIContainerRuntime(nil) // 默认 runCmd（executor.Run）
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
	fake := &fakeRunner{out: "Error: No such object: easyserver-db-mysql-8", code: 1}
	rt := NewCLIContainerRuntime(fake.run)

	exists, err := rt.Exists(context.Background(), "podman", "c1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatal("expected not-found container to report exists=false")
	}
}

func TestExistsReportsTrueOnHit(t *testing.T) {
	fake := &fakeRunner{out: "c1"}
	rt := NewCLIContainerRuntime(fake.run)
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
	// 否则会刷出整屏重复状态行（hook=false 分支）。
	fake := &fakeRunner{out: "running|starting"}
	runtime := NewCLIContainerRuntime(fake.run)
	var hooked []string
	runtime.SetOutputHook(func(line string) { hooked = append(hooked, line) })

	if _, err := runtime.Status(context.Background(), "podman", "c1"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(hooked) != 0 {
		t.Fatalf("Status output leaked into output hook: %v", hooked)
	}
}
