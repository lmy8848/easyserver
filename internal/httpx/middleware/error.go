package middleware

import (
	"errors"
	"log"
	"net/http"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// mapKind maps an errx.Kind to standard HTTP status and business error code.
func mapKind(k errx.Kind) (status int, code int) {
	switch k {
	case errx.KindBadRequest:
		return http.StatusBadRequest, errx.CodeBadRequest
	case errx.KindUnauthorized:
		return http.StatusUnauthorized, errx.CodeUnauthorized
	case errx.KindForbidden:
		return http.StatusForbidden, errx.CodeForbidden
	case errx.KindNotFound:
		return http.StatusNotFound, errx.CodeNotFound
	case errx.KindConflict:
		return http.StatusConflict, errx.CodeConflict
	case errx.KindRateLimit:
		return http.StatusTooManyRequests, errx.CodeRateLimit
	case errx.KindNotImplemented:
		return http.StatusNotImplemented, errx.CodeNotImplemented
	case errx.KindUnavailable:
		return http.StatusServiceUnavailable, errx.CodeUnavailable
	case errx.KindTimeout:
		return http.StatusGatewayTimeout, errx.CodeTimeout
	case errx.KindInternal:
		fallthrough
	default:
		return http.StatusInternalServerError, errx.CodeInternalError
	}
}

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

	var status, code int
	var message, safeLog string

	var xErr *errx.Error

	switch {
	case errors.As(err, &xErr):
		status, code = mapKind(xErr.Kind)
		if xErr.Code != 0 {
			code = xErr.Code
		}
		message = xErr.Message
		safeLog = xErr.SafeError()

	default:
		status = http.StatusInternalServerError
		code = errx.CodeInternalError
		message = "internal server error"
		safeLog = err.Error()
	}

	// Log based on severity level
	switch {
	case status >= 500:
		// Server errors: full details for debugging (SafeError filters out sensitive values)
		log.Printf("ERROR [%s %s] user=%v(%v) ip=%s: %s",
			method, path, username, userID, clientIP, safeLog)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// Auth errors: security audit trail
		log.Printf("WARN  [%s %s] ip=%s: %s",
			method, path, clientIP, message)
	case status >= 400:
		// Client errors: brief log
		log.Printf("WARN  [%s %s] user=%v ip=%s: %s",
			method, path, username, clientIP, message)
	}

	c.JSON(status, errorResponse{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}
