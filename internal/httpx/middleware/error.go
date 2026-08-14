package middleware

import (
	"errors"
	"log"
	"net/http"

	"easyserver/internal/infra/apperror"

	"github.com/gin-gonic/gin"
)

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
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

	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		// Log based on severity level
		switch {
		case appErr.HTTPStatus >= 500:
			// Server errors: full details for debugging（SafeError 过滤底层
			// 错误中的敏感值，如 token/password 字段）
			log.Printf("ERROR [%s %s] user=%v(%v) ip=%s: %s",
				method, path, username, userID, clientIP, appErr.SafeError())
		case appErr.HTTPStatus == 401 || appErr.HTTPStatus == 403:
			// Auth errors: security audit trail
			log.Printf("WARN  [%s %s] ip=%s: %s",
				method, path, clientIP, appErr.Message)
		case appErr.HTTPStatus >= 400:
			// Client errors: brief log
			log.Printf("WARN  [%s %s] user=%v ip=%s: %s",
				method, path, username, clientIP, appErr.Message)
		}

		c.JSON(appErr.HTTPStatus, errorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
			Data:    nil,
		})
		return
	}

	// Unknown error: always log full details
	log.Printf("ERROR [%s %s] user=%v(%v) ip=%s: %v",
		method, path, username, userID, clientIP, err)

	c.JSON(http.StatusInternalServerError, errorResponse{
		Code:    apperror.CodeInternalError,
		Message: "internal server error",
		Data:    nil,
	})
}
