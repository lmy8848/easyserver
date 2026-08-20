package container

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrEngineUnavailable indicates that the container engine daemon/socket is not reachable.
	ErrEngineUnavailable = errors.New("container engine socket is unavailable")
	// ErrUnsupportedEngine indicates an unknown engine type.
	ErrUnsupportedEngine = errors.New("unsupported container engine")
)

// EngineClient defines all container engine operations needed by EasyServer.
type EngineClient interface {
	// Ping checks if the specified engine's socket is reachable.
	Ping(ctx context.Context, engine Engine) (PingResponse, error)

	// Version returns version details of the engine.
	Version(ctx context.Context, engine Engine) (VersionResponse, error)

	// Container operations
	ContainerList(ctx context.Context, engine Engine, all bool) ([]ContainerSummary, error)
	ContainerInspect(ctx context.Context, engine Engine, containerID string) (ContainerInspect, error)
	ContainerStart(ctx context.Context, engine Engine, containerID string) error
	ContainerStop(ctx context.Context, engine Engine, containerID string, timeoutSec int) error
	ContainerRestart(ctx context.Context, engine Engine, containerID string, timeoutSec int) error
	ContainerPause(ctx context.Context, engine Engine, containerID string) error
	ContainerUnpause(ctx context.Context, engine Engine, containerID string) error
	ContainerRemove(ctx context.Context, engine Engine, containerID string, force bool) error
	ContainerCreate(ctx context.Context, engine Engine, name string, req ContainerCreateRequest) (ContainerCreateResponse, error)
	ContainerLogs(ctx context.Context, engine Engine, containerID string, tail int, stdout, stderr bool) (io.ReadCloser, error)
	ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (io.ReadCloser, error)
	ContainerExecCreate(ctx context.Context, engine Engine, containerID string, req ExecCreateRequest) (ExecCreateResponse, error)
	ContainerExecStart(ctx context.Context, engine Engine, execID string, req ExecStartRequest) (io.ReadCloser, error)
	ContainerExecInspect(ctx context.Context, engine Engine, execID string) (ExecInspectResponse, error)

	// Image operations
	ImageList(ctx context.Context, engine Engine) ([]ImageSummary, error)
	ImageInspect(ctx context.Context, engine Engine, imageID string) (ImageInspect, error)
	ImagePull(ctx context.Context, engine Engine, imageRef string, authEncoded string) (io.ReadCloser, error)
	ImageRemove(ctx context.Context, engine Engine, imageID string, force bool) ([]ImageDeleteResponseItem, error)
	ImagesPrune(ctx context.Context, engine Engine) (ImagesPruneReport, error)

	// Volume operations
	VolumeList(ctx context.Context, engine Engine) (VolumeListResponse, error)
	VolumeInspect(ctx context.Context, engine Engine, volumeID string) (Volume, error)
	VolumeCreate(ctx context.Context, engine Engine, req VolumeCreateRequest) (Volume, error)
	VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error
	VolumesPrune(ctx context.Context, engine Engine) (VolumesPruneReport, error)

	// Network operations
	NetworkList(ctx context.Context, engine Engine) ([]NetworkSummary, error)
	NetworkInspect(ctx context.Context, engine Engine, networkID string) (NetworkSummary, error)
	NetworkCreate(ctx context.Context, engine Engine, req NetworkCreateRequest) (NetworkCreateResponse, error)
	NetworkRemove(ctx context.Context, engine Engine, networkID string) error
	NetworksPrune(ctx context.Context, engine Engine) (NetworksPruneReport, error)

	// Close closes any idle connections.
	Close() error
}

type realClient struct {
	mu           sync.Mutex
	dockerSocket string
	podmanSocket string
	dockerHTTP   *http.Client
	podmanHTTP   *http.Client
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

// NewEngineClient creates a new pure Go stdlib EngineClient with configurable Unix socket paths.
func NewEngineClient(dockerSocket, podmanSocket string) EngineClient {
	if dockerSocket == "" {
		dockerSocket = DefaultDockerHost
	}
	if podmanSocket == "" {
		podmanSocket = DefaultPodmanHost
	}
	return &realClient{
		dockerSocket: dockerSocket,
		podmanSocket: podmanSocket,
	}
}

func newUnixHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
			},
			DisableKeepAlives: false,
			MaxIdleConns:      10,
			IdleConnTimeout:   90 * time.Second,
		},
	}
}

func (c *realClient) getHTTP(engine Engine) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch engine {
	case EngineDocker, "":
		if c.dockerHTTP == nil {
			c.dockerHTTP = newUnixHTTPClient(c.dockerSocket)
		}
		return c.dockerHTTP, nil
	case EnginePodman:
		if c.podmanHTTP == nil {
			c.podmanHTTP = newUnixHTTPClient(c.podmanSocket)
		}
		return c.podmanHTTP, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEngine, engine)
	}
}

func (c *realClient) do(ctx context.Context, engine Engine, method, path string, query url.Values, body any, headers map[string]string) (*http.Response, error) {
	client, err := c.getHTTP(engine)
	if err != nil {
		return nil, err
	}

	u := "http://localhost" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEngineUnavailable, err)
	}
	return resp, nil
}

func parseJSON[T any](resp *http.Response) (T, error) {
	defer resp.Body.Close()
	var out T
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("engine API error (status %d): %s", resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode json: %w", err)
	}
	return out, nil
}

func checkStatus(resp *http.Response, expectedCodes ...int) error {
	defer resp.Body.Close()
	if slices.Contains(expectedCodes, resp.StatusCode) {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("engine API error (status %d): %s", resp.StatusCode, string(body))
}

// System methods

func (c *realClient) Ping(ctx context.Context, engine Engine) (PingResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/_ping", nil, nil, nil)
	if err != nil {
		return PingResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PingResponse{}, fmt.Errorf("ping status: %d", resp.StatusCode)
	}
	return PingResponse{
		APIVersion: resp.Header.Get("API-Version"),
		OSType:     resp.Header.Get("OSType"),
	}, nil
}

func (c *realClient) Version(ctx context.Context, engine Engine) (VersionResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/version", nil, nil, nil)
	if err != nil {
		return VersionResponse{}, err
	}
	return parseJSON[VersionResponse](resp)
}

// Container methods

func (c *realClient) ContainerList(ctx context.Context, engine Engine, all bool) ([]ContainerSummary, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	resp, err := c.do(ctx, engine, http.MethodGet, "/containers/json", q, nil, nil)
	if err != nil {
		return nil, err
	}
	return parseJSON[[]ContainerSummary](resp)
}

func (c *realClient) ContainerInspect(ctx context.Context, engine Engine, containerID string) (ContainerInspect, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/json", nil, nil, nil)
	if err != nil {
		return ContainerInspect{}, err
	}
	return parseJSON[ContainerInspect](resp)
}

func (c *realClient) ContainerStart(ctx context.Context, engine Engine, containerID string) error {
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/start", nil, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent, http.StatusNotModified)
}

func (c *realClient) ContainerStop(ctx context.Context, engine Engine, containerID string, timeoutSec int) error {
	q := url.Values{}
	if timeoutSec > 0 {
		q.Set("t", strconv.Itoa(timeoutSec))
	}
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/stop", q, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent, http.StatusNotModified)
}

func (c *realClient) ContainerRestart(ctx context.Context, engine Engine, containerID string, timeoutSec int) error {
	q := url.Values{}
	if timeoutSec > 0 {
		q.Set("t", strconv.Itoa(timeoutSec))
	}
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/restart", q, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (c *realClient) ContainerPause(ctx context.Context, engine Engine, containerID string) error {
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/pause", nil, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (c *realClient) ContainerUnpause(ctx context.Context, engine Engine, containerID string) error {
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/unpause", nil, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (c *realClient) ContainerRemove(ctx context.Context, engine Engine, containerID string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	resp, err := c.do(ctx, engine, http.MethodDelete, "/containers/"+url.PathEscape(containerID), q, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (c *realClient) ContainerCreate(ctx context.Context, engine Engine, name string, req ContainerCreateRequest) (ContainerCreateResponse, error) {
	q := url.Values{}
	if name != "" {
		q.Set("name", name)
	}
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/create", q, req, nil)
	if err != nil {
		return ContainerCreateResponse{}, err
	}
	return parseJSON[ContainerCreateResponse](resp)
}

func (c *realClient) ContainerLogs(ctx context.Context, engine Engine, containerID string, tail int, stdout, stderr bool) (io.ReadCloser, error) {
	q := url.Values{}
	if stdout {
		q.Set("stdout", "1")
	}
	if stderr {
		q.Set("stderr", "1")
	}
	if tail > 0 {
		q.Set("tail", strconv.Itoa(tail))
	} else {
		q.Set("tail", "all")
	}
	resp, err := c.do(ctx, engine, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/logs", q, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("logs failed (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (c *realClient) ContainerStats(ctx context.Context, engine Engine, containerID string, stream bool) (io.ReadCloser, error) {
	q := url.Values{}
	if !stream {
		q.Set("stream", "0")
	}
	resp, err := c.do(ctx, engine, http.MethodGet, "/containers/"+url.PathEscape(containerID)+"/stats", q, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stats failed (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (c *realClient) ContainerExecCreate(ctx context.Context, engine Engine, containerID string, req ExecCreateRequest) (ExecCreateResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/containers/"+url.PathEscape(containerID)+"/exec", nil, req, nil)
	if err != nil {
		return ExecCreateResponse{}, err
	}
	return parseJSON[ExecCreateResponse](resp)
}

func (c *realClient) ContainerExecStart(ctx context.Context, engine Engine, execID string, req ExecStartRequest) (io.ReadCloser, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/exec/"+url.PathEscape(execID)+"/start", nil, req, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("exec start failed (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (c *realClient) ContainerExecInspect(ctx context.Context, engine Engine, execID string) (ExecInspectResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/exec/"+url.PathEscape(execID)+"/json", nil, nil, nil)
	if err != nil {
		return ExecInspectResponse{}, err
	}
	return parseJSON[ExecInspectResponse](resp)
}

// Image methods

func (c *realClient) ImageList(ctx context.Context, engine Engine) ([]ImageSummary, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/images/json", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return parseJSON[[]ImageSummary](resp)
}

func (c *realClient) ImageInspect(ctx context.Context, engine Engine, imageID string) (ImageInspect, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/images/"+url.PathEscape(imageID)+"/json", nil, nil, nil)
	if err != nil {
		return ImageInspect{}, err
	}
	return parseJSON[ImageInspect](resp)
}

func (c *realClient) ImagePull(ctx context.Context, engine Engine, imageRef string, authEncoded string) (io.ReadCloser, error) {
	q := url.Values{}
	fromImage := imageRef
	tag := ""

	if strings.Contains(imageRef, "@") {
		fromImage = imageRef
	} else if i := strings.LastIndex(imageRef, ":"); i != -1 && !strings.Contains(imageRef[i+1:], "/") {
		fromImage = imageRef[:i]
		tag = imageRef[i+1:]
	} else {
		tag = "latest"
	}

	q.Set("fromImage", fromImage)
	if tag != "" {
		q.Set("tag", tag)
	}

	headers := map[string]string{}
	if authEncoded != "" {
		headers["X-Registry-Auth"] = authEncoded
	}
	resp, err := c.do(ctx, engine, http.MethodPost, "/images/create", q, nil, headers)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("image pull failed (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func (c *realClient) ImageRemove(ctx context.Context, engine Engine, imageID string, force bool) ([]ImageDeleteResponseItem, error) {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	resp, err := c.do(ctx, engine, http.MethodDelete, "/images/"+url.PathEscape(imageID), q, nil, nil)
	if err != nil {
		return nil, err
	}
	return parseJSON[[]ImageDeleteResponseItem](resp)
}

func (c *realClient) ImagesPrune(ctx context.Context, engine Engine) (ImagesPruneReport, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/images/prune", nil, nil, nil)
	if err != nil {
		return ImagesPruneReport{}, err
	}
	return parseJSON[ImagesPruneReport](resp)
}

// Volume methods

func (c *realClient) VolumeList(ctx context.Context, engine Engine) (VolumeListResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/volumes", nil, nil, nil)
	if err != nil {
		return VolumeListResponse{}, err
	}
	return parseJSON[VolumeListResponse](resp)
}

func (c *realClient) VolumeInspect(ctx context.Context, engine Engine, volumeID string) (Volume, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/volumes/"+url.PathEscape(volumeID), nil, nil, nil)
	if err != nil {
		return Volume{}, err
	}
	return parseJSON[Volume](resp)
}

func (c *realClient) VolumeCreate(ctx context.Context, engine Engine, req VolumeCreateRequest) (Volume, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/volumes/create", nil, req, nil)
	if err != nil {
		return Volume{}, err
	}
	return parseJSON[Volume](resp)
}

func (c *realClient) VolumeRemove(ctx context.Context, engine Engine, volumeID string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	resp, err := c.do(ctx, engine, http.MethodDelete, "/volumes/"+url.PathEscape(volumeID), q, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (c *realClient) VolumesPrune(ctx context.Context, engine Engine) (VolumesPruneReport, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/volumes/prune", nil, nil, nil)
	if err != nil {
		return VolumesPruneReport{}, err
	}
	return parseJSON[VolumesPruneReport](resp)
}

// Network methods

func (c *realClient) NetworkList(ctx context.Context, engine Engine) ([]NetworkSummary, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/networks", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return parseJSON[[]NetworkSummary](resp)
}

func (c *realClient) NetworkInspect(ctx context.Context, engine Engine, networkID string) (NetworkSummary, error) {
	resp, err := c.do(ctx, engine, http.MethodGet, "/networks/"+url.PathEscape(networkID), nil, nil, nil)
	if err != nil {
		return NetworkSummary{}, err
	}
	return parseJSON[NetworkSummary](resp)
}

func (c *realClient) NetworkCreate(ctx context.Context, engine Engine, req NetworkCreateRequest) (NetworkCreateResponse, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/networks/create", nil, req, nil)
	if err != nil {
		return NetworkCreateResponse{}, err
	}
	return parseJSON[NetworkCreateResponse](resp)
}

func (c *realClient) NetworkRemove(ctx context.Context, engine Engine, networkID string) error {
	resp, err := c.do(ctx, engine, http.MethodDelete, "/networks/"+url.PathEscape(networkID), nil, nil, nil)
	if err != nil {
		return err
	}
	return checkStatus(resp, http.StatusOK, http.StatusNoContent, http.StatusAccepted)
}

func (c *realClient) NetworksPrune(ctx context.Context, engine Engine) (NetworksPruneReport, error) {
	resp, err := c.do(ctx, engine, http.MethodPost, "/networks/prune", nil, nil, nil)
	if err != nil {
		return NetworksPruneReport{}, err
	}
	return parseJSON[NetworksPruneReport](resp)
}

func (c *realClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dockerHTTP != nil {
		c.dockerHTTP.CloseIdleConnections()
		c.dockerHTTP = nil
	}
	if c.podmanHTTP != nil {
		c.podmanHTTP.CloseIdleConnections()
		c.podmanHTTP = nil
	}
	return nil
}
