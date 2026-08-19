package container

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	infracontainer "easyserver/internal/infra/container"
)

func TestMapSummaryToContainer(t *testing.T) {
	summary := infracontainer.ContainerSummary{
		ID:    "c123",
		Names: []string{"/web-server"},
		Image: "nginx:latest",
		State: "running",
		Ports: []infracontainer.PortMapping{
			{IP: "0.0.0.0", PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
			{IP: "127.0.0.1", PublicPort: 8443, PrivatePort: 443, Type: "tcp"},
		},
		Mounts: []infracontainer.ContainerMount{
			{Source: "/host/data", Destination: "/container/data"},
		},
	}
	c := mapSummaryToContainer(summary)
	if c.ID != "c123" || c.Name != "web-server" || c.State != "running" {
		t.Errorf("unexpected mapped container: %+v", c)
	}
	if len(c.Ports) != 2 || c.Ports[0].HostPort != "8080" || c.Ports[1].HostPort != "127.0.0.1:8443" {
		t.Errorf("unexpected mapped ports: %+v", c.Ports)
	}
	if c.Mounts != "/host/data:/container/data" {
		t.Errorf("unexpected mapped mounts: %s", c.Mounts)
	}
}

func TestMapInspectToContainer(t *testing.T) {
	var insp infracontainer.ContainerInspect
	insp.ID = "c456"
	insp.Name = "/db"
	insp.Config.Image = "postgres:15"
	insp.State.Running = true
	insp.State.Status = "running"
	insp.NetworkSettings.Ports = map[string][]infracontainer.PortBinding{
		"5432/tcp": {{HostIP: "0.0.0.0", HostPort: "5432"}},
	}
	c := mapInspectToContainer(insp)
	if c.ID != "c456" || c.Name != "db" || c.State != "running" {
		t.Errorf("unexpected mapped container: %+v", c)
	}
	if len(c.Ports) != 1 || c.Ports[0].HostPort != "5432" {
		t.Errorf("unexpected mapped ports: %+v", c.Ports)
	}
}

func TestService_ContainerOperations_Mock(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ContainerListFn: func(ctx context.Context, engine infracontainer.Engine, all bool) ([]infracontainer.ContainerSummary, error) {
			return []infracontainer.ContainerSummary{
				{ID: "c1", Names: []string{"/app"}, Image: "app:v1", State: "running"},
			}, nil
		},
		ContainerStartFn: func(ctx context.Context, engine infracontainer.Engine, containerID string) error {
			if containerID != "c1" {
				return errors.New("not found")
			}
			return nil
		},
		ContainerCreateFn: func(ctx context.Context, engine infracontainer.Engine, name string, req infracontainer.ContainerCreateRequest) (infracontainer.ContainerCreateResponse, error) {
			return infracontainer.ContainerCreateResponse{ID: "new-id-" + name}, nil
		},
		ContainerLogsFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, tail int, stdout, stderr bool) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("sample log line\n")), nil
		},
		ContainerExecCreateFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, req infracontainer.ExecCreateRequest) (infracontainer.ExecCreateResponse, error) {
			return infracontainer.ExecCreateResponse{ID: "exec-123"}, nil
		},
		ContainerExecStartFn: func(ctx context.Context, engine infracontainer.Engine, execID string, req infracontainer.ExecStartRequest) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("exec output")), nil
		},
		ContainerStatsFn: func(ctx context.Context, engine infracontainer.Engine, containerID string, stream bool) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(`{
				"cpu_stats": {"cpu_usage": {"total_usage": 200000000}, "system_cpu_usage": 200000000, "online_cpus": 2},
				"precpu_stats": {"cpu_usage": {"total_usage": 100000000}, "system_cpu_usage": 100000000, "online_cpus": 2},
				"memory_stats": {"usage": 104857600, "limit": 1073741824},
				"pids_stats": {"current": 10}
			}`)), nil
		},
	}
	infracontainer.SetDefaultClient(mock)
	defer infracontainer.SetDefaultClient(nil)

	svc := NewService()
	list, err := svc.ListContainers(context.Background(), EngineDocker, true)
	if err != nil || len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("ListContainers error: %v, list: %+v", err, list)
	}

	if err := svc.StartContainer(context.Background(), EngineDocker, "c1"); err != nil {
		t.Errorf("StartContainer error: %v", err)
	}

	createdID, err := svc.CreateContainer(context.Background(), EngineDocker, CreateRequest{
		Name:  "my-app",
		Image: "nginx:latest",
	})
	if err != nil || createdID != "new-id-my-app" {
		t.Errorf("CreateContainer error: %v, id: %s", err, createdID)
	}

	logs, err := svc.GetContainerLogs(context.Background(), EngineDocker, "c1", 100)
	if err != nil || logs != "sample log line\n" {
		t.Errorf("GetContainerLogs error: %v, logs: %s", err, logs)
	}

	execOut, err := svc.ExecInContainer(context.Background(), EngineDocker, "c1", "echo hello")
	if err != nil || execOut != "exec output" {
		t.Errorf("ExecInContainer error: %v, output: %s", err, execOut)
	}

	stats, err := svc.GetContainerStats(context.Background(), EngineDocker, "c1")
	if err != nil || stats.PIDs != 10 || stats.MemUsage != 104857600 {
		t.Errorf("GetContainerStats error: %v, stats: %+v", err, stats)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{164982104, "165MB"},
		{1536, "1.5KB"},
		{1 << 20, "1MB"},
		{5 * 1 << 30, "5.4GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.in); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExpandImageRef(t *testing.T) {
	cases := []struct{ in, want string }{
		{"nginx:latest", "docker.io/library/nginx:latest"},
		{"nginx", "docker.io/library/nginx"},
		{"redis:alpine", "docker.io/library/redis:alpine"},
		{"foo/bar:v1", "docker.io/foo/bar:v1"},
		{"docker.io/library/nginx:latest", "docker.io/library/nginx:latest"},
		{"ghcr.io/org/app:tag", "ghcr.io/org/app:tag"},
		{"localhost:5000/app", "localhost:5000/app"},
	}
	for _, tc := range cases {
		if got := expandImageRef(tc.in); got != tc.want {
			t.Errorf("expandImageRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseRegistriesConf(t *testing.T) {
	got := parseRegistriesConf(`
unqualified-search-registries = ["docker.io"]

[[registry]]
location = "registry.local:5000"
insecure = true

[[registry]]
location = "docker.io"
insecure = false
`)
	want := RegistryConfig{
		Mirrors:            []string{"docker.io"},
		InsecureRegistries: []string{"registry.local:5000"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%+v want=%+v", got, want)
	}
}
