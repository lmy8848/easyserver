package errx

import (
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
	Code    int    // 业务码；0 = 用 mapKind 默认码，非 0 = 覆盖（业务细分）
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
// 1. 指针身份（哨兵精确匹配，如 ErrDockerNotInstalled）
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
// 预定义细分哨兵（保持 50001/50002 等兼容）
// ============================================================

var (
	ErrDockerNotInstalled = &Error{Kind: KindUnavailable, Code: 50001, Message: "Docker 未安装或未启动"}
	ErrServiceNotReady    = &Error{Kind: KindUnavailable, Code: 50002, Message: "服务未就绪"}
)

// ============================================================
// 工厂与便捷构造函数
// ============================================================

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

// Wrap 包装底层错误，Message 取 Kind 默认文案
func Wrap(kind Kind, cause error) error {
	if cause == nil {
		return nil
	}
	return &Error{
		Kind:    kind,
		Message: defaultMessage(kind),
		Cause:   cause,
	}
}

func format(kind Kind, msg string, args ...any) error {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	return &Error{
		Kind:    kind,
		Message: msg,
	}
}

func BadRequest(msg string, args ...any) error     { return format(KindBadRequest, msg, args...) }
func Unauthorized(msg string, args ...any) error   { return format(KindUnauthorized, msg, args...) }
func Forbidden(msg string, args ...any) error      { return format(KindForbidden, msg, args...) }
func NotFound(msg string, args ...any) error       { return format(KindNotFound, msg, args...) }
func Conflict(msg string, args ...any) error       { return format(KindConflict, msg, args...) }
func RateLimit(msg string, args ...any) error      { return format(KindRateLimit, msg, args...) }
func Internal(msg string, args ...any) error       { return format(KindInternal, msg, args...) }
func Unavailable(msg string, args ...any) error    { return format(KindUnavailable, msg, args...) }
func Timeout(msg string, args ...any) error        { return format(KindTimeout, msg, args...) }
func NotImplemented(msg string, args ...any) error { return format(KindNotImplemented, msg, args...) }

// ============================================================
// 日志脱敏内部函数
// ============================================================

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

func sanitizeSensitiveInfo(s string) string {
	lower := strings.ToLower(s)
	for _, pattern := range sensitivePatterns {
		idx := strings.Index(lower, pattern)
		if idx >= 0 {
			start := idx + len(pattern)
			if start < len(s) {
				for start < len(s) && (s[start] == ':' || s[start] == '=' || s[start] == ' ') {
					start++
				}
				end := start
				for end < len(s) && s[end] != ' ' && s[end] != ',' && s[end] != '\n' {
					end++
				}
				if end > start {
					s = s[:start] + "[REDACTED]" + s[end:]
					lower = strings.ToLower(s)
				}
			}
		}
	}
	return s
}
