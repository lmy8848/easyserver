package database

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	infracontainer "easyserver/internal/infra/container"
)

func TestExecSeparatesStderrFromQueryOutput(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerExecCreateFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, req infracontainer.ExecCreateRequest) (infracontainer.ExecCreateResponse, error) {
			return infracontainer.ExecCreateResponse{ID: "exec-1"}, nil
		},
		ContainerExecStartFn: func(ctx context.Context, engine infracontainer.Engine, execID string, req infracontainer.ExecStartRequest) (io.ReadCloser, error) {
			var buf bytes.Buffer
			// Write multiplexed frames: stdout stream 1, stderr stream 2
			stdoutData := []byte("testdb\tutf8mb4\n")
			stderrData := []byte("mysql: [Warning] Using a password on the command line interface can be insecure.")
			buf.Write([]byte{1, 0, 0, 0, 0, 0, 0, byte(len(stdoutData))})
			buf.Write(stdoutData)
			buf.Write([]byte{2, 0, 0, 0, 0, 0, 0, byte(len(stderrData))})
			buf.Write(stderrData)
			return io.NopCloser(&buf), nil
		},
		ContainerExecInspectFn: func(ctx context.Context, engine infracontainer.Engine, execID string) (infracontainer.ExecInspectResponse, error) {
			return infracontainer.ExecInspectResponse{ExitCode: 0}, nil
		},
	}

	runtime := NewSocketContainerRuntime(mock)
	out, err := runtime.Exec(context.Background(), "docker", "c1", "mysql", "-N", "-B", "-e", "SHOW DATABASES")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out != "testdb\tutf8mb4\n" {
		t.Fatalf("stderr leaked into query output: %q", out)
	}
}

func TestExecHoistsEnvBeforeContainerName(t *testing.T) {
	var capturedReq infracontainer.ExecCreateRequest
	mock := &infracontainer.MockEngineClient{
		ContainerExecCreateFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, req infracontainer.ExecCreateRequest) (infracontainer.ExecCreateResponse, error) {
			capturedReq = req
			return infracontainer.ExecCreateResponse{ID: "exec-2"}, nil
		},
		ContainerExecStartFn: func(ctx context.Context, engine infracontainer.Engine, execID string, req infracontainer.ExecStartRequest) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(nil)), nil
		},
		ContainerExecInspectFn: func(ctx context.Context, engine infracontainer.Engine, execID string) (infracontainer.ExecInspectResponse, error) {
			return infracontainer.ExecInspectResponse{ExitCode: 0}, nil
		},
	}

	runtime := NewSocketContainerRuntime(mock)
	_, err := runtime.Exec(context.Background(), "docker", "c1",
		"-e", "MYSQL_PWD=secret", "mysql", "-uroot", "-N", "-B", "-e", "SHOW DATABASES")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	if len(capturedReq.Env) != 1 || capturedReq.Env[0] != "MYSQL_PWD=secret" {
		t.Fatalf("unexpected hoisted env: %+v", capturedReq.Env)
	}
	if len(capturedReq.Cmd) != 6 || capturedReq.Cmd[0] != "mysql" {
		t.Fatalf("unexpected hoisted cmd: %+v", capturedReq.Cmd)
	}
}

func TestContainerRuntimeCreateUsesStableManagedArguments(t *testing.T) {
	var capturedName string
	var capturedReq infracontainer.ContainerCreateRequest
	mock := &infracontainer.MockEngineClient{
		ContainerCreateFn: func(ctx context.Context, engine infracontainer.Engine, name string, req infracontainer.ContainerCreateRequest) (infracontainer.ContainerCreateResponse, error) {
			capturedName = name
			capturedReq = req
			return infracontainer.ContainerCreateResponse{ID: "c-123"}, nil
		},
	}

	runtime := NewSocketContainerRuntime(mock)
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

	if capturedName != "easyserver-db-mysql-8" {
		t.Fatalf("unexpected container name: %s", capturedName)
	}
	if capturedReq.Image != "mysql:8.0" {
		t.Fatalf("unexpected image: %s", capturedReq.Image)
	}
	if capturedReq.Labels["com.easyserver.managed"] != "true" || capturedReq.Labels["com.easyserver.kind"] != "database" {
		t.Fatalf("missing managed labels: %+v", capturedReq.Labels)
	}
	if len(capturedReq.HostConfig.Binds) != 1 || capturedReq.HostConfig.Binds[0] != "/opt/easyserver/db/mysql-8.0/data:/var/lib/mysql" {
		t.Fatalf("unexpected binds: %+v", capturedReq.HostConfig.Binds)
	}
	portBinds := capturedReq.HostConfig.PortBindings["3306/tcp"]
	if len(portBinds) != 1 || portBinds[0].HostIP != "127.0.0.1" || portBinds[0].HostPort != "3306" {
		t.Fatalf("unexpected port bindings: %+v", portBinds)
	}
	if capturedReq.Healthcheck == nil || len(capturedReq.Healthcheck.Test) != 2 || capturedReq.Healthcheck.Test[1] != "mysqladmin ping -h localhost" {
		t.Fatalf("unexpected healthcheck: %+v", capturedReq.Healthcheck)
	}
}

func TestRemoveToleratesAlreadyGone(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerRemoveFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, force bool) error {
			return errors.New("Error response from daemon: 404 no such container")
		},
	}
	rt := NewSocketContainerRuntime(mock)
	if err := rt.Remove(context.Background(), "podman", "easyserver-db-mysql-8.0"); err != nil {
		t.Fatalf("Remove of already-gone container should be a no-op: %v", err)
	}
}

func TestExistsTreatsNotFoundAsFalse(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerInspectFn: func(ctx context.Context, engine infracontainer.Engine, containerID string) (infracontainer.ContainerInspect, error) {
			return infracontainer.ContainerInspect{}, errors.New("Error: 404 not found")
		},
	}
	rt := NewSocketContainerRuntime(mock)

	exists, err := rt.Exists(context.Background(), "podman", "c1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatal("expected not-found container to report exists=false")
	}
}

func TestExistsReportsTrueOnHit(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerInspectFn: func(ctx context.Context, engine infracontainer.Engine, containerID string) (infracontainer.ContainerInspect, error) {
			return infracontainer.ContainerInspect{ID: "c1"}, nil
		},
	}
	rt := NewSocketContainerRuntime(mock)
	exists, err := rt.Exists(context.Background(), "docker", "c1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected existing container to report exists=true")
	}
}

func TestStatusDoesNotPolluteOutputHook(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerInspectFn: func(ctx context.Context, engine infracontainer.Engine, containerID string) (infracontainer.ContainerInspect, error) {
			var insp infracontainer.ContainerInspect
			insp.State.Status = "running"
			insp.State.Health = &infracontainer.ContainerHealth{Status: "healthy"}
			return insp, nil
		},
	}
	runtime := NewSocketContainerRuntime(mock)
	var hooked []string
	runtime.SetOutputHook(func(line string) { hooked = append(hooked, line) })

	st, err := runtime.Status(context.Background(), "podman", "c1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.State != "running" || st.Health != "healthy" {
		t.Fatalf("unexpected status: %+v", st)
	}
	if len(hooked) != 0 {
		t.Fatalf("Status output leaked into output hook: %v", hooked)
	}
}
