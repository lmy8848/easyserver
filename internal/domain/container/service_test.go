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

	// Test exec command failure (non-zero exit code)
	mock.ContainerExecInspectFn = func(ctx context.Context, engine infracontainer.Engine, execID string) (infracontainer.ExecInspectResponse, error) {
		return infracontainer.ExecInspectResponse{ExitCode: 127}, nil
	}
	if _, err := svc.ExecInContainer(context.Background(), EngineDocker, "c1", "unknown_cmd"); err == nil {
		t.Errorf("expected error on non-zero exit code, got nil")
	}

	// Test exec inspect error
	mock.ContainerExecInspectFn = func(ctx context.Context, engine infracontainer.Engine, execID string) (infracontainer.ExecInspectResponse, error) {
		return infracontainer.ExecInspectResponse{}, errors.New("daemon inspect failure")
	}
	if _, err := svc.ExecInContainer(context.Background(), EngineDocker, "c1", "echo fail"); err == nil {
		t.Errorf("expected error on exec inspect failure, got nil")
	}

	// Reset mock inspect
	mock.ContainerExecInspectFn = nil

	stats, err := svc.GetContainerStats(context.Background(), EngineDocker, "c1")
	if err != nil || stats.PIDs != 10 || stats.MemUsage != 104857600 {
		t.Errorf("GetContainerStats error: %v, stats: %+v", err, stats)
	}
}

func TestMapSummaryToImages(t *testing.T) {
	sum := infracontainer.ImageSummary{
		ID:       "img1",
		RepoTags: []string{"nginx:latest", "nginx:1.25"},
		Size:     1024 * 1024 * 50,
	}
	images := mapSummaryToImages(sum)
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0].Repository != "nginx" || images[0].Tag != "latest" {
		t.Errorf("unexpected image 0: %+v", images[0])
	}
	if images[1].Repository != "nginx" || images[1].Tag != "1.25" {
		t.Errorf("unexpected image 1: %+v", images[1])
	}
}

func TestService_ImageOperations_Mock(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		ImageListFn: func(ctx context.Context, engine infracontainer.Engine) ([]infracontainer.ImageSummary, error) {
			return []infracontainer.ImageSummary{
				{ID: "i1", RepoTags: []string{"redis:7.0"}, Size: 10485760},
			}, nil
		},
		ImagePullFn: func(ctx context.Context, engine infracontainer.Engine, imageRef string, authEncoded string) (io.ReadCloser, error) {
			if imageRef == "invalid:image" {
				return io.NopCloser(strings.NewReader(`{"error":"manifest unknown","errorDetail":{"message":"manifest unknown"}}`)), nil
			}
			return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}`)), nil
		},
		ImageRemoveFn: func(ctx context.Context, engine infracontainer.Engine, imageID string, force bool) ([]infracontainer.ImageDeleteResponseItem, error) {
			return []infracontainer.ImageDeleteResponseItem{{Deleted: imageID}}, nil
		},
		ImagesPruneFn: func(ctx context.Context, engine infracontainer.Engine) (infracontainer.ImagesPruneReport, error) {
			return infracontainer.ImagesPruneReport{SpaceReclaimed: 2048}, nil
		},
	}
	infracontainer.SetDefaultClient(mock)
	defer infracontainer.SetDefaultClient(nil)

	svc := NewService()
	images, err := svc.ListImages(context.Background(), EngineDocker)
	if err != nil || len(images) != 1 || images[0].Repository != "redis" {
		t.Fatalf("ListImages error: %v, images: %+v", err, images)
	}

	if err := svc.PullImage(context.Background(), EngineDocker, "redis:7.0"); err != nil {
		t.Errorf("PullImage error: %v", err)
	}

	if err := svc.PullImage(context.Background(), EngineDocker, "invalid:image"); err == nil {
		t.Errorf("expected error for invalid image pull, got nil")
	}

	if err := svc.RemoveImage(context.Background(), EngineDocker, "i1", true); err != nil {
		t.Errorf("RemoveImage error: %v", err)
	}

	report, err := svc.PruneImages(context.Background(), EngineDocker)
	if err != nil || report.SpaceReclaimed != 2048 {
		t.Errorf("PruneImages error: %v, report: %+v", err, report)
	}
}

func TestSplitRepoTag(t *testing.T) {
	cases := []struct {
		in       string
		wantRepo string
		wantTag  string
	}{
		{"nginx:latest", "nginx", "latest"},
		{"nginx:1.25", "nginx", "1.25"},
		{"nginx", "nginx", "latest"},
		{"redis:alpine", "redis", "alpine"},
		{"localhost:5000/app:v1", "localhost:5000/app", "v1"},
		{"localhost:5000/app", "localhost:5000/app", "latest"},
		{"myregistry.com:8443/ns/app:2.0", "myregistry.com:8443/ns/app", "2.0"},
	}
	for _, tc := range cases {
		repo, tag := splitRepoTag(tc.in)
		if repo != tc.wantRepo || tag != tc.wantTag {
			t.Errorf("splitRepoTag(%q) = (%q, %q), want (%q, %q)", tc.in, repo, tag, tc.wantRepo, tc.wantTag)
		}
	}
}

func TestService_VolumeAndNetworkOperations_Mock(t *testing.T) {
	mock := &infracontainer.MockEngineClient{
		VolumeListFn: func(ctx context.Context, engine infracontainer.Engine) (infracontainer.VolumeListResponse, error) {
			return infracontainer.VolumeListResponse{
				Volumes: []infracontainer.Volume{
					{Name: "data_vol", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data_vol/_data"},
				},
			}, nil
		},
		VolumeCreateFn: func(ctx context.Context, engine infracontainer.Engine, req infracontainer.VolumeCreateRequest) (infracontainer.Volume, error) {
			return infracontainer.Volume{Name: req.Name, Driver: req.Driver}, nil
		},
		VolumeRemoveFn: func(ctx context.Context, engine infracontainer.Engine, volumeID string, force bool) error {
			return nil
		},
		VolumesPruneFn: func(ctx context.Context, engine infracontainer.Engine) (infracontainer.VolumesPruneReport, error) {
			return infracontainer.VolumesPruneReport{VolumesDeleted: []string{"unused_vol"}, SpaceReclaimed: 1024}, nil
		},
		NetworkListFn: func(ctx context.Context, engine infracontainer.Engine) ([]infracontainer.NetworkSummary, error) {
			return []infracontainer.NetworkSummary{
				{
					ID:     "net1",
					Name:   "custom-net",
					Driver: "bridge",
					IPAM: infracontainer.IPAM{
						Config: []infracontainer.IPAMConfig{
							{Subnet: "172.20.0.0/16", Gateway: "172.20.0.1"},
						},
					},
				},
			}, nil
		},
		NetworkCreateFn: func(ctx context.Context, engine infracontainer.Engine, req infracontainer.NetworkCreateRequest) (infracontainer.NetworkCreateResponse, error) {
			return infracontainer.NetworkCreateResponse{ID: "net-id-123"}, nil
		},
		NetworkRemoveFn: func(ctx context.Context, engine infracontainer.Engine, networkID string) error {
			return nil
		},
		NetworksPruneFn: func(ctx context.Context, engine infracontainer.Engine) (infracontainer.NetworksPruneReport, error) {
			return infracontainer.NetworksPruneReport{NetworksDeleted: []string{"unused_net"}}, nil
		},
	}
	infracontainer.SetDefaultClient(mock)
	defer infracontainer.SetDefaultClient(nil)

	svc := NewService()

	// Volumes
	vols, err := svc.ListVolumes(context.Background(), EngineDocker)
	if err != nil || len(vols) != 1 || vols[0].Name != "data_vol" {
		t.Fatalf("ListVolumes error: %v, vols: %+v", err, vols)
	}

	if err := svc.CreateVolume(context.Background(), EngineDocker, "new_vol", "local", nil); err != nil {
		t.Errorf("CreateVolume error: %v", err)
	}

	if err := svc.RemoveVolume(context.Background(), EngineDocker, "data_vol", true); err != nil {
		t.Errorf("RemoveVolume error: %v", err)
	}

	volPrune, err := svc.PruneVolumes(context.Background(), EngineDocker)
	if err != nil || len(volPrune.VolumesDeleted) != 1 {
		t.Errorf("PruneVolumes error: %v, report: %+v", err, volPrune)
	}

	// Networks
	nets, err := svc.ListNetworks(context.Background(), EngineDocker)
	if err != nil || len(nets) != 1 || nets[0].Subnet != "172.20.0.0/16" {
		t.Fatalf("ListNetworks error: %v, nets: %+v", err, nets)
	}

	if err := svc.CreateNetwork(context.Background(), EngineDocker, "app-net", "bridge"); err != nil {
		t.Errorf("CreateNetwork error: %v", err)
	}

	if err := svc.RemoveNetwork(context.Background(), EngineDocker, "net1"); err != nil {
		t.Errorf("RemoveNetwork error: %v", err)
	}

	netPrune, err := svc.PruneNetworks(context.Background(), EngineDocker)
	if err != nil || len(netPrune.NetworksDeleted) != 1 {
		t.Errorf("PruneNetworks error: %v, report: %+v", err, netPrune)
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
