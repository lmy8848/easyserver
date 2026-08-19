package container

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func TestDefaultClient_SetAndGet(t *testing.T) {
	mock := &MockEngineClient{
		PingFn: func(ctx context.Context, engine Engine) (types.Ping, error) {
			if engine == EnginePodman {
				return types.Ping{APIVersion: "1.41"}, nil
			}
			return types.Ping{APIVersion: "1.45"}, nil
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
		ContainerListFn: func(ctx context.Context, engine Engine, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "c1", Names: []string{"/web"}}}, nil
		},
		ContainerStartFn: func(ctx context.Context, engine Engine, containerID string, options container.StartOptions) error {
			if containerID != "c1" {
				return errors.New("container not found")
			}
			return nil
		},
	}

	list, err := mock.ContainerList(context.Background(), EngineDocker, container.ListOptions{})
	if err != nil || len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("unexpected ContainerList result: %v, %v", list, err)
	}

	if err := mock.ContainerStart(context.Background(), EngineDocker, "c1", container.StartOptions{}); err != nil {
		t.Errorf("ContainerStart failed: %v", err)
	}

	if err := mock.ContainerStart(context.Background(), EngineDocker, "unknown", container.StartOptions{}); err == nil {
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
