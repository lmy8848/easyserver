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

// 已迁移到产生端显式分类的场景（WrapError 不再嗅探）：
//   - path traversal → filemanager 直接返回 ErrForbidden
//   - invalid password / invalid TOTP code → auth 直接返回 ErrUnauthorized
//   - docker 系列 → container 直接返回 ErrDockerNotInstalled / ErrBadRequest
//   - not found / 已存在 / cannot be empty 等 → 各领域产生端显式分类
// 这些错误若再裸传给 WrapError，会按未知错误走 500 —— 这正是迁移的预期
// 行为（分类责任在产生端）。

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

func TestAppError_WrapMessage(t *testing.T) {
	inner := errors.New("UNIQUE constraint failed: users.email")
	wrapped := ErrConflict.WrapMessage(inner)

	// 分类保留哨兵语义
	assert.Equal(t, http.StatusConflict, wrapped.HTTPStatus)
	assert.Equal(t, CodeConflict, wrapped.Code)
	// 用户可见消息 = 底层错误文本
	assert.Equal(t, "UNIQUE constraint failed: users.email", wrapped.Message)
	// 原始错误留在链上：Unwrap + errors.Is/As 穿透 + errors.Is 匹配哨兵
	assert.Equal(t, inner, wrapped.Unwrap())
	require.ErrorIs(t, wrapped, ErrConflict)
	var target *AppError
	require.ErrorAs(t, wrapped, &target)
	assert.Equal(t, inner, target.Unwrap())
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

func TestAppError_Is_TokenInvalidCode(t *testing.T) {
	// token 无效保持独立 Code（40101）：匹配自身哨兵，但不匹配 ErrUnauthorized
	tokenErr := ErrTokenExpired.WithMessage("invalid or expired token")
	require.ErrorIs(t, tokenErr, ErrTokenExpired)
	require.NotErrorIs(t, tokenErr, ErrUnauthorized)

	// 400 段细分哨兵保持独立 Code：与 ErrBadRequest 互不匹配
	dockerErr := ErrDockerNotInstalled.WithMessage("Docker 未安装")
	require.ErrorIs(t, dockerErr, ErrDockerNotInstalled)
	require.NotErrorIs(t, dockerErr, ErrBadRequest)
}

func TestAppError_Is_NonAppErrorTarget(t *testing.T) {
	err := ErrBadRequest.WithMessage("x")
	require.NotErrorIs(t, err, errors.New("请求参数错误"))
}

// --- 预定义错误约束：Code 全局唯一，且与 HTTP 分类一致 ---

func TestPredefinedErrors_UniqueCodes(t *testing.T) {
	// 新增预定义哨兵时必须加入此列表：Code 是 errors.Is 的分类/身份标识，
	// 重复会让两个不同错误互相匹配，破坏调用方判断。
	sentinels := []*AppError{
		ErrBadRequest, ErrUnauthorized, ErrTokenExpired, ErrForbidden,
		ErrNotFound, ErrConflict, ErrRateLimit, ErrInternal,
		ErrDockerNotInstalled, ErrServiceNotReady,
	}

	seen := make(map[int]string, len(sentinels))
	for _, s := range sentinels {
		if prev, ok := seen[s.Code]; ok {
			t.Errorf("code %d duplicated: %q 与 %q 共用", s.Code, prev, s.Message)
		}
		seen[s.Code] = s.Message

		// Code 与 HTTP 分类一致（40000 段 ↔ 400，40100 ↔ 401 ...）
		if want := s.HTTPStatus * 100; s.Code < want || s.Code >= want+100 {
			t.Errorf("code %d 与 HTTPStatus %d 分类不符（应在 %d~%d 段）", s.Code, s.HTTPStatus, want, want+99)
		}
	}
}
