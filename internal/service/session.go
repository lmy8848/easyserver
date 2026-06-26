package service

import (
	"easyserver/internal/auth"
)

// SessionService migrated to auth package; kept as alias for backward compatibility.
type SessionService = auth.SessionService

// NewSessionService creates a SessionService.
func NewSessionService(sessionRepo auth.SessionRepo) *SessionService {
	return auth.NewSessionService(sessionRepo)
}
