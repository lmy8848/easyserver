package executor

import (
	"context"
	"fmt"
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

// Helper to create a success response
func MockSuccess(stdout string) MockResponse {
	return MockResponse{Stdout: stdout, ExitCode: 0}
}

// Helper to create an error response
func MockError(stderr string, exitCode int) MockResponse {
	return MockResponse{Stderr: stderr, ExitCode: exitCode, Error: fmt.Errorf("exit code %d", exitCode)}
}
