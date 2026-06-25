package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// MockResponse defines the response for a mock command
type MockResponse struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

// MockExecutor is a mock implementation for testing
type MockExecutor struct {
	Responses map[string]MockResponse
	Default   MockResponse
}

// NewMockExecutor creates a new MockExecutor
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		Responses: make(map[string]MockResponse),
		Default: MockResponse{
			Stdout:   "",
			Stderr:   "",
			ExitCode: 0,
			Error:    nil,
		},
	}
}

// SetResponse sets the response for a specific command
func (m *MockExecutor) SetResponse(name string, response MockResponse) {
	m.Responses[name] = response
}

// Run executes a mock command
func (m *MockExecutor) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}

	if resp, ok := m.Responses[key]; ok {
		return resp.Stdout, resp.Stderr, resp.ExitCode, resp.Error
	}

	return m.Default.Stdout, m.Default.Stderr, m.Default.ExitCode, m.Default.Error
}

// RunWithTimeout executes a mock command with timeout
func (m *MockExecutor) RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, error) {
	return m.Run(ctx, name, args...)
}

// RunCombined executes a mock command and returns combined output
func (m *MockExecutor) RunCombined(ctx context.Context, name string, args ...string) (string, int, error) {
	stdout, stderr, exitCode, err := m.Run(ctx, name, args...)
	if err != nil {
		return "", exitCode, err
	}
	if stderr != "" {
		return stdout + "\n" + stderr, exitCode, nil
	}
	return stdout, exitCode, nil
}

// RunWithOptions executes a mock command with custom options
func (m *MockExecutor) RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (string, int, error) {
	return m.RunCombined(ctx, name, args...)
}

// MockProcess is a mock implementation of Process
type MockProcess struct {
	pid    int
	waitCh chan error
}

// NewMockProcess creates a new MockProcess
func NewMockProcess(pid int) *MockProcess {
	return &MockProcess{
		pid:    pid,
		waitCh: make(chan error, 1),
	}
}

// Pid returns the process ID
func (p *MockProcess) Pid() int {
	return p.pid
}

// StdoutPipe returns a reader for stdout
func (p *MockProcess) StdoutPipe() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// StderrPipe returns a reader for stderr
func (p *MockProcess) StderrPipe() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// Wait waits for the process to exit
func (p *MockProcess) Wait() error {
	return <-p.waitCh
}

// Kill kills the process
func (p *MockProcess) Kill() error {
	p.waitCh <- nil
	return nil
}

// Signal sends a signal to the process
func (p *MockProcess) Signal(sig syscall.Signal) error {
	return nil
}

// Cmd returns the underlying exec.Cmd (for PTY usage)
func (p *MockProcess) Cmd() *exec.Cmd {
	return nil
}

// Start starts a mock process
func (m *MockExecutor) Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error) {
	return NewMockProcess(12345), nil
}

// Helper to create a success response
func MockSuccess(stdout string) MockResponse {
	return MockResponse{Stdout: stdout, ExitCode: 0}
}

// Helper to create an error response
func MockError(stderr string, exitCode int) MockResponse {
	return MockResponse{Stderr: stderr, ExitCode: exitCode, Error: fmt.Errorf("exit code %d", exitCode)}
}
