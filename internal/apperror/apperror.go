package apperror

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================
// AppError: 统一应用错误类型
// ============================================================

// AppError is the unified application error type used across all packages.
// Handlers return AppError (or wrap it), and middleware converts it to HTTP responses.
type AppError struct {
	HTTPStatus int    // HTTP status code
	Code       int    // Business error code
	Message    string // User-facing message
	Err        error  // Original error for logging
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// SafeError returns a sanitized error string for logging,
// filtering out potential sensitive information like tokens or passwords.
func (e *AppError) SafeError() string {
	msg := e.Message
	if e.Err != nil {
		errMsg := e.Err.Error()
		// Filter out sensitive patterns
		errMsg = sanitizeSensitiveInfo(errMsg)
		return fmt.Sprintf("%s: %s", msg, errMsg)
	}
	return msg
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// Wrap creates a new AppError wrapping an underlying error
func (e *AppError) Wrap(err error) *AppError {
	return &AppError{
		HTTPStatus: e.HTTPStatus,
		Code:       e.Code,
		Message:    e.Message,
		Err:        err,
	}
}

// WithMessage creates a copy with a custom message
func (e *AppError) WithMessage(msg string) *AppError {
	return &AppError{
		HTTPStatus: e.HTTPStatus,
		Code:       e.Code,
		Message:    msg,
		Err:        e.Err,
	}
}

// ============================================================
// 错误码常量
// ============================================================

const (
	CodeSuccess       = 0
	CodeBadRequest    = 40000
	CodeUnauthorized  = 40100
	CodeTokenExpired  = 40101
	CodeForbidden     = 40300
	CodeNotFound      = 40400
	CodeConflict      = 40900
	CodeRateLimit     = 42900
	CodeInternalError = 50000
)

// ============================================================
// 预定义错误
// ============================================================

var (
	// 400 Bad Request
	ErrBadRequest = &AppError{HTTPStatus: http.StatusBadRequest, Code: CodeBadRequest, Message: "请求参数错误"}

	// 401 Unauthorized
	ErrUnauthorized = &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "未授权"}
	ErrTokenExpired = &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeTokenExpired, Message: "token 已过期"}

	// 403 Forbidden
	ErrForbidden    = &AppError{HTTPStatus: http.StatusForbidden, Code: CodeForbidden, Message: "禁止访问"}
	ErrPathViolation = &AppError{HTTPStatus: http.StatusForbidden, Code: CodeForbidden, Message: "路径越权"}

	// 404 Not Found
	ErrNotFound = &AppError{HTTPStatus: http.StatusNotFound, Code: CodeNotFound, Message: "资源不存在"}

	// 409 Conflict
	ErrConflict = &AppError{HTTPStatus: http.StatusConflict, Code: CodeConflict, Message: "资源冲突"}

	// 429 Rate Limit
	ErrRateLimit = &AppError{HTTPStatus: http.StatusTooManyRequests, Code: CodeRateLimit, Message: "请求过于频繁"}

	// 500 Internal Server Error
	ErrInternal = &AppError{HTTPStatus: http.StatusInternalServerError, Code: CodeInternalError, Message: "内部服务器错误"}

	// Domain-specific errors
	ErrDockerNotInstalled = &AppError{HTTPStatus: http.StatusBadRequest, Code: CodeBadRequest, Message: "Docker 未安装或未启动"}
	ErrServiceNotReady    = &AppError{HTTPStatus: http.StatusBadRequest, Code: CodeBadRequest, Message: "服务未就绪"}
)

// ============================================================
// 错误分类函数
// ============================================================

// IsPathError checks if the error is a path validation error
func IsPathError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr == ErrPathViolation
	}
	msg := err.Error()
	return contains(msg, "path traversal") ||
		contains(msg, "absolute paths are not allowed") ||
		contains(msg, "cannot resolve path")
}

// IsDockerNotInstalled checks if the error is about Docker not being available
func IsDockerNotInstalled(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr == ErrDockerNotInstalled
	}
	msg := err.Error()
	return contains(msg, "docker info failed") ||
		contains(msg, "Cannot connect to the Docker daemon") ||
		contains(msg, "docker: command not found") ||
		contains(msg, "executable file not found") ||
		contains(msg, "docker is not installed") ||
		contains(msg, "not accessible")
}

// errorPattern maps error message patterns to AppError types
type errorPattern struct {
	matches []string    // substrings to match
	target  *AppError   // target error type
}

// errorRegistry is the ordered list of error patterns.
// First match wins. Add new patterns here instead of modifying WrapError.
var errorRegistry = []errorPattern{
	// Security: path traversal
	{matches: []string{"path traversal", "absolute paths are not allowed", "cannot resolve path"}, target: ErrPathViolation},
	// Docker not available
	{matches: []string{"docker info failed", "Cannot connect to the Docker daemon", "docker: command not found", "executable file not found", "docker is not installed", "not accessible"}, target: ErrDockerNotInstalled},
	// Auth errors
	{matches: []string{"invalid password", "invalid TOTP code", "invalid credentials"}, target: ErrUnauthorized},
	// Not found
	{matches: []string{"not found", "未安装", "不存在"}, target: ErrNotFound},
	// Already exists / installed / running
	{matches: []string{"already installed", "已安装", "已存在", "is already running"}, target: ErrConflict},
	// Bad state / precondition
	{matches: []string{"is not running", "cannot change", "cannot be empty", "stop it first"}, target: ErrBadRequest},
	// UNIQUE constraint violation (SQLite)
	{matches: []string{"UNIQUE constraint failed", "constraint failed"}, target: ErrConflict},
	// No data available
	{matches: []string{"no versions available", "无可用版本"}, target: ErrBadRequest},
}

// WrapError automatically wraps an error into the appropriate AppError
// based on error pattern matching. Add new patterns to errorRegistry.
func WrapError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return err
	}

	msg := err.Error()
	for _, p := range errorRegistry {
		for _, pattern := range p.matches {
			if contains(msg, pattern) {
				return p.target.WithMessage(msg)
			}
		}
	}

	return ErrInternal.Wrap(err)
}

// ============================================================
// 内部工具函数
// ============================================================

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// sensitivePatterns are patterns that should be filtered from error logs
var sensitivePatterns = []string{
	"token",
	"password",
	"secret",
	"credential",
	"authorization",
	"bearer",
	"jwt",
	"api_key",
	"apikey",
	"access_key",
}

// sanitizeSensitiveInfo replaces sensitive patterns in error messages with [REDACTED]
func sanitizeSensitiveInfo(s string) string {
	lower := strings.ToLower(s)
	for _, pattern := range sensitivePatterns {
		idx := strings.Index(lower, pattern)
		if idx >= 0 {
			// Find the value part after the pattern (e.g., "token: xxx" or "token=xxx")
			start := idx + len(pattern)
			if start < len(s) {
				// Skip separator (colon, equals, space)
				for start < len(s) && (s[start] == ':' || s[start] == '=' || s[start] == ' ') {
					start++
				}
				// Find end of value (next space, comma, or end of string)
				end := start
				for end < len(s) && s[end] != ' ' && s[end] != ',' && s[end] != '\n' {
					end++
				}
				if end > start {
					// Replace the value with [REDACTED]
					s = s[:start] + "[REDACTED]" + s[end:]
					lower = strings.ToLower(s) // Recalculate lower
				}
			}
		}
	}
	return s
}
