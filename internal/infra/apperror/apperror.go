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
//
// 分类语义：Code 按 HTTP 分类（同分类细分哨兵共享 Code），Is 按 Code
// 比较——因此 WithMessage/Wrap 产生的副本与哨兵之间 errors.Is 成立，
// 分类粒度即 HTTP 分类。
type AppError struct {
	HTTPStatus int    // HTTP status code
	Code       int    // Business error code（HTTP 分类标识，兼作哨兵身份）
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

// Is 让 errors.Is 对 WithMessage/Wrap 副本成立：副本与哨兵 Code 相同即
// 视为同一分类错误（如 errors.Is(err, ErrNotFound) 匹配所有 404 副本）。
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Wrap creates a new AppError wrapping an underlying error
func (e *AppError) Wrap(err error) *AppError {
	ne := *e
	ne.Err = err
	return &ne
}

// WithMessage creates a copy with a custom message
func (e *AppError) WithMessage(msg string) *AppError {
	ne := *e
	ne.Message = msg
	return &ne
}

// WrapMessage 一步完成"分类 + 原始消息 + 错误链"：产生端显式指定错误分类
// 的推荐方式——
//
//	return apperror.ErrConflict.WrapMessage(err)
//
// 等价于 WithMessage(err.Error()).Wrap(err)：分类（Code/HTTPStatus）保留
// 哨兵语义，用户可见消息取底层错误文本，原始错误经 Unwrap 留在链上
// （errors.Is/As 仍可穿透）。
func (e *AppError) WrapMessage(err error) *AppError {
	ne := *e
	ne.Message = err.Error()
	ne.Err = err
	return &ne
}

// ============================================================
// 错误码常量（按 HTTP 分类，兼作 errors.Is 的分类标识；同一分类的
// 细分哨兵共享 Code）
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

// 派生业务码：500 段细分哨兵（仍属 InternalError 分类区段，Code 唯一以支持
// 精确 errors.Is 判断，如"是否 Docker 未安装"、"服务是否就绪"）。
const (
	CodeDockerNotInstalled = 50001
	CodeServiceNotReady    = 50002
)

// ============================================================
// 预定义错误（哨兵）
// ============================================================

var (
	// 400 Bad Request
	ErrBadRequest = &AppError{HTTPStatus: http.StatusBadRequest, Code: CodeBadRequest, Message: "请求参数错误"}

	// 401 Unauthorized
	ErrUnauthorized = &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "未授权"}
	// token 无效：覆盖签名错误/格式错误/过期等一切校验失败，独立 Code
	// （前端依据 40101 识别登录失效）
	ErrTokenExpired = &AppError{HTTPStatus: http.StatusUnauthorized, Code: CodeTokenExpired, Message: "token 无效"}

	// 403 Forbidden
	ErrForbidden = &AppError{HTTPStatus: http.StatusForbidden, Code: CodeForbidden, Message: "禁止访问"}

	// 404 Not Found
	ErrNotFound = &AppError{HTTPStatus: http.StatusNotFound, Code: CodeNotFound, Message: "资源不存在"}

	// 409 Conflict
	ErrConflict = &AppError{HTTPStatus: http.StatusConflict, Code: CodeConflict, Message: "资源冲突"}

	// 429 Rate Limit
	ErrRateLimit = &AppError{HTTPStatus: http.StatusTooManyRequests, Code: CodeRateLimit, Message: "请求过于频繁"}

	// 500 Internal Server Error
	ErrInternal = &AppError{HTTPStatus: http.StatusInternalServerError, Code: CodeInternalError, Message: "内部服务器错误"}

	// Domain-specific errors（服务端环境/状态问题 → 500 分类）
	ErrDockerNotInstalled = &AppError{HTTPStatus: http.StatusInternalServerError, Code: CodeDockerNotInstalled, Message: "Docker 未安装或未启动"}
	ErrServiceNotReady    = &AppError{HTTPStatus: http.StatusInternalServerError, Code: CodeServiceNotReady, Message: "服务未就绪"}
)

// ============================================================
// 错误分类函数
// ============================================================

// errorPattern maps error message patterns to AppError types
type errorPattern struct {
	matches []string  // substrings to match
	target  *AppError // target error type
}

// errorRegistry is the ordered list of error patterns.
// First match wins. Add new patterns here instead of modifying WrapError.
//
// 迁移状态：产生端已显式分类的条目已移除（path traversal → filemanager、
// 无效的表名/列名 → database/sql_builder、npm/pip → runtimeenv、DB 配置驱动
// → database/config、invalid password/TOTP → auth、docker 系列 → container）。
// 剩余条目是领域级/驱动级泛化错误（SQLite 约束、实体不存在/已存在等），待各
// 领域迁移到产生端后逐条移除。
var errorRegistry = []errorPattern{
	// Not found
	{matches: []string{"not found", "未安装", "不存在", "does not exist", "No such container"}, target: ErrNotFound},
	// Already exists / installed / running
	{matches: []string{"already installed", "已安装", "已存在", "is already running", "is not running", "未运行"}, target: ErrConflict},
	// Bad state / precondition
	{matches: []string{"cannot change", "cannot be empty", "stop it first"}, target: ErrBadRequest},
	// UNIQUE constraint violation (SQLite)
	{matches: []string{"UNIQUE constraint failed", "constraint failed"}, target: ErrConflict},
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
