package httpx

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// CodeSuccess 是 API 成功响应的业务状态码
const CodeSuccess = 0

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
		Code:    CodeSuccess,
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

// ============================================================
// H() 包装器
// ============================================================

// H wraps a handler returning (any, error) into a standard gin.HandlerFunc.
// On success, it calls Success(c, data).
// On error, it attaches the error to c via c.Error(err) and aborts, letting
// the ErrorHandler middleware render the unified error response.
func H(fn func(c *gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fn(c)
		if err != nil {
			c.Error(err)
			c.Abort()
			return
		}
		if c.Writer.Written() {
			return
		}
		Success(c, data)
	}
}

// ============================================================
// Content-Disposition 工具
// ============================================================

// FormatContentDisposition constructs an RFC 6266 & RFC 5987 compliant Content-Disposition header.
// It provides an ASCII fallback in filename="..." and the full UTF-8 filename in filename*=UTF-8”...
// This avoids sending raw non-ASCII bytes in HTTP headers which breaks in HTTP proxies and modern browsers.
func FormatContentDisposition(disposition, filename string) string {
	ascii := toASCIIFilename(filename)
	encoded := url.PathEscape(filename)
	return fmt.Sprintf(`%s; filename=%q; filename*=UTF-8''%s`, disposition, ascii, encoded)
}

func toASCIIFilename(filename string) string {
	var b strings.Builder
	for _, r := range filename {
		if r >= 0x20 && r <= 0x7E && r != '"' && r != '\\' && r != ';' {
			b.WriteRune(r)
		} else if r > 0x7E {
			b.WriteRune('_')
		}
	}
	res := b.String()
	ext := filepath.Ext(filename)
	if res == "" || res == ext {
		if ext != "" {
			return "file" + ext
		}
		return "file"
	}
	return res
}
