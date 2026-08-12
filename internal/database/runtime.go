package database

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/infra/executor"
)

// ContainerSpec describes a database container without exposing CLI details to callers.
type ContainerSpec struct {
	ContainerEngine string
	Name            string
	Image           string
	Volume          string
	DataDir         string
	ConfigVolume    string
	ConfigDir       string
	BindAddress     string
	HostPort        int
	ContainerPort   int
	Environment     map[string]string
	Labels          map[string]string
	HealthCommand   string
	Command         []string
}

// ContainerStatus is the externally observable state of a managed database container.
type ContainerStatus struct {
	State  string
	Health string
}

// DatabaseRuntime is the seam between database lifecycle logic and Docker/Podman.
type DatabaseRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) error
	Start(ctx context.Context, runtime, name string) error
	Stop(ctx context.Context, runtime, name string) error
	Restart(ctx context.Context, runtime, name string) error
	Remove(ctx context.Context, runtime, name string) error
	Status(ctx context.Context, runtime, name string) (ContainerStatus, error)
	Logs(ctx context.Context, runtime, name string, lines int) (string, error)
	Exec(ctx context.Context, runtime, name string, args ...string) (string, error)
	CopyFrom(ctx context.Context, runtime, name, source, destination string) error
	CopyTo(ctx context.Context, runtime, name, source, destination string) error
	Exists(ctx context.Context, runtime, name string) (bool, error)
}

// CLIContainerRuntime implements DatabaseRuntime with Docker or rootful Podman.
type CLIContainerRuntime struct {
	executor executor.CommandExecutor
	// lastSpec records the most recent Create spec, exposed for tests to assert
	// the structured contract (label, volume, port binding) without inspecting
	// command concatenation.
	lastSpec ContainerSpec
	// outputHook, when set, receives every non-empty trimmed command output line
	// (e.g. image pull progress). The installer wires it to the install log.
	outputHook func(string)
}

func NewCLIContainerRuntime(exec executor.CommandExecutor) *CLIContainerRuntime {
	return &CLIContainerRuntime{executor: exec}
}

// SetOutputHook installs a per-line callback for all command output. Used by
// the installer to stream pull/create/start output into the install log.
func (r *CLIContainerRuntime) SetOutputHook(fn func(string)) {
	r.outputHook = fn
}

func containerBinary(runtime string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "docker", "":
		return "docker", nil
	case "podman":
		return "podman", nil
	default:
		return "", fmt.Errorf("unsupported container runtime %q", runtime)
	}
}

// command runs a lifecycle command with stdout+stderr combined — install/log
// paths want the whole stream (pull progress, container logs, errors).
func (r *CLIContainerRuntime) command(ctx context.Context, runtime string, args ...string) (string, error) {
	return r.run(ctx, true, true, runtime, args...)
}

// execCommand runs a command inside the container with stdout and stderr
// separated — query output (mysql/psql listings, SQL results) is parsed, so
// stderr warnings (e.g. mysql's "password on the command line" notice) must
// never be merged into it. stderr only surfaces in the error when the command
// itself fails.
//
// Leading `-e KEY=VAL` pairs (from withAdminCredentials) are hoisted before
// the container name: they are `docker exec` options, not part of the in-container
// command, and exec fails if they land after the name.
func (r *CLIContainerRuntime) execCommand(ctx context.Context, runtime, name string, args ...string) (string, error) {
	var env []string
	for len(args) >= 2 && args[0] == "-e" {
		env = append(env, args[0], args[1])
		args = args[2:]
	}
	execArgs := append([]string{"exec"}, env...)
	execArgs = append(execArgs, name)
	execArgs = append(execArgs, args...)
	return r.run(ctx, false, true, runtime, execArgs...)
}

// run executes one container CLI command. hook=false skips the output hook:
// used by Status, whose `podman inspect` poll output would otherwise spam the
// install log with a `running|starting` line every 500ms during waitForHealthy.
func (r *CLIContainerRuntime) run(ctx context.Context, combine, hook bool, runtime string, args ...string) (string, error) {
	bin, err := containerBinary(runtime)
	if err != nil {
		return "", err
	}
	// 安装场景（挂载了 outputHook）的合并命令改走流式：拉镜像/启动是长耗时
	// 操作，输出要逐行实时进安装日志，而不是等命令整体结束后一次性回放。
	if combine && hook && r.outputHook != nil {
		return r.streamRun(ctx, bin, args...)
	}
	var out, stderr string
	var code int
	var runErr error
	if combine {
		out, code, runErr = r.executor.RunCombined(ctx, bin, args...)
	} else {
		out, stderr, code, runErr = r.executor.Run(ctx, bin, args...)
	}
	if hook && r.outputHook != nil {
		for line := range strings.SplitSeq(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				r.outputHook(line)
			}
		}
	}
	if runErr != nil || code != 0 {
		if runErr != nil {
			return out, fmt.Errorf("%s: %w", bin, runErr)
		}
		// For separated runs, append stderr to the detail so the real error
		// isn't hidden by an empty stdout.
		detail := strings.TrimSpace(out)
		if !combine {
			if s := strings.TrimSpace(stderr); s != "" {
				detail = strings.TrimSpace(detail + "\n" + s)
			}
		}
		return out, fmt.Errorf("%s exited with code %d: %s", bin, code, detail)
	}
	return out, nil
}

// streamRun executes a lifecycle command with stdout+stderr streamed line by
// line into the output hook as they arrive — image pulls and starts are
// long-running, so their progress should stream, not replay when the command
// finishes. Returns the full merged output with the same error shape as run.
func (r *CLIContainerRuntime) streamRun(ctx context.Context, bin string, args ...string) (string, error) {
	out, _, err := r.executor.RunStream(ctx, func(line string) {
		if t := strings.TrimSpace(line); t != "" && r.outputHook != nil {
			r.outputHook(t)
		}
	}, bin, args...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return out, fmt.Errorf("%s exited with code %d: %s", bin, exitErr.ExitCode(), strings.TrimSpace(out))
		}
		return out, fmt.Errorf("%s: %w", bin, err)
	}
	return out, nil
}

func (r *CLIContainerRuntime) Create(ctx context.Context, spec ContainerSpec) error {
	if spec.Name == "" || spec.Image == "" || spec.Volume == "" || spec.DataDir == "" {
		return errors.New("container name, image, volume and data directory are required")
	}
	if spec.HostPort < 1 || spec.HostPort > 65535 || spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return errors.New("invalid container port")
	}
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	spec.Labels["com.easyserver.managed"] = "true"
	r.lastSpec = spec

	args := []string{"create", "--name", spec.Name}
	args = append(args, "--label", "com.easyserver.managed=true", "--label", "com.easyserver.kind=database")
	for _, key := range sortedKeys(spec.Labels) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}
	args = append(args, "--publish", fmt.Sprintf("%s:%d:%d", spec.BindAddress, spec.HostPort, spec.ContainerPort))
	// Volume/ConfigVolume are host absolute paths the panel owns (created + chowned
	// in prepareHostDirs before create). No engine-side `volume create` needed, and
	// no named-volume branches — everything mounts a real host directory.
	args = append(args, "--volume", spec.Volume+":"+spec.DataDir)
	if spec.ConfigVolume != "" && spec.ConfigDir != "" {
		args = append(args, "--volume", spec.ConfigVolume+":"+spec.ConfigDir)
	}
	for _, key := range sortedKeys(spec.Environment) {
		args = append(args, "--env", key+"="+spec.Environment[key])
	}
	if spec.HealthCommand != "" {
		args = append(args, "--health-cmd", spec.HealthCommand, "--health-interval", "5s", "--health-timeout", "3s", "--health-retries", "20")
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	if _, err := r.command(ctx, spec.ContainerEngine, args...); err != nil {
		return fmt.Errorf("create database container: %w", err)
	}
	return nil
}

func (r *CLIContainerRuntime) Start(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "start", name)
	return err
}

func (r *CLIContainerRuntime) Stop(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "stop", name)
	return err
}

func (r *CLIContainerRuntime) Restart(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "restart", name)
	return err
}

func (r *CLIContainerRuntime) Remove(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "rm", "--force", name)
	// The container may already be gone (failed install rolls it back, then a
	// reinstall/uninstall removes it again) — treat that as success.
	if err != nil && notFound(err, "no such container") {
		return nil
	}
	return err
}

// notFound reports whether err is a docker/podman "does not exist" error for the
// given resource marker ("no such container", "no such volume", …). Idempotent
// deletes (rm / volume rm) surface these as failures even though the desired
// end state (resource gone) is already true.
func notFound(err error, markers ...string) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, m := range markers {
		if strings.Contains(s, m) || strings.Contains(s, "not found") {
			return true
		}
	}
	return false
}

func (r *CLIContainerRuntime) Status(ctx context.Context, runtime, name string) (ContainerStatus, error) {
	out, err := r.run(ctx, true, false, runtime, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name)
	if err != nil {
		return ContainerStatus{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	status := ContainerStatus{State: parts[0]}
	if len(parts) == 2 {
		status.Health = parts[1]
	}
	return status, nil
}

func (r *CLIContainerRuntime) Logs(ctx context.Context, runtime, name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	return r.command(ctx, runtime, "logs", "--tail", strconv.Itoa(lines), name)
}

func (r *CLIContainerRuntime) Exec(ctx context.Context, runtime, name string, args ...string) (string, error) {
	if name == "" || len(args) == 0 {
		return "", errors.New("container name and command are required")
	}
	return r.execCommand(ctx, runtime, name, args...)
}

func (r *CLIContainerRuntime) CopyFrom(ctx context.Context, runtime, name, source, destination string) error {
	_, err := r.command(ctx, runtime, "cp", name+":"+source, destination)
	return err
}

func (r *CLIContainerRuntime) CopyTo(ctx context.Context, runtime, name, source, destination string) error {
	_, err := r.command(ctx, runtime, "cp", source, name+":"+destination)
	return err
}

// Exists reports whether a container with the given name already exists in the
// engine (running or stopped). "Not found" is treated as false, not an error.
// Used to pre-check a user-supplied container name before install so a
// duplicate name surfaces as a clear error instead of a cryptic `create`
// conflict.
func (r *CLIContainerRuntime) Exists(ctx context.Context, runtime, name string) (bool, error) {
	if name == "" {
		return false, errors.New("container name is required")
	}
	_, err := r.run(ctx, true, false, runtime, "inspect", name)
	if err != nil {
		// notFound 匹配小写后的错误文本，marker 用小写（docker "No such object"
		// / podman "no such container"）。
		if notFound(err, "no such object", "no such container") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func waitForHealthy(ctx context.Context, runtime DatabaseRuntime, runtimeName, container string, timeout time.Duration) (ContainerStatus, error) {
	// timeout > 0 enforces a deadline; timeout <= 0 waits indefinitely (the
	// install path relies on container exit or user cancel to terminate).
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		status, err := runtime.Status(ctx, runtimeName, container)
		if err != nil {
			return status, err
		}
		if status.State == "running" && status.Health == "healthy" {
			return status, nil
		}
		if status.State == "exited" || status.State == "dead" {
			return status, errors.New("database container stopped before becoming healthy")
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return status, fmt.Errorf("database container did not become healthy within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
