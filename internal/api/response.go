package api

import (
	"easyserver/internal/httpx"
	"easyserver/internal/infra/apperror"
)

// ============================================================
// 类型别名：从 apperror 包 re-export
// ============================================================

// AppError is the unified application error type
type AppError = apperror.AppError

// 预定义错误
var (
	ErrBadRequest         = apperror.ErrBadRequest
	ErrUnauthorized       = apperror.ErrUnauthorized
	ErrTokenExpired       = apperror.ErrTokenExpired
	ErrForbidden          = apperror.ErrForbidden
	ErrPathViolation      = apperror.ErrPathViolation
	ErrNotFound           = apperror.ErrNotFound
	ErrConflict           = apperror.ErrConflict
	ErrRateLimit          = apperror.ErrRateLimit
	ErrInternal           = apperror.ErrInternal
	ErrDockerNotInstalled = apperror.ErrDockerNotInstalled
	ErrServiceNotReady    = apperror.ErrServiceNotReady
)

// 错误码常量
const (
	CodeSuccess       = apperror.CodeSuccess
	CodeBadRequest    = apperror.CodeBadRequest
	CodeUnauthorized  = apperror.CodeUnauthorized
	CodeTokenExpired  = apperror.CodeTokenExpired
	CodeForbidden     = apperror.CodeForbidden
	CodeNotFound      = apperror.CodeNotFound
	CodeConflict      = apperror.CodeConflict
	CodeRateLimit     = apperror.CodeRateLimit
	CodeInternalError = apperror.CodeInternalError
)

// 错误分类和包装函数
var (
	IsPathError          = apperror.IsPathError
	IsDockerNotInstalled = apperror.IsDockerNotInstalled
	WrapError            = apperror.WrapError
)

// ============================================================
// 响应格式：转发到 httpx
// 迁移期间供仍留在 internal/api 的 handler 使用；各领域迁出后切换为
// 直接 import httpx，本 shim 随之删除。
// ============================================================

// Response is the standard API response format
type Response = httpx.Response

// PaginatedData is the paginated response data
type PaginatedData = httpx.PaginatedData

var (
	// Success returns a success response
	Success = httpx.Success
	// SuccessPaginated returns a paginated success response
	SuccessPaginated = httpx.SuccessPaginated
	// ErrorHandler processes errors added to the gin context
	ErrorHandler = httpx.ErrorHandler
)
