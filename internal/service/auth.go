package service

import (
	"time"

	"easyserver/internal/auth"
)

// AuthService migrated to auth package; kept as alias for backward compatibility.
type AuthService = auth.AuthService

// NewAuthService creates a new AuthService.
func NewAuthService(maxAttempts int, lockoutDuration time.Duration) *AuthService {
	return auth.NewAuthService(maxAttempts, lockoutDuration)
}
