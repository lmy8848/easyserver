package executor

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CommandExecutor defines the interface for executing system commands.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)
	RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (stdout, stderr string, exitCode int, err error)
	RunCombined(ctx context.Context, name string, args ...string) (output string, exitCode int, err error)
	RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (output string, exitCode int, err error)
	// RunStream runs a command with stdout+stderr merged and each line delivered
	// to onLine as it arrives (progress for long-running commands like image
	// pulls). It blocks until the process exits and returns the full merged
	// output, exit code and error.
	RunStream(ctx context.Context, onLine func(string), name string, args ...string) (output string, exitCode int, err error)
	Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error)
	Command(ctx context.Context, opts StartOptions, name string, args ...string) *exec.Cmd
	LookPath(name string) (string, error)
}

type StartOptions struct {
	WorkDir string
	Env     []string
	Setpgid bool
}

type Process interface {
	Pid() int
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Wait() error
	Kill() error
	Signal(sig syscall.Signal) error
	Cmd() *exec.Cmd
}

type CommandOptions struct {
	WorkDir string
	Env     []string
	Timeout time.Duration
	// Stdin, when non-empty, is fed to the command's stdin (e.g. passwords via
	// --password-stdin).
	Stdin string
}

type OSExecutor struct{}

func NewOSExecutor() CommandExecutor { return &OSExecutor{} }

func (e *OSExecutor) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdoutBytes, err := cmd.Output()
	stdout := string(stdoutBytes)
	stderr := ""
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
			exitCode = exitErr.ExitCode()
		} else {
			return stdout, stderr, -1, err
		}
	}
	return stdout, stderr, exitCode, nil
}

func (e *OSExecutor) RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return e.Run(ctx, name, args...)
}

func (e *OSExecutor) RunCombined(ctx context.Context, name string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	result := string(output)
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return result, -1, err
		}
	}
	return result, exitCode, nil
}

// RunWithOptions runs a command with custom options (env, workdir, timeout,
// stdin). Stdin feeds the command's stdin (e.g. --password-stdin) to keep
// secrets out of argv/ps.
func (e *OSExecutor) RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (string, int, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	applyCommandOptions(cmd, opts.WorkDir, opts.Env)
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	output, err := cmd.CombinedOutput()
	result := string(output)
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return result, -1, err
		}
	}
	return result, exitCode, nil
}

// RunStream runs a command with stdout+stderr piped and merged, delivering each
// line to onLine as it arrives (progress for image pulls / long commands). The
// pipes are set up before Start (os/exec requires it) and both are drained —
// reading only one would block the process once the other fills its buffer.
func (e *OSExecutor) RunStream(ctx context.Context, onLine func(string), name string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	so, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	se, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, err
	}
	if err := cmd.Start(); err != nil {
		return "", -1, err
	}

	lines := make(chan string, 256)
	var wg sync.WaitGroup
	read := func(rc io.ReadCloser) {
		defer wg.Done()
		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}
	wg.Add(2)
	go read(so)
	go read(se)
	go func() { wg.Wait(); close(lines) }()

	var sb strings.Builder
	collect := make(chan struct{})
	go func() {
		defer close(collect)
		for line := range lines {
			sb.WriteString(line)
			sb.WriteString("\n")
			if onLine != nil {
				onLine(line)
			}
		}
	}()

	err = cmd.Wait()
	<-collect

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
	}
	return sb.String(), code, err
}

type OSProcess struct{ cmd *exec.Cmd }

func (p *OSProcess) Pid() int                           { return p.cmd.Process.Pid }
func (p *OSProcess) StdoutPipe() (io.ReadCloser, error) { return p.cmd.StdoutPipe() }
func (p *OSProcess) StderrPipe() (io.ReadCloser, error) { return p.cmd.StderrPipe() }
func (p *OSProcess) Wait() error                        { return p.cmd.Wait() }
func (p *OSProcess) Kill() error                        { return p.cmd.Process.Kill() }
func (p *OSProcess) Signal(sig syscall.Signal) error    { return p.cmd.Process.Signal(sig) }
func (p *OSProcess) Cmd() *exec.Cmd                     { return p.cmd }

func (e *OSExecutor) Command(ctx context.Context, opts StartOptions, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	applyCommandOptions(cmd, opts.WorkDir, opts.Env)
	if opts.Setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

func (e *OSExecutor) Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error) {
	cmd := e.Command(ctx, opts, name, args...)

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &OSProcess{cmd: cmd}, nil
}

func (e *OSExecutor) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func applyCommandOptions(cmd *exec.Cmd, workDir string, env []string) {
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
}
