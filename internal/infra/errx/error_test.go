package errx

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKind_ErrorString(t *testing.T) {
	assert.Equal(t, "bad_request", KindBadRequest.Error())
	assert.Equal(t, "unauthorized", KindUnauthorized.Error())
	assert.Equal(t, "forbidden", KindForbidden.Error())
	assert.Equal(t, "not_found", KindNotFound.Error())
	assert.Equal(t, "conflict", KindConflict.Error())
	assert.Equal(t, "rate_limit", KindRateLimit.Error())
	assert.Equal(t, "internal", KindInternal.Error())
	assert.Equal(t, "unavailable", KindUnavailable.Error())
	assert.Equal(t, "timeout", KindTimeout.Error())
	assert.Equal(t, "not_implemented", KindNotImplemented.Error())
	assert.Equal(t, "unknown", Kind(99).Error())
}

func TestConvenienceConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind Kind
		wantMsg  string
	}{
		{"BadRequest plain", BadRequest("invalid param"), KindBadRequest, "invalid param"},
		{"BadRequest format", BadRequest("invalid id: %d", 123), KindBadRequest, "invalid id: 123"},
		{"Unauthorized plain", Unauthorized("login required"), KindUnauthorized, "login required"},
		{"Forbidden plain", Forbidden("permission denied"), KindForbidden, "permission denied"},
		{"NotFound plain", NotFound("item not found"), KindNotFound, "item not found"},
		{"NotFound format", NotFound("item %s not found", "user1"), KindNotFound, "item user1 not found"},
		{"Conflict plain", Conflict("name exists"), KindConflict, "name exists"},
		{"RateLimit plain", RateLimit("too many requests"), KindRateLimit, "too many requests"},
		{"Internal plain", Internal("database error"), KindInternal, "database error"},
		{"Unavailable plain", Unavailable("service unavailable"), KindUnavailable, "service unavailable"},
		{"Timeout plain", Timeout("operation timed out"), KindTimeout, "operation timed out"},
		{"NotImplemented plain", NotImplemented("not implemented"), KindNotImplemented, "not implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var xErr *Error
			require.ErrorAs(t, tt.err, &xErr)
			assert.Equal(t, tt.wantKind, xErr.Kind)
			assert.Equal(t, tt.wantMsg, xErr.Message)
			assert.Equal(t, tt.wantMsg, xErr.Error())
			assert.NoError(t, xErr.Cause)
		})
	}
}

func TestNew_And_Wrap(t *testing.T) {
	t.Run("New with cause", func(t *testing.T) {
		cause := errors.New("sql: no rows")
		err := New(KindNotFound, "user not found", cause)
		var xErr *Error
		require.ErrorAs(t, err, &xErr)
		assert.Equal(t, KindNotFound, xErr.Kind)
		assert.Equal(t, "user not found", xErr.Message)
		assert.Equal(t, cause, xErr.Unwrap())
		assert.Equal(t, "user not found: sql: no rows", xErr.Error())
	})

	t.Run("New with empty msg", func(t *testing.T) {
		err := New(KindNotFound, "")
		var xErr *Error
		require.ErrorAs(t, err, &xErr)
		assert.Equal(t, "资源不存在", xErr.Message)
	})

	t.Run("Wrap with cause", func(t *testing.T) {
		cause := errors.New("disk full")
		err := Wrap(KindInternal, cause)
		var xErr *Error
		require.ErrorAs(t, err, &xErr)
		assert.Equal(t, KindInternal, xErr.Kind)
		assert.Equal(t, "内部服务器错误", xErr.Message)
		assert.Equal(t, cause, xErr.Cause)
		assert.Equal(t, "内部服务器错误: disk full", xErr.Error())
	})

	t.Run("Wrap nil returns nil", func(t *testing.T) {
		assert.NoError(t, Wrap(KindInternal, nil))
	})
}

func TestTwoTierIs(t *testing.T) {
	t.Run("Tier 1: Sentinel exact pointer match", func(t *testing.T) {
		wrappedSentinel := fmt.Errorf("wrapped: %w", ErrDockerNotInstalled)
		require.ErrorIs(t, wrappedSentinel, ErrDockerNotInstalled)
		require.NotErrorIs(t, wrappedSentinel, ErrServiceNotReady)
		require.NotErrorIs(t, ErrDockerNotInstalled, Internal("other internal"))
	})

	t.Run("Tier 2: Kind semantic match", func(t *testing.T) {
		nf1 := NotFound("file 1 not found")
		nf2 := NotFound("file 2 not found")

		// Both match KindNotFound
		require.ErrorIs(t, nf1, KindNotFound)
		require.ErrorIs(t, nf2, KindNotFound)
		require.NotErrorIs(t, nf1, KindBadRequest)

		// Sentinel also matches its underlying Kind
		require.ErrorIs(t, ErrDockerNotInstalled, KindUnavailable)
		require.ErrorIs(t, ErrServiceNotReady, KindUnavailable)

		// Different error instances don't pointer-match each other
		require.NotErrorIs(t, nf1, nf2)
	})

	t.Run("Unwrap chain penetration", func(t *testing.T) {
		root := errors.New("root cause")
		wrapped := New(KindInternal, "operation failed", root)
		outer := fmt.Errorf("outer wrapper: %w", wrapped)

		require.ErrorIs(t, outer, KindInternal)
		require.ErrorIs(t, outer, root)
	})

	t.Run("Non-errx target comparison", func(t *testing.T) {
		err := BadRequest("bad")
		require.NotErrorIs(t, err, errors.New("bad"))
	})
}

func TestSafeError_Sanitization(t *testing.T) {
	t.Run("Without cause", func(t *testing.T) {
		err := BadRequest("invalid name")
		var xErr *Error
		require.ErrorAs(t, err, &xErr)
		assert.Equal(t, "invalid name", xErr.SafeError())
	})

	t.Run("With sensitive tokens in cause", func(t *testing.T) {
		cause := errors.New("auth failed: token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz and password=Secret123, api_key: key-456")
		err := New(KindUnauthorized, "认证失败", cause)
		var xErr *Error
		require.ErrorAs(t, err, &xErr)

		safe := xErr.SafeError()
		assert.NotContains(t, safe, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz")
		assert.NotContains(t, safe, "Secret123")
		assert.NotContains(t, safe, "key-456")
		assert.Contains(t, safe, "[REDACTED]")
		assert.Contains(t, safe, "认证失败")
	})
}

var _ = context.Background // avoid unused import if any
