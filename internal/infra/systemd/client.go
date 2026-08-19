package systemd

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
)

var (
	// ErrSystemdUnavailable indicates that systemd D-Bus is not reachable on this system.
	ErrSystemdUnavailable = errors.New("systemd D-Bus is unavailable on this host")
)

// SystemdClient abstracts all D-Bus IPC operations with systemd.
type SystemdClient interface {
	// IsAvailable returns whether systemd D-Bus is accessible.
	IsAvailable() bool

	// ListUnitsContext returns all active and loaded systemd units.
	ListUnitsContext(ctx context.Context) ([]dbus.UnitStatus, error)

	// ListUnitsByPatternsContext returns units filtered by states and name patterns.
	ListUnitsByPatternsContext(ctx context.Context, states []string, patterns []string) ([]dbus.UnitStatus, error)

	// GetUnitPropertiesContext returns a map of all properties for the specified unit.
	GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error)

	// GetUnitPropertyContext returns a single typed property for the specified unit.
	GetUnitPropertyContext(ctx context.Context, unit string, propertyName string) (*dbus.Property, error)

	// GetUnitTypePropertiesContext returns properties from a specific D-Bus interface for the unit.
	GetUnitTypePropertiesContext(ctx context.Context, unit string, dbusInterface string) (map[string]any, error)

	// StartUnitContext synchronously starts a unit and waits for the job to complete.
	StartUnitContext(ctx context.Context, name string, mode string) (string, error)

	// StopUnitContext synchronously stops a unit and waits for the job to complete.
	StopUnitContext(ctx context.Context, name string, mode string) (string, error)

	// RestartUnitContext synchronously restarts a unit and waits for the job to complete.
	RestartUnitContext(ctx context.Context, name string, mode string) (string, error)

	// ReloadUnitContext synchronously reloads a unit's configuration and waits for the job to complete.
	ReloadUnitContext(ctx context.Context, name string, mode string) (string, error)

	// ReloadContext reloads systemd daemon configuration (daemon-reload).
	ReloadContext(ctx context.Context) error

	// EnableUnitFilesContext enables unit files with systemd.
	EnableUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) (bool, []dbus.EnableUnitFileChange, error)

	// DisableUnitFilesContext disables unit files with systemd.
	DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error)

	// ResetFailedUnitContext resets the failed state of a unit.
	ResetFailedUnitContext(ctx context.Context, name string) error

	// MaskUnitFilesContext masks unit files.
	MaskUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) ([]dbus.MaskUnitFileChange, error)

	// UnmaskUnitFilesContext unmasks unit files.
	UnmaskUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.UnmaskUnitFileChange, error)

	// Close closes the underlying D-Bus connection.
	Close()
}

type realClient struct {
	mu   sync.Mutex
	conn *dbus.Conn
}

var (
	globalClient SystemdClient
	globalMu     sync.Mutex
)

// DefaultClient returns the global singleton SystemdClient instance.
func DefaultClient() SystemdClient {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalClient == nil {
		globalClient = NewSystemdClient()
	}
	return globalClient
}

// SetDefaultClient sets the global singleton SystemdClient (useful for dependency injection in tests).
func SetDefaultClient(c SystemdClient) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalClient = c
}

// NewSystemdClient creates a new SystemdClient instance with lazy connection initialization.
func NewSystemdClient() SystemdClient {
	return &realClient{}
}

func (c *realClient) getConn(ctx context.Context) (*dbus.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If we have a cached connection, check if it's still valid
	if c.conn != nil {
		if c.conn.Connected() {
			return c.conn, nil
		}
		// Connection is invalid, close it and clear cache
		c.conn.Close()
		c.conn = nil
	}

	// Create a new connection
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSystemdUnavailable, err)
	}
	c.conn = conn
	return c.conn, nil
}

func (c *realClient) IsAvailable() bool {
	conn, err := c.getConn(context.Background())
	return err == nil && conn != nil
}

func (c *realClient) ListUnitsContext(ctx context.Context) ([]dbus.UnitStatus, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.ListUnitsContext(ctx)
}

func (c *realClient) ListUnitsByPatternsContext(ctx context.Context, states []string, patterns []string) ([]dbus.UnitStatus, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.ListUnitsByPatternsContext(ctx, states, patterns)
}

func (c *realClient) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.GetUnitPropertiesContext(ctx, unit)
}

func (c *realClient) GetUnitPropertyContext(ctx context.Context, unit string, propertyName string) (*dbus.Property, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.GetUnitPropertyContext(ctx, unit, propertyName)
}

func (c *realClient) GetUnitTypePropertiesContext(ctx context.Context, unit string, dbusInterface string) (map[string]any, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.GetUnitTypePropertiesContext(ctx, unit, dbusInterface)
}

// waitJob runs a unit action and waits for the job result channel.
func (c *realClient) waitJob(ctx context.Context, action string, name string, fn func(conn *dbus.Conn, ch chan<- string) (int, error)) (string, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return "", err
	}
	ch := make(chan string, 1)
	if _, err := fn(conn, ch); err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res != "done" {
			return res, fmt.Errorf("systemd %s job for %s resulted in '%s'", action, name, res)
		}
		return res, nil
	}
}

func (c *realClient) StartUnitContext(ctx context.Context, name string, mode string) (string, error) {
	if mode == "" {
		mode = "replace"
	}
	return c.waitJob(ctx, "start", name, func(conn *dbus.Conn, ch chan<- string) (int, error) {
		return conn.StartUnitContext(ctx, name, mode, ch)
	})
}

func (c *realClient) StopUnitContext(ctx context.Context, name string, mode string) (string, error) {
	if mode == "" {
		mode = "replace"
	}
	return c.waitJob(ctx, "stop", name, func(conn *dbus.Conn, ch chan<- string) (int, error) {
		return conn.StopUnitContext(ctx, name, mode, ch)
	})
}

func (c *realClient) RestartUnitContext(ctx context.Context, name string, mode string) (string, error) {
	if mode == "" {
		mode = "replace"
	}
	return c.waitJob(ctx, "restart", name, func(conn *dbus.Conn, ch chan<- string) (int, error) {
		return conn.RestartUnitContext(ctx, name, mode, ch)
	})
}

func (c *realClient) ReloadUnitContext(ctx context.Context, name string, mode string) (string, error) {
	if mode == "" {
		mode = "replace"
	}
	return c.waitJob(ctx, "reload", name, func(conn *dbus.Conn, ch chan<- string) (int, error) {
		return conn.ReloadUnitContext(ctx, name, mode, ch)
	})
}

func (c *realClient) ReloadContext(ctx context.Context) error {
	conn, err := c.getConn(ctx)
	if err != nil {
		return err
	}
	return conn.ReloadContext(ctx)
}

func (c *realClient) EnableUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return false, nil, err
	}
	return conn.EnableUnitFilesContext(ctx, files, runtime, force)
}

func (c *realClient) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.DisableUnitFilesContext(ctx, files, runtime)
}

func (c *realClient) ResetFailedUnitContext(ctx context.Context, name string) error {
	conn, err := c.getConn(ctx)
	if err != nil {
		return err
	}
	return conn.ResetFailedUnitContext(ctx, name)
}

func (c *realClient) MaskUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) ([]dbus.MaskUnitFileChange, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.MaskUnitFilesContext(ctx, files, runtime, force)
}

func (c *realClient) UnmaskUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.UnmaskUnitFileChange, error) {
	conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}
	return conn.UnmaskUnitFilesContext(ctx, files, runtime)
}

func (c *realClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}
