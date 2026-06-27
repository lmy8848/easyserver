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
	assert.Nil(t, result)
}

func TestWrapError_NotFound(t *testing.T) {
	err := errors.New("resource not found in database")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
	assert.Equal(t, http.StatusNotFound, appErr.HTTPStatus)
	assert.Equal(t, CodeNotFound, appErr.Code)
	assert.Equal(t, err.Error(), appErr.Message)
}

func TestWrapError_PathTraversal(t *testing.T) {
	err := errors.New("path traversal detected")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
	assert.Equal(t, http.StatusForbidden, appErr.HTTPStatus)
	assert.Equal(t, CodeForbidden, appErr.Code)
}

func TestWrapError_DockerNotInstalled(t *testing.T) {
	err := errors.New("docker is not installed")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
	assert.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	assert.Equal(t, CodeBadRequest, appErr.Code)
	assert.Contains(t, appErr.Message, "docker is not installed")
}

func TestWrapError_InvalidPassword(t *testing.T) {
	err := errors.New("invalid password")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
	assert.Equal(t, http.StatusUnauthorized, appErr.HTTPStatus)
	assert.Equal(t, CodeUnauthorized, appErr.Code)
}

func TestWrapError_UniqueConstraint(t *testing.T) {
	err := errors.New("UNIQUE constraint failed: users.email")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
	assert.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	assert.Equal(t, CodeConflict, appErr.Code)
}

func TestWrapError_UnknownError(t *testing.T) {
	err := errors.New("some completely unknown error")
	result := WrapError(err)

	var appErr *AppError
	require.True(t, errors.As(result, &appErr))
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
	inner := fmt.Errorf("disk full")
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
