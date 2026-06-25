package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the standard API response format
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Error codes
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

// Success returns a success response
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "ok",
		Data:    data,
	})
}

// Error returns an error response
func Error(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// BadRequest returns a 400 error
func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, CodeBadRequest, message)
}

// Unauthorized returns a 401 error
func Unauthorized(c *gin.Context, message string) {
	Error(c, http.StatusUnauthorized, CodeUnauthorized, message)
}

// TokenExpired returns a 401 error with token expired code
func TokenExpired(c *gin.Context) {
	Error(c, http.StatusUnauthorized, CodeTokenExpired, "token expired")
}

// Forbidden returns a 403 error
func Forbidden(c *gin.Context, message string) {
	Error(c, http.StatusForbidden, CodeForbidden, message)
}

// NotFound returns a 404 error
func NotFound(c *gin.Context, message string) {
	Error(c, http.StatusNotFound, CodeNotFound, message)
}

// Conflict returns a 409 error
func Conflict(c *gin.Context, message string) {
	Error(c, http.StatusConflict, CodeConflict, message)
}

// internalServerErrorMsg is the generic message returned to clients on internal errors
const internalServerErrorMsg = "internal server error"

// InternalError logs the full error server-side and returns a generic message to the client.
func InternalError(c *gin.Context, message string) {
	log.Printf("InternalError [%s %s]: %s", c.Request.Method, c.Request.URL.Path, message)
	Error(c, http.StatusInternalServerError, CodeInternalError, internalServerErrorMsg)
}

// Paginated returns a paginated response
type PaginatedData struct {
	Total int64       `json:"total"`
	Items interface{} `json:"items"`
}

// SuccessPaginated returns a paginated success response
func SuccessPaginated(c *gin.Context, total int64, items interface{}) {
	Success(c, PaginatedData{
		Total: total,
		Items: items,
	})
}
