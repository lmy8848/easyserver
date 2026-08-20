package httpx

import (
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// BindJSON parses the request body as JSON into type T.
// If binding or validation fails, it returns an errx.BadRequest error with %w cause wrapping.
func BindJSON[T any](c *gin.Context) (T, error) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		return req, errx.BadRequest("请求参数错误: %w", err)
	}
	return req, nil
}

// BindQuery parses query parameters into type T.
// If binding or validation fails, it returns an errx.BadRequest error with %w cause wrapping.
func BindQuery[T any](c *gin.Context) (T, error) {
	var req T
	if err := c.ShouldBindQuery(&req); err != nil {
		return req, errx.BadRequest("请求参数错误: %w", err)
	}
	return req, nil
}

// BindURI parses URI path parameters into type T.
// If binding or validation fails, it returns an errx.BadRequest error with %w cause wrapping.
func BindURI[T any](c *gin.Context) (T, error) {
	var req T
	if err := c.ShouldBindUri(&req); err != nil {
		return req, errx.BadRequest("请求路径参数错误: %w", err)
	}
	return req, nil
}
