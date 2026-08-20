package container

import (
	"context"
	"io"
	"strings"
)

var _ EngineClient = (*MockEngineClient)(nil)

// MockEngineClient is a test stub implementing EngineClient.
type MockEngineClient struct {
	PingFn                 func(ctx context.Context, engine Engine) (PingResponse, error)
	VersionFn              func(ctx context.Context, engine Engine) (VersionResponse, error)
	InfoFn                 func(ctx context.Context, engine Engine) (map[string]any, error)
	ContainerListFn        func(ctx context.Context, engine Engine, all bool) ([]ContainerSummary, error)
	ContainerInspectFn     func(ctx context.Context, engine Engine, containerID string) (ContainerInspect, error)
	ContainerRenameFn      func(ctx context.Context, engine Engine, containerID string, newName string) error
	ContainerStartFn       func(ctx context.Context, engine Engine, containerID string) error
	ContainerStopFn        func(ctx context.Context, engine Engine, containerID string, timeoutSec int) error
	ContainerRestartFn     func(ctx context.Context, engine Engine, containerID string, timeoutSec int) error
	ContainerPauseFn       func(ctx context.Context, engine Engine, containerID string) error
	ContainerUnpauseFn     func(ctx context.Context, engine Engine, containerID string) error
	ContainerRemoveFn      func(ctx context.Context, engine Engine, containerID string, force bool) error
	ContainerCreateFn      func(ctx context.Context, engine Engine, name string, req ContainerCreateRequest) (ContainerCreateResponse, error)
	ContainerLogsFn        func(ctx context.Context, engine Engine, containerID string, tail int, stdout, stderr bool) (io.ReadCloser, error)
	ContainerStatsFn       func(ctx context.Context, engine Engine, containerID string, stream bool) (io.ReadCloser, error)
	ContainerExecCreateFn  func(ctx context.Context, engine Engine, containerID string, req ExecCreateRequest) (ExecCreateResponse, error)
	ContainerExecStartFn   func(ctx context.Context, engine Engine, execID string, req ExecStartRequest) (io.ReadCloser, error)
	ContainerExecInspectFn func(ctx context.Context, engine Engine, execID string) (ExecInspectResponse, error)
	ImageListFn            func(ctx context.Context, engine Engine) ([]ImageSummary, error)
	ImageInspectFn         func(ctx context.Context, engine Engine, imageID string) (ImageInspect, error)
	ImagePullFn            func(ctx context.Context, engine Engine, imageRef string, authEncoded string) (io.ReadCloser, error)
	ImageRemoveFn          func(ctx context.Context, engine Engine, imageID string, force bool) ([]ImageDeleteResponseItem, error)
	ImagesPruneFn          func(ctx context.Context, engine Engine) (ImagesPruneReport, error)
	VolumeListFn           func(ctx context.Context, engine Engine) (VolumeListResponse, error)
	VolumeInspectFn        func(ctx context.Context, engine Engine, volumeID string) (Volume, error)
	VolumeCreateFn         func(ctx context.Context, engine Engine, req VolumeCreateRequest) (Volume, error)
	VolumeRemoveFn         func(ctx context.Context, engine Engine, volumeID string, force bool) error
	VolumesPruneFn         func(ctx context.Context, engine Engine) (VolumesPruneReport, error)
	NetworkListFn          func(ctx context.Context, engine Engine) ([]NetworkSummary, error)
	NetworkInspectFn       func(ctx context.Context, engine Engine, networkID string) (NetworkSummary, error)
	NetworkCreateFn        func(ctx context.Context, engine Engine, req NetworkCreateRequest) (NetworkCreateResponse, error)
	NetworkRemoveFn        func(ctx context.Context, engine Engine, networkID string) error
	NetworksPruneFn        func(ctx context.Context, engine Engine) (NetworksPruneReport, error)
	CloseFn                func() error
}

func (m *MockEngineClient) Ping(ctx context.Context, engine Engine) (PingResponse, error) {
	if m.PingFn != nil {
		return m.PingFn(ctx, engine)
	}
	return PingResponse{APIVersion: "1.45"}, nil
}

func (m *MockEngineClient) Version(ctx context.Context, engine Engine) (VersionResponse, error) {
	if m.VersionFn != nil {
		return m.VersionFn(ctx, engine)
	}
	return VersionResponse{Version: "27.5.1", APIVersion: "1.45"}, nil
}

func (m *MockEngineClient) Info(ctx context.Context, engine Engine) (map[string]any, error) {
	if m.InfoFn != nil {
		return m.InfoFn(ctx, engine)
	}
	return map[string]any{"ServerVersion": "27.5.1"}, nil
}

func (m *MockEngineClient) ContainerList(ctx context.Context, engine Engine, all bool) ([]ContainerSummary, error) {
	if m.ContainerListFn != nil {
		return m.ContainerListFn(ctx, engine, all)
	}
	return []ContainerSummary{}, nil
}

func (m *MockEngineClient) ContainerInspect(ctx context.Context, engine Engine, containerID string) (ContainerInspect, error) {
	if m.ContainerInspectFn != nil {
		return m.ContainerInspectFn(ctx, engine, containerID)
	}
	var out ContainerInspect
	out.ID = containerID
	return out, nil
}

func (m *MockEngineClient) ContainerRename(ctx context.Context, engine Engine, containerID string, newName string) error {
	if m.ContainerRenameFn != nil {
		return m.ContainerRenameFn(ctx, engine, containerID, newName)
	}
	return nil
}

func (m *MockEngineClient) ContainerStart(ctx context.Context, engine Engine, containerID string) error {
	if m.ContainerStartFn != nil {
		return m.ContainerStartFn(ctx, engine, containerID)
	}
	return nil
}

func (m *MockEngineClient) ContainerStop(ctx context.Context, engine Engine, containerID string, timeoutSec int) error {
	if m.ContainerStopFn != nil {
		return m.ContainerStopFn(ctx, engine, containerID, timeoutSec)
	}
	return nil
}

func (m *MockEngineClient) ContainerRestart(ctx context.Context, engine Engine, containerID string, timeoutSec int) error {
	if m.ContainerRestartFn != nil {
		return m.ContainerRestartFn(ctx, engine, containerID, timeoutSec)
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

func (m *MockEngineClient) ContainerRemove(ctx context.Context, engine Engine, containerID string, force bool) error {
	if m.ContainerRemoveFn != nil {
		return m.ContainerRemoveFn(ctx, engine, containerID, force)
	}
	return nil
}

func (m *MockEngineClient) ContainerCreate(ctx context.Context, engine Engine, name string, req ContainerCreateRequest) (ContainerCreateResponse, error) {
	if m.ContainerCreateFn != nil {
		return m.ContainerCreateFn(ctx, engine, name, req)
	}
	return ContainerCreateResponse{ID: "mock-id-" + name}, nil
}

func (m *MockEngineClient) ContainerLogs(ctx context.Context, engine Engine, containerID string, tail int, stdout, stderr bool) (io.ReadCloser, error) {
	if m.ContainerLogsFn != nil {
		return m.ContainerLogsFn(ctx, engine, containerID, tail, stdout, stderr)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockEngineClient) ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (io.ReadCloser, error) {
	if m.ContainerStatsFn != nil {
		return m.ContainerStatsFn(ctx, engine, containerID, stream)
	}
	return io.NopCloser(strings.NewReader("{}")), nil
}

func (m *MockEngineClient) ContainerExecCreate(ctx context.Context, engine Engine, containerID string, req ExecCreateRequest) (ExecCreateResponse, error) {
	if m.ContainerExecCreateFn != nil {
		return m.ContainerExecCreateFn(ctx, engine, containerID, req)
	}
	return ExecCreateResponse{ID: "mock-exec-id"}, nil
}

func (m *MockEngineClient) ContainerExecStart(ctx context.Context, engine Engine, execID string, req ExecStartRequest) (io.ReadCloser, error) {
	if m.ContainerExecStartFn != nil {
		return m.ContainerExecStartFn(ctx, engine, execID, req)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockEngineClient) ContainerExecInspect(ctx context.Context, engine Engine, execID string) (ExecInspectResponse, error) {
	if m.ContainerExecInspectFn != nil {
		return m.ContainerExecInspectFn(ctx, engine, execID)
	}
	return ExecInspectResponse{ExitCode: 0}, nil
}

func (m *MockEngineClient) ImageList(ctx context.Context, engine Engine) ([]ImageSummary, error) {
	if m.ImageListFn != nil {
		return m.ImageListFn(ctx, engine)
	}
	return []ImageSummary{}, nil
}

func (m *MockEngineClient) ImageInspect(ctx context.Context, engine Engine, imageID string) (ImageInspect, error) {
	if m.ImageInspectFn != nil {
		return m.ImageInspectFn(ctx, engine, imageID)
	}
	return ImageInspect{ID: imageID}, nil
}

func (m *MockEngineClient) ImagePull(ctx context.Context, engine Engine, imageRef string, authEncoded string) (io.ReadCloser, error) {
	if m.ImagePullFn != nil {
		return m.ImagePullFn(ctx, engine, imageRef, authEncoded)
	}
	return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}`)), nil
}

func (m *MockEngineClient) ImageRemove(ctx context.Context, engine Engine, imageID string, force bool) ([]ImageDeleteResponseItem, error) {
	if m.ImageRemoveFn != nil {
		return m.ImageRemoveFn(ctx, engine, imageID, force)
	}
	return []ImageDeleteResponseItem{{Deleted: imageID}}, nil
}

func (m *MockEngineClient) ImagesPrune(ctx context.Context, engine Engine) (ImagesPruneReport, error) {
	if m.ImagesPruneFn != nil {
		return m.ImagesPruneFn(ctx, engine)
	}
	return ImagesPruneReport{}, nil
}

func (m *MockEngineClient) VolumeList(ctx context.Context, engine Engine) (VolumeListResponse, error) {
	if m.VolumeListFn != nil {
		return m.VolumeListFn(ctx, engine)
	}
	return VolumeListResponse{}, nil
}

func (m *MockEngineClient) VolumeInspect(ctx context.Context, engine Engine, volumeID string) (Volume, error) {
	if m.VolumeInspectFn != nil {
		return m.VolumeInspectFn(ctx, engine, volumeID)
	}
	return Volume{Name: volumeID}, nil
}

func (m *MockEngineClient) VolumeCreate(ctx context.Context, engine Engine, req VolumeCreateRequest) (Volume, error) {
	if m.VolumeCreateFn != nil {
		return m.VolumeCreateFn(ctx, engine, req)
	}
	return Volume{Name: req.Name}, nil
}

func (m *MockEngineClient) VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error {
	if m.VolumeRemoveFn != nil {
		return m.VolumeRemoveFn(ctx, engine, volumeID, force)
	}
	return nil
}

func (m *MockEngineClient) VolumesPrune(ctx context.Context, engine Engine) (VolumesPruneReport, error) {
	if m.VolumesPruneFn != nil {
		return m.VolumesPruneFn(ctx, engine)
	}
	return VolumesPruneReport{}, nil
}

func (m *MockEngineClient) NetworkList(ctx context.Context, engine Engine) ([]NetworkSummary, error) {
	if m.NetworkListFn != nil {
		return m.NetworkListFn(ctx, engine)
	}
	return []NetworkSummary{}, nil
}

func (m *MockEngineClient) NetworkInspect(ctx context.Context, engine Engine, networkID string) (NetworkSummary, error) {
	if m.NetworkInspectFn != nil {
		return m.NetworkInspectFn(ctx, engine, networkID)
	}
	return NetworkSummary{ID: networkID, Name: "mock-net"}, nil
}

func (m *MockEngineClient) NetworkCreate(ctx context.Context, engine Engine, req NetworkCreateRequest) (NetworkCreateResponse, error) {
	if m.NetworkCreateFn != nil {
		return m.NetworkCreateFn(ctx, engine, req)
	}
	return NetworkCreateResponse{ID: "mock-net-id"}, nil
}

func (m *MockEngineClient) NetworkRemove(ctx context.Context, engine Engine, networkID string) error {
	if m.NetworkRemoveFn != nil {
		return m.NetworkRemoveFn(ctx, engine, networkID)
	}
	return nil
}

func (m *MockEngineClient) NetworksPrune(ctx context.Context, engine Engine) (NetworksPruneReport, error) {
	if m.NetworksPruneFn != nil {
		return m.NetworksPruneFn(ctx, engine)
	}
	return NetworksPruneReport{}, nil
}

func (m *MockEngineClient) Close() error {
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}
