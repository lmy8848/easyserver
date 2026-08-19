package container

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestDefaultClient_SetAndGet(t *testing.T) {
	mock := &MockEngineClient{
		PingFn: func(ctx context.Context, engine Engine) (PingResponse, error) {
			if engine == EnginePodman {
				return PingResponse{APIVersion: "1.41"}, nil
			}
			return PingResponse{APIVersion: "1.45"}, nil
		},
	}
	SetDefaultClient(mock)
	defer SetDefaultClient(nil)

	c := DefaultClient()
	if c == nil {
		t.Fatal("DefaultClient returned nil")
	}

	pingDocker, err := c.Ping(context.Background(), EngineDocker)
	if err != nil || pingDocker.APIVersion != "1.45" {
		t.Errorf("unexpected Ping result for docker: %v, %v", pingDocker, err)
	}

	pingPodman, err := c.Ping(context.Background(), EnginePodman)
	if err != nil || pingPodman.APIVersion != "1.41" {
		t.Errorf("unexpected Ping result for podman: %v, %v", pingPodman, err)
	}
}

func TestMockEngineClient_Operations(t *testing.T) {
	mock := &MockEngineClient{
		ContainerListFn: func(ctx context.Context, engine Engine, all bool) ([]ContainerSummary, error) {
			return []ContainerSummary{{ID: "c1", Names: []string{"/web"}}}, nil
		},
		ContainerStartFn: func(ctx context.Context, engine Engine, containerID string) error {
			if containerID != "c1" {
				return errors.New("container not found")
			}
			return nil
		},
	}

	list, err := mock.ContainerList(context.Background(), EngineDocker, true)
	if err != nil || len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("unexpected ContainerList result: %v, %v", list, err)
	}

	if err := mock.ContainerStart(context.Background(), EngineDocker, "c1"); err != nil {
		t.Errorf("ContainerStart failed: %v", err)
	}

	if err := mock.ContainerStart(context.Background(), EngineDocker, "unknown"); err == nil {
		t.Errorf("expected error for unknown container, got nil")
	}
}

func TestNewEngineClient_InvalidEngine(t *testing.T) {
	c := NewEngineClient("", "")
	defer func() { _ = c.Close() }()

	_, err := c.Ping(context.Background(), "invalid_engine")
	if !errors.Is(err, ErrUnsupportedEngine) {
		t.Errorf("expected ErrUnsupportedEngine, got: %v", err)
	}
}

func TestRealClient_UnixSocket_HTTP(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := tmpDir + "/docker_test.sock"

	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix socket error: %v", err)
	}
	defer l.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.45")
		w.Header().Set("OSType", "linux")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VersionResponse{
			Version:    "27.5.1",
			APIVersion: "1.45",
		})
	})
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ContainerSummary{
			{ID: "c1", Names: []string{"/my-container"}, Image: "nginx:latest"},
		})
	})
	mux.HandleFunc("/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ContainerCreateResponse{
			ID: "created-c1",
		})
	})
	mux.HandleFunc("/volumes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VolumeListResponse{
			Volumes: []Volume{{Name: "vol1", Driver: "local"}},
		})
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(l)
	}()
	defer func() { _ = server.Close() }()

	client := NewEngineClient(sockPath, sockPath)
	defer func() { _ = client.Close() }()

	ping, err := client.Ping(context.Background(), EngineDocker)
	if err != nil || ping.APIVersion != "1.45" {
		t.Errorf("Ping failed: %v, resp: %+v", err, ping)
	}

	ver, err := client.Version(context.Background(), EngineDocker)
	if err != nil || ver.Version != "27.5.1" {
		t.Errorf("Version failed: %v, resp: %+v", err, ver)
	}

	containers, err := client.ContainerList(context.Background(), EngineDocker, false)
	if err != nil || len(containers) != 1 || containers[0].ID != "c1" {
		t.Errorf("ContainerList failed: %v, containers: %+v", err, containers)
	}

	created, err := client.ContainerCreate(context.Background(), EngineDocker, "test", ContainerCreateRequest{})
	if err != nil || created.ID != "created-c1" {
		t.Errorf("ContainerCreate failed: %v, created: %+v", err, created)
	}

	vols, err := client.VolumeList(context.Background(), EngineDocker)
	if err != nil || len(vols.Volumes) != 1 || vols.Volumes[0].Name != "vol1" {
		t.Errorf("VolumeList failed: %v, vols: %+v", err, vols)
	}
}
