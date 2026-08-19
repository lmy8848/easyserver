package container

import (
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// MockEngineClient is a test stub implementing EngineClient.
type MockEngineClient struct {
	PingFn                 func(ctx context.Context, engine Engine) (types.Ping, error)
	InfoFn                 func(ctx context.Context, engine Engine) (system.Info, error)
	ServerVersionFn        func(ctx context.Context, engine Engine) (types.Version, error)
	ContainerListFn        func(ctx context.Context, engine Engine, options container.ListOptions) ([]types.Container, error)
	ContainerInspectFn     func(ctx context.Context, engine Engine, containerID string) (types.ContainerJSON, error)
	ContainerStartFn       func(ctx context.Context, engine Engine, containerID string, options container.StartOptions) error
	ContainerStopFn        func(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error
	ContainerRestartFn     func(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error
	ContainerPauseFn       func(ctx context.Context, engine Engine, containerID string) error
	ContainerUnpauseFn     func(ctx context.Context, engine Engine, containerID string) error
	ContainerRemoveFn      func(ctx context.Context, engine Engine, containerID string, options container.RemoveOptions) error
	ContainerCreateFn      func(ctx context.Context, engine Engine, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerLogsFn        func(ctx context.Context, engine Engine, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerStatsFn       func(ctx context.Context, engine Engine, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerExecCreateFn  func(ctx context.Context, engine Engine, containerID string, config container.ExecOptions) (types.IDResponse, error)
	ContainerExecAttachFn  func(ctx context.Context, engine Engine, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspectFn func(ctx context.Context, engine Engine, execID string) (container.ExecInspect, error)
	ImageListFn            func(ctx context.Context, engine Engine, options image.ListOptions) ([]image.Summary, error)
	ImageInspectFn         func(ctx context.Context, engine Engine, imageID string) (types.ImageInspect, error)
	ImagePullFn            func(ctx context.Context, engine Engine, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ImageRemoveFn          func(ctx context.Context, engine Engine, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
	ImagesPruneFn          func(ctx context.Context, engine Engine, pruneFilters filters.Args) (image.PruneReport, error)
	VolumeListFn           func(ctx context.Context, engine Engine, options volume.ListOptions) (volume.ListResponse, error)
	VolumeInspectFn        func(ctx context.Context, engine Engine, volumeID string) (volume.Volume, error)
	VolumeCreateFn         func(ctx context.Context, engine Engine, options volume.CreateOptions) (volume.Volume, error)
	VolumeRemoveFn         func(ctx context.Context, engine Engine, volumeID string, force bool) error
	VolumesPruneFn         func(ctx context.Context, engine Engine, pruneFilters filters.Args) (volume.PruneReport, error)
	NetworkListFn          func(ctx context.Context, engine Engine, options network.ListOptions) ([]network.Summary, error)
	NetworkInspectFn       func(ctx context.Context, engine Engine, networkID string, options network.InspectOptions) (network.Inspect, error)
	NetworkCreateFn        func(ctx context.Context, engine Engine, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkRemoveFn        func(ctx context.Context, engine Engine, networkID string) error
	NetworksPruneFn        func(ctx context.Context, engine Engine, pruneFilters filters.Args) (network.PruneReport, error)
	CloseFn                func() error
}

func (m *MockEngineClient) Ping(ctx context.Context, engine Engine) (types.Ping, error) {
	if m.PingFn != nil {
		return m.PingFn(ctx, engine)
	}
	return types.Ping{APIVersion: "1.45"}, nil
}

func (m *MockEngineClient) Info(ctx context.Context, engine Engine) (system.Info, error) {
	if m.InfoFn != nil {
		return m.InfoFn(ctx, engine)
	}
	return system.Info{ServerVersion: "27.5.1"}, nil
}

func (m *MockEngineClient) ServerVersion(ctx context.Context, engine Engine) (types.Version, error) {
	if m.ServerVersionFn != nil {
		return m.ServerVersionFn(ctx, engine)
	}
	return types.Version{Version: "27.5.1", APIVersion: "1.45"}, nil
}

func (m *MockEngineClient) ContainerList(ctx context.Context, engine Engine, options container.ListOptions) ([]types.Container, error) {
	if m.ContainerListFn != nil {
		return m.ContainerListFn(ctx, engine, options)
	}
	return []types.Container{}, nil
}

func (m *MockEngineClient) ContainerInspect(ctx context.Context, engine Engine, containerID string) (types.ContainerJSON, error) {
	if m.ContainerInspectFn != nil {
		return m.ContainerInspectFn(ctx, engine, containerID)
	}
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: containerID}}, nil
}

func (m *MockEngineClient) ContainerStart(ctx context.Context, engine Engine, containerID string, options container.StartOptions) error {
	if m.ContainerStartFn != nil {
		return m.ContainerStartFn(ctx, engine, containerID, options)
	}
	return nil
}

func (m *MockEngineClient) ContainerStop(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error {
	if m.ContainerStopFn != nil {
		return m.ContainerStopFn(ctx, engine, containerID, options)
	}
	return nil
}

func (m *MockEngineClient) ContainerRestart(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error {
	if m.ContainerRestartFn != nil {
		return m.ContainerRestartFn(ctx, engine, containerID, options)
	}
	return nil
}

func (m *MockEngineClient) ContainerPause(ctx context.Context, engine Engine, containerID string) error {
	if m.ContainerPauseFn != nil {
		return m.ContainerPauseFn(ctx, engine, containerID)
	}
	return nil
}

func (m *MockEngineClient) ContainerUnpause(ctx context.Context, engine Engine, containerID string) error {
	if m.ContainerUnpauseFn != nil {
		return m.ContainerUnpauseFn(ctx, engine, containerID)
	}
	return nil
}

func (m *MockEngineClient) ContainerRemove(ctx context.Context, engine Engine, containerID string, options container.RemoveOptions) error {
	if m.ContainerRemoveFn != nil {
		return m.ContainerRemoveFn(ctx, engine, containerID, options)
	}
	return nil
}

func (m *MockEngineClient) ContainerCreate(ctx context.Context, engine Engine, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error) {
	if m.ContainerCreateFn != nil {
		return m.ContainerCreateFn(ctx, engine, config, hostConfig, networkingConfig, platform, containerName)
	}
	return container.CreateResponse{ID: "mock-id-" + containerName}, nil
}

func (m *MockEngineClient) ContainerLogs(ctx context.Context, engine Engine, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFn != nil {
		return m.ContainerLogsFn(ctx, engine, containerID, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockEngineClient) ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.ContainerStatsFn != nil {
		return m.ContainerStatsFn(ctx, engine, containerID, stream)
	}
	return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func (m *MockEngineClient) ContainerExecCreate(ctx context.Context, engine Engine, containerID string, config container.ExecOptions) (types.IDResponse, error) {
	if m.ContainerExecCreateFn != nil {
		return m.ContainerExecCreateFn(ctx, engine, containerID, config)
	}
	return types.IDResponse{ID: "mock-exec-id"}, nil
}

func (m *MockEngineClient) ContainerExecAttach(ctx context.Context, engine Engine, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error) {
	if m.ContainerExecAttachFn != nil {
		return m.ContainerExecAttachFn(ctx, engine, execID, config)
	}
	return types.HijackedResponse{}, nil
}

func (m *MockEngineClient) ContainerExecInspect(ctx context.Context, engine Engine, execID string) (container.ExecInspect, error) {
	if m.ContainerExecInspectFn != nil {
		return m.ContainerExecInspectFn(ctx, engine, execID)
	}
	return container.ExecInspect{ExitCode: 0}, nil
}

func (m *MockEngineClient) ImageList(ctx context.Context, engine Engine, options image.ListOptions) ([]image.Summary, error) {
	if m.ImageListFn != nil {
		return m.ImageListFn(ctx, engine, options)
	}
	return []image.Summary{}, nil
}

func (m *MockEngineClient) ImageInspect(ctx context.Context, engine Engine, imageID string) (types.ImageInspect, error) {
	if m.ImageInspectFn != nil {
		return m.ImageInspectFn(ctx, engine, imageID)
	}
	return types.ImageInspect{ID: imageID}, nil
}

func (m *MockEngineClient) ImagePull(ctx context.Context, engine Engine, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	if m.ImagePullFn != nil {
		return m.ImagePullFn(ctx, engine, refStr, options)
	}
	return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}`)), nil
}

func (m *MockEngineClient) ImageRemove(ctx context.Context, engine Engine, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	if m.ImageRemoveFn != nil {
		return m.ImageRemoveFn(ctx, engine, imageID, options)
	}
	return []image.DeleteResponse{{Deleted: imageID}}, nil
}

func (m *MockEngineClient) ImagesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (image.PruneReport, error) {
	if m.ImagesPruneFn != nil {
		return m.ImagesPruneFn(ctx, engine, pruneFilters)
	}
	return image.PruneReport{}, nil
}

func (m *MockEngineClient) VolumeList(ctx context.Context, engine Engine, options volume.ListOptions) (volume.ListResponse, error) {
	if m.VolumeListFn != nil {
		return m.VolumeListFn(ctx, engine, options)
	}
	return volume.ListResponse{}, nil
}

func (m *MockEngineClient) VolumeInspect(ctx context.Context, engine Engine, volumeID string) (volume.Volume, error) {
	if m.VolumeInspectFn != nil {
		return m.VolumeInspectFn(ctx, engine, volumeID)
	}
	return volume.Volume{Name: volumeID}, nil
}

func (m *MockEngineClient) VolumeCreate(ctx context.Context, engine Engine, options volume.CreateOptions) (volume.Volume, error) {
	if m.VolumeCreateFn != nil {
		return m.VolumeCreateFn(ctx, engine, options)
	}
	return volume.Volume{Name: options.Name}, nil
}

func (m *MockEngineClient) VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error {
	if m.VolumeRemoveFn != nil {
		return m.VolumeRemoveFn(ctx, engine, volumeID, force)
	}
	return nil
}

func (m *MockEngineClient) VolumesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (volume.PruneReport, error) {
	if m.VolumesPruneFn != nil {
		return m.VolumesPruneFn(ctx, engine, pruneFilters)
	}
	return volume.PruneReport{}, nil
}

func (m *MockEngineClient) NetworkList(ctx context.Context, engine Engine, options network.ListOptions) ([]network.Summary, error) {
	if m.NetworkListFn != nil {
		return m.NetworkListFn(ctx, engine, options)
	}
	return []network.Summary{}, nil
}

func (m *MockEngineClient) NetworkInspect(ctx context.Context, engine Engine, networkID string, options network.InspectOptions) (network.Inspect, error) {
	if m.NetworkInspectFn != nil {
		return m.NetworkInspectFn(ctx, engine, networkID, options)
	}
	return network.Inspect{ID: networkID, Name: "mock-net"}, nil
}

func (m *MockEngineClient) NetworkCreate(ctx context.Context, engine Engine, name string, options network.CreateOptions) (network.CreateResponse, error) {
	if m.NetworkCreateFn != nil {
		return m.NetworkCreateFn(ctx, engine, name, options)
	}
	return network.CreateResponse{ID: "mock-net-id"}, nil
}

func (m *MockEngineClient) NetworkRemove(ctx context.Context, engine Engine, networkID string) error {
	if m.NetworkRemoveFn != nil {
		return m.NetworkRemoveFn(ctx, engine, networkID)
	}
	return nil
}

func (m *MockEngineClient) NetworksPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (network.PruneReport, error) {
	if m.NetworksPruneFn != nil {
		return m.NetworksPruneFn(ctx, engine, pruneFilters)
	}
	return network.PruneReport{}, nil
}

func (m *MockEngineClient) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}
