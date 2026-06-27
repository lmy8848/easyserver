package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateToken_DifferentSecrets(t *testing.T) {
	secret1 := "test-secret-key-at-least-32-bytes-long"
	secret2 := "different-secret-key-at-least-32-bytes"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token1, err := GenerateToken(secret1, userID, username, role, timeout)
	assert.NoError(t, err)

	token2, err := GenerateToken(secret2, userID, username, role, timeout)
	assert.NoError(t, err)

	assert.NotEqual(t, token1, token2)
}

func TestGenerateTOTPTempToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateTOTPTempToken_Valid(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)

	validatedUserID, err := ValidateTOTPTempToken(secret, token)
	assert.NoError(t, err)
	assert.Equal(t, userID, validatedUserID)
}

func TestValidateTOTPTempToken_InvalidSecret(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	wrongSecret := "wrong-secret-key-at-least-32-bytes-"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)

	_, err = ValidateTOTPTempToken(wrongSecret, token)
	assert.Error(t, err)
}
