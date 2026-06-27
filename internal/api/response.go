package api

import (
	"errors"
	"log"
	"net/http"

	"easyserver/internal/infra/apperror"

	"github.com/gin-gonic/gin"
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
// 响应格式
// ============================================================

// Response is the standard API response format
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// PaginatedData is the paginated response data
type PaginatedData struct {
	Total int64       `json:"total"`
	Items interface{} `json:"items"`
}

// ============================================================
// 成功响应
// ============================================================

// Success returns a success response
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "ok",
		Data:    data,
	})
}

// SuccessPaginated returns a paginated success response
func SuccessPaginated(c *gin.Context, total int64, items interface{}) {
	Success(c, PaginatedData{
		Total: total,
		Items: items,
	})
}

// ============================================================
// ErrorHandler: 全局错误处理中间件
// ============================================================

// ErrorHandler is a middleware that processes errors added to the gin context
// via c.Error() and converts them to appropriate HTTP responses.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if !c.Writer.Written() && len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			handleError(c, err)
		}
	}
}

// handleError converts an error to the appropriate HTTP response with proper logging
func handleError(c *gin.Context, err error) {
	// Extract request context for logging
	method := c.Request.Method
	path := c.Request.URL.Path
	clientIP := c.ClientIP()
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		// Log based on severity level
		switch {
		case appErr.HTTPStatus >= 500:
			// Server errors: full details for debugging
			log.Printf("ERROR [%s %s] user=%v(%v) ip=%s: %v",
				method, path, username, userID, clientIP, appErr)
		case appErr.HTTPStatus == 401 || appErr.HTTPStatus == 403:
			// Auth errors: security audit trail
			log.Printf("WARN  [%s %s] ip=%s: %s",
				method, path, clientIP, appErr.Message)
		case appErr.HTTPStatus >= 400:
			// Client errors: brief log
			log.Printf("WARN  [%s %s] user=%v ip=%s: %s",
				method, path, username, clientIP, appErr.Message)
		}

		c.JSON(appErr.HTTPStatus, Response{
			Code:    appErr.Code,
			Message: appErr.Message,
			Data:    nil,
		})
		return
	}

	// Unknown error: always log full details
	log.Printf("ERROR [%s %s] user=%v(%v) ip=%s: %v",
		method, path, username, userID, clientIP, err)

	c.JSON(http.StatusInternalServerError, Response{
		Code:    CodeInternalError,
		Message: "internal server error",
		Data:    nil,
	})
}
