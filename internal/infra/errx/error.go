package errx

import (
	"errors"
	"fmt"
	"strings"
)

// ============================================================
// Kind: 语义错误分类枚举（纯语义，不依赖 HTTP）
// ============================================================

type Kind int

const (
	KindBadRequest Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindRateLimit
	KindInternal
	KindUnavailable
	KindTimeout
	KindNotImplemented
)

// Error 实现 error 接口，使得 errors.Is(err, errx.KindNotFound) 成立
func (k Kind) Error() string {
	switch k {
	case KindBadRequest:
		return "bad_request"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindRateLimit:
		return "rate_limit"
	case KindInternal:
		return "internal"
	case KindUnavailable:
		return "unavailable"
	case KindTimeout:
		return "timeout"
	case KindNotImplemented:
		return "not_implemented"
	default:
		return "unknown"
	}
}

// defaultMessage 返回各 Kind 对应的默认用户文案
func defaultMessage(k Kind) string {
	switch k {
	case KindBadRequest:
		return "请求参数错误"
	case KindUnauthorized:
		return "未授权"
	case KindForbidden:
		return "禁止访问"
	case KindNotFound:
		return "资源不存在"
	case KindConflict:
		return "资源冲突"
	case KindRateLimit:
		return "请求过于频繁"
	case KindUnavailable:
		return "服务不可用或未就绪"
	case KindTimeout:
		return "操作超时"
	case KindNotImplemented:
		return "功能暂未支持或未实现"
	case KindInternal:
		fallthrough
	default:
		return "内部服务器错误"
	}
}

// ============================================================
// Error: 统一语义错误类型
// ============================================================

type Error struct {
	Kind    Kind   // 语义分类
	Message string // 用户可见文案
	Code    int    // 业务码；0 = 用 mapKind 默认码，非 0 = 领域自定义细分业务码
	Cause   error  // 底层错误（日志用），可为 nil
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// Is 实现两档判断：
// 1. 指针身份（哨兵精确匹配，如各领域自定义哨兵）
// 2. Kind 分类兜底（如 errors.Is(err, errx.KindNotFound)）
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e == t
	}
	if k, ok := target.(Kind); ok {
		return e.Kind == k
	}
	return false
}

// SafeError 返回脱敏后的错误字符串供日志使用，过滤 token/password 等敏感信息
func (e *Error) SafeError() string {
	msg := e.Message
	if e.Cause != nil {
		causeMsg := sanitizeSensitiveInfo(e.Cause.Error())
		return fmt.Sprintf("%s: %s", msg, causeMsg)
	}
	return msg
}

// ============================================================
// 工厂与便捷构造函数
// ============================================================

// NewSentinel 创建带有自定义业务码的领域哨兵错误（各领域定义专属错误使用）
func NewSentinel(kind Kind, code int, message string) *Error {
	return &Error{
		Kind:    kind,
		Code:    code,
		Message: message,
	}
}

// New 创建指定 Kind 的通用错误
func New(kind Kind, msg string, cause ...error) error {
	var c error
	if len(cause) > 0 {
		c = cause[0]
	}
	if msg == "" {
		msg = defaultMessage(kind)
	}
	return &Error{
		Kind:    kind,
		Message: msg,
		Cause:   c,
	}
}

// BadRequest 创建参数错误 (KindBadRequest)
func BadRequest(msg string, args ...any) error {
	return formatError(KindBadRequest, msg, args...)
}

// Unauthorized 创建未授权错误 (KindUnauthorized)
func Unauthorized(msg string, args ...any) error {
	return formatError(KindUnauthorized, msg, args...)
}

// Forbidden 创建禁止访问错误 (KindForbidden)
func Forbidden(msg string, args ...any) error {
	return formatError(KindForbidden, msg, args...)
}

// NotFound 创建资源不存在错误 (KindNotFound)
func NotFound(msg string, args ...any) error {
	return formatError(KindNotFound, msg, args...)
}

// Conflict 创建资源冲突错误 (KindConflict)
func Conflict(msg string, args ...any) error {
	return formatError(KindConflict, msg, args...)
}

// RateLimit 创建限流错误 (KindRateLimit)
func RateLimit(msg string, args ...any) error {
	return formatError(KindRateLimit, msg, args...)
}

// Internal 创建内部错误 (KindInternal)
func Internal(msg string, args ...any) error {
	return formatError(KindInternal, msg, args...)
}

// Unavailable 创建服务不可用/未就绪错误 (KindUnavailable)
func Unavailable(msg string, args ...any) error {
	return formatError(KindUnavailable, msg, args...)
}

// Timeout 创建操作超时错误 (KindTimeout)
func Timeout(msg string, args ...any) error {
	return formatError(KindTimeout, msg, args...)
}

// NotImplemented 创建功能未实现错误 (KindNotImplemented)
func NotImplemented(msg string, args ...any) error {
	return formatError(KindNotImplemented, msg, args...)
}

// formatError 解析 msg 字符串和参数，使用 fmt.Errorf 支持 %w 与标准格式化
func formatError(kind Kind, msg string, args ...any) error {
	var cause error
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			cause = err
		}
	}

	if len(args) == 0 {
		return &Error{
			Kind:    kind,
			Message: msg,
			Cause:   cause,
		}
	}

	formattedErr := fmt.Errorf(msg, args...)
	if cause == nil {
		cause = errors.Unwrap(formattedErr)
	}

	return &Error{
		Kind:    kind,
		Message: formattedErr.Error(),
		Cause:   cause,
	}
}

// ============================================================
// 日志敏感信息脱敏
// ============================================================

var sensitiveKeys = []string{
	"token", "password", "secret", "authorization",
	"jwt", "api_key", "apikey", "private_key", "passwd",
}

// sanitizeSensitiveInfo 脱敏日志字符串中的敏感信息
func sanitizeSensitiveInfo(s string) string {
	lower := strings.ToLower(s)
	for _, key := range sensitiveKeys {
		if idx := strings.Index(lower, key); idx != -1 {
			sepIdx := strings.IndexAny(s[idx:], "=:")
			if sepIdx != -1 {
				valStart := idx + sepIdx + 1
				for valStart < len(s) && (s[valStart] == ' ' || s[valStart] == '"' || s[valStart] == '\'') {
					valStart++
				}
				valEnd := valStart
				for valEnd < len(s) && s[valEnd] != ' ' && s[valEnd] != '&' && s[valEnd] != ',' && s[valEnd] != '"' && s[valEnd] != '\'' && s[valEnd] != '\n' {
					valEnd++
				}
				if valEnd > valStart {
					s = s[:valStart] + "******" + s[valEnd:]
					lower = strings.ToLower(s)
				}
			}
		}
	}
	return s
}
