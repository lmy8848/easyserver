package httpx

import (
	"net/http"

	"easyserver/internal/infra/apperror"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 响应格式
// ============================================================

// Response is the standard API response format
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// PaginatedData is the paginated response data
type PaginatedData struct {
	Total int64 `json:"total"`
	Items any   `json:"items"`
}

// ============================================================
// 成功响应
// ============================================================

// Success returns a success response
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    apperror.CodeSuccess,
		Message: "ok",
		Data:    data,
	})
}

// SuccessPaginated returns a paginated success response
func SuccessPaginated(c *gin.Context, total int64, items any) {
	Success(c, PaginatedData{
		Total: total,
		Items: items,
	})
}
