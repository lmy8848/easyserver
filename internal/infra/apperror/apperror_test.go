package apperror

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapError_NilInput(t *testing.T) {
	result := WrapError(nil)
	assert.NoError(t, result)
}

func TestWrapError_NotFound(t *testing.T) {
	err := errors.New("resource not found in database")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusNotFound, appErr.HTTPStatus)
	assert.Equal(t, CodeNotFound, appErr.Code)
	assert.Equal(t, err.Error(), appErr.Message)
}

func TestWrapError_PathTraversal(t *testing.T) {
	err := errors.New("path traversal detected")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusForbidden, appErr.HTTPStatus)
	assert.Equal(t, CodeForbidden, appErr.Code)
}

func TestWrapError_DockerNotInstalled(t *testing.T) {
	err := errors.New("docker is not installed")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	assert.Equal(t, CodeDockerNotInstalled, appErr.Code)
	assert.Contains(t, appErr.Message, "docker is not installed")
}

func TestWrapError_InvalidPassword(t *testing.T) {
	err := errors.New("invalid password")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusUnauthorized, appErr.HTTPStatus)
	assert.Equal(t, CodeUnauthorized, appErr.Code)
}

func TestWrapError_UniqueConstraint(t *testing.T) {
	err := errors.New("UNIQUE constraint failed: users.email")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	assert.Equal(t, CodeConflict, appErr.Code)
}

func TestWrapError_UnknownError(t *testing.T) {
	err := errors.New("some completely unknown error")
	result := WrapError(err)

	var appErr *AppError
	require.ErrorAs(t, result, &appErr)
	assert.Equal(t, http.StatusInternalServerError, appErr.HTTPStatus)
	assert.Equal(t, CodeInternalError, appErr.Code)
	// Unknown errors should wrap the original
	assert.Equal(t, err, appErr.Unwrap())
}

func TestWrapError_AlreadyAppError(t *testing.T) {
	// If the error is already an AppError, WrapError should return it as-is
	result := WrapError(ErrNotFound)
	assert.Equal(t, ErrNotFound, result)
}

func TestAppError_WithMessage(t *testing.T) {
	custom := ErrNotFound.WithMessage("用户不存在")
	assert.Equal(t, "用户不存在", custom.Message)
	assert.Equal(t, http.StatusNotFound, custom.HTTPStatus)
	assert.Equal(t, CodeNotFound, custom.Code)
	// Original should be unchanged
	assert.Equal(t, "资源不存在", ErrNotFound.Message)
}

func TestAppError_Wrap(t *testing.T) {
	inner := errors.New("disk full")
	wrapped := ErrInternal.Wrap(inner)

	assert.Equal(t, http.StatusInternalServerError, wrapped.HTTPStatus)
	assert.Equal(t, inner, wrapped.Err)
	assert.Equal(t, inner, wrapped.Unwrap())
	assert.Contains(t, wrapped.Error(), "内部服务器错误")
	assert.Contains(t, wrapped.Error(), "disk full")
}

func TestAppError_ErrorString(t *testing.T) {
	// Without underlying error
	e := &AppError{Message: "test msg"}
	assert.Equal(t, "test msg", e.Error())

	// With underlying error
	e2 := &AppError{Message: "outer", Err: errors.New("inner")}
	assert.Equal(t, "outer: inner", e2.Error())
}

// --- Is 语义：WithMessage/Wrap 副本与哨兵的 errors.Is 匹配 ---

func TestAppError_Is_WithMessageCopy(t *testing.T) {
	// 副本与哨兵：Code 相同 → errors.Is 成立
	err := ErrNotFound.WithMessage("用户不存在")
	require.ErrorIs(t, err, ErrNotFound)
	require.NotErrorIs(t, err, ErrBadRequest)

	// 反向：哨兵与副本
	require.ErrorIs(t, ErrNotFound, ErrNotFound.WithMessage("x"))
}

func TestAppError_Is_WrappedCopy(t *testing.T) {
	err := ErrInternal.Wrap(errors.New("disk full"))
	require.ErrorIs(t, err, ErrInternal)
	require.NotErrorIs(t, err, ErrBadRequest)
}

func TestAppError_Is_Chain(t *testing.T) {
	// 链式：AppError 包普通错误，errors.Is 穿透
	inner := errors.New("disk full")
	err := ErrInternal.Wrap(inner)
	require.ErrorIs(t, err, ErrInternal)
	require.NotErrorIs(t, inner, ErrInternal)

	// fmt.Errorf %w 再包一层
	outer := fmt.Errorf("outer: %w", err)
	require.ErrorIs(t, outer, ErrInternal)
}

func TestAppError_Is_SameCategorySentinels(t *testing.T) {
	// 同 HTTP 分类的细分哨兵共享 Code：相互 errors.Is 匹配（分类粒度 = HTTP 分类）
	pathErr := ErrPathViolation.WithMessage("路径越权")
	require.ErrorIs(t, pathErr, ErrPathViolation)
	require.ErrorIs(t, pathErr, ErrForbidden) // 同为 403 分类

	// token 无效归入 401 分类
	tokenErr := ErrTokenExpired.WithMessage("invalid or expired token")
	require.ErrorIs(t, tokenErr, ErrTokenExpired)
	require.ErrorIs(t, tokenErr, ErrUnauthorized)

	// 400 段细分哨兵保持独立 Code：与 ErrBadRequest 互不匹配
	dockerErr := ErrDockerNotInstalled.WithMessage("Docker 未安装")
	require.ErrorIs(t, dockerErr, ErrDockerNotInstalled)
	require.NotErrorIs(t, dockerErr, ErrBadRequest)
}

func TestAppError_Is_NonAppErrorTarget(t *testing.T) {
	err := ErrBadRequest.WithMessage("x")
	require.NotErrorIs(t, err, errors.New("请求参数错误"))
}
