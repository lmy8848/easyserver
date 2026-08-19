package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Engine identifies a container runtime engine.
type Engine string

const (
	EngineDocker Engine = "docker"
	EnginePodman Engine = "podman"
)

// Default Unix Socket paths for Docker and Podman.
const (
	DefaultDockerHost = "unix:///var/run/docker.sock"
	DefaultPodmanHost = "unix:///run/podman/podman.sock"
)

var (
	// ErrEngineUnavailable indicates that the container engine daemon/socket is not reachable.
	ErrEngineUnavailable = errors.New("container engine socket is unavailable")
	// ErrUnsupportedEngine indicates an unknown engine type.
	ErrUnsupportedEngine = errors.New("unsupported container engine")
)

// EngineClient abstracts all Docker/Podman Engine API operations for EasyServer.
type EngineClient interface {
	// Ping checks if the specified engine's socket is reachable.
	Ping(ctx context.Context, engine Engine) (types.Ping, error)

	// Info returns system info from the engine daemon.
	Info(ctx context.Context, engine Engine) (system.Info, error)

	// ServerVersion returns version details of the engine.
	ServerVersion(ctx context.Context, engine Engine) (types.Version, error)

	// Container operations
	ContainerList(ctx context.Context, engine Engine, options container.ListOptions) ([]types.Container, error)
	ContainerInspect(ctx context.Context, engine Engine, containerID string) (types.ContainerJSON, error)
	ContainerStart(ctx context.Context, engine Engine, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error
	ContainerPause(ctx context.Context, engine Engine, containerID string) error
	ContainerUnpause(ctx context.Context, engine Engine, containerID string) error
	ContainerRemove(ctx context.Context, engine Engine, containerID string, options container.RemoveOptions) error
	ContainerCreate(ctx context.Context, engine Engine, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerLogs(ctx context.Context, engine Engine, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerExecCreate(ctx context.Context, engine Engine, containerID string, config container.ExecOptions) (types.IDResponse, error)
	ContainerExecAttach(ctx context.Context, engine Engine, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, engine Engine, execID string) (container.ExecInspect, error)

	// Image operations
	ImageList(ctx context.Context, engine Engine, options image.ListOptions) ([]image.Summary, error)
	ImageInspect(ctx context.Context, engine Engine, imageID string) (types.ImageInspect, error)
	ImagePull(ctx context.Context, engine Engine, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, engine Engine, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
	ImagesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (image.PruneReport, error)

	// Volume operations
	VolumeList(ctx context.Context, engine Engine, options volume.ListOptions) (volume.ListResponse, error)
	VolumeInspect(ctx context.Context, engine Engine, volumeID string) (volume.Volume, error)
	VolumeCreate(ctx context.Context, engine Engine, options volume.CreateOptions) (volume.Volume, error)
	VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error
	VolumesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (volume.PruneReport, error)

	// Network operations
	NetworkList(ctx context.Context, engine Engine, options network.ListOptions) ([]network.Summary, error)
	NetworkInspect(ctx context.Context, engine Engine, networkID string, options network.InspectOptions) (network.Inspect, error)
	NetworkCreate(ctx context.Context, engine Engine, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, engine Engine, networkID string) error
	NetworksPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (network.PruneReport, error)

	// Close closes all underlying engine socket connections.
	Close() error
}

type realClient struct {
	mu         sync.Mutex
	dockerHost string
	podmanHost string
	dockerCli  *client.Client
	podmanCli  *client.Client
}

var (
	globalClient EngineClient
	globalMu     sync.Mutex
)

// DefaultClient returns the global singleton EngineClient instance.
func DefaultClient() EngineClient {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalClient == nil {
		globalClient = NewEngineClient(DefaultDockerHost, DefaultPodmanHost)
	}
	return globalClient
}

// SetDefaultClient sets the global singleton EngineClient (useful for dependency injection in tests).
func SetDefaultClient(c EngineClient) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalClient = c
}

// NewEngineClient creates a new EngineClient with configurable socket paths and lazy connection initialization.
func NewEngineClient(dockerHost, podmanHost string) EngineClient {
	if dockerHost == "" {
		dockerHost = DefaultDockerHost
	}
	if podmanHost == "" {
		podmanHost = DefaultPodmanHost
	}
	return &realClient{
		dockerHost: dockerHost,
		podmanHost: podmanHost,
	}
}

func (c *realClient) getCli(engine Engine) (*client.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch engine {
	case EngineDocker, "":
		if c.dockerCli != nil {
			return c.dockerCli, nil
		}
		cli, err := client.NewClientWithOpts(
			client.WithHost(c.dockerHost),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrEngineUnavailable, err)
		}
		c.dockerCli = cli
		return c.dockerCli, nil

	case EnginePodman:
		if c.podmanCli != nil {
			return c.podmanCli, nil
		}
		cli, err := client.NewClientWithOpts(
			client.WithHost(c.podmanHost),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrEngineUnavailable, err)
		}
		c.podmanCli = cli
		return c.podmanCli, nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEngine, engine)
	}
}

func (c *realClient) Ping(ctx context.Context, engine Engine) (types.Ping, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.Ping{}, err
	}
	return cli.Ping(ctx)
}

func (c *realClient) Info(ctx context.Context, engine Engine) (system.Info, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return system.Info{}, err
	}
	return cli.Info(ctx)
}

func (c *realClient) ServerVersion(ctx context.Context, engine Engine) (types.Version, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.Version{}, err
	}
	return cli.ServerVersion(ctx)
}

// Container methods

func (c *realClient) ContainerList(ctx context.Context, engine Engine, options container.ListOptions) ([]types.Container, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.ContainerList(ctx, options)
}

func (c *realClient) ContainerInspect(ctx context.Context, engine Engine, containerID string) (types.ContainerJSON, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.ContainerJSON{}, err
	}
	return cli.ContainerInspect(ctx, containerID)
}

func (c *realClient) ContainerStart(ctx context.Context, engine Engine, containerID string, options container.StartOptions) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerStart(ctx, containerID, options)
}

func (c *realClient) ContainerStop(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerStop(ctx, containerID, options)
}

func (c *realClient) ContainerRestart(ctx context.Context, engine Engine, containerID string, options container.StopOptions) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerRestart(ctx, containerID, options)
}

func (c *realClient) ContainerPause(ctx context.Context, engine Engine, containerID string) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerPause(ctx, containerID)
}

func (c *realClient) ContainerUnpause(ctx context.Context, engine Engine, containerID string) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerUnpause(ctx, containerID)
}

func (c *realClient) ContainerRemove(ctx context.Context, engine Engine, containerID string, options container.RemoveOptions) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.ContainerRemove(ctx, containerID, options)
}

func (c *realClient) ContainerCreate(ctx context.Context, engine Engine, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return container.CreateResponse{}, err
	}
	return cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (c *realClient) ContainerLogs(ctx context.Context, engine Engine, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.ContainerLogs(ctx, containerID, options)
}

func (c *realClient) ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (container.StatsResponseReader, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return container.StatsResponseReader{}, err
	}
	return cli.ContainerStats(ctx, containerID, stream)
}

func (c *realClient) ContainerExecCreate(ctx context.Context, engine Engine, containerID string, config container.ExecOptions) (types.IDResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.IDResponse{}, err
	}
	return cli.ContainerExecCreate(ctx, containerID, config)
}

func (c *realClient) ContainerExecAttach(ctx context.Context, engine Engine, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.HijackedResponse{}, err
	}
	return cli.ContainerExecAttach(ctx, execID, config)
}

func (c *realClient) ContainerExecInspect(ctx context.Context, engine Engine, execID string) (container.ExecInspect, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return container.ExecInspect{}, err
	}
	return cli.ContainerExecInspect(ctx, execID)
}

// Image methods

func (c *realClient) ImageList(ctx context.Context, engine Engine, options image.ListOptions) ([]image.Summary, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.ImageList(ctx, options)
}

func (c *realClient) ImageInspect(ctx context.Context, engine Engine, imageID string) (types.ImageInspect, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return types.ImageInspect{}, err
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, imageID)
	return inspect, err
}

func (c *realClient) ImagePull(ctx context.Context, engine Engine, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.ImagePull(ctx, refStr, options)
}

func (c *realClient) ImageRemove(ctx context.Context, engine Engine, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.ImageRemove(ctx, imageID, options)
}

func (c *realClient) ImagesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (image.PruneReport, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return image.PruneReport{}, err
	}
	return cli.ImagesPrune(ctx, pruneFilters)
}

// Volume methods

func (c *realClient) VolumeList(ctx context.Context, engine Engine, options volume.ListOptions) (volume.ListResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return volume.ListResponse{}, err
	}
	return cli.VolumeList(ctx, options)
}

func (c *realClient) VolumeInspect(ctx context.Context, engine Engine, volumeID string) (volume.Volume, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return volume.Volume{}, err
	}
	return cli.VolumeInspect(ctx, volumeID)
}

func (c *realClient) VolumeCreate(ctx context.Context, engine Engine, options volume.CreateOptions) (volume.Volume, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return volume.Volume{}, err
	}
	return cli.VolumeCreate(ctx, options)
}

func (c *realClient) VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.VolumeRemove(ctx, volumeID, force)
}

func (c *realClient) VolumesPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (volume.PruneReport, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return volume.PruneReport{}, err
	}
	return cli.VolumesPrune(ctx, pruneFilters)
}

// Network methods

func (c *realClient) NetworkList(ctx context.Context, engine Engine, options network.ListOptions) ([]network.Summary, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return nil, err
	}
	return cli.NetworkList(ctx, options)
}

func (c *realClient) NetworkInspect(ctx context.Context, engine Engine, networkID string, options network.InspectOptions) (network.Inspect, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return network.Inspect{}, err
	}
	return cli.NetworkInspect(ctx, networkID, options)
}

func (c *realClient) NetworkCreate(ctx context.Context, engine Engine, name string, options network.CreateOptions) (network.CreateResponse, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return network.CreateResponse{}, err
	}
	return cli.NetworkCreate(ctx, name, options)
}

func (c *realClient) NetworkRemove(ctx context.Context, engine Engine, networkID string) error {
	cli, err := c.getCli(engine)
	if err != nil {
		return err
	}
	return cli.NetworkRemove(ctx, networkID)
}

func (c *realClient) NetworksPrune(ctx context.Context, engine Engine, pruneFilters filters.Args) (network.PruneReport, error) {
	cli, err := c.getCli(engine)
	if err != nil {
		return network.PruneReport{}, err
	}
	return cli.NetworksPrune(ctx, pruneFilters)
}

func (c *realClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	if c.dockerCli != nil {
		if err := c.dockerCli.Close(); err != nil {
			errs = append(errs, err)
		}
		c.dockerCli = nil
	}
	if c.podmanCli != nil {
		if err := c.podmanCli.Close(); err != nil {
			errs = append(errs, err)
		}
		c.podmanCli = nil
	}
	return errors.Join(errs...)
}
