package model

import "easyserver/internal/auth"

// Types migrated to internal/auth; kept as aliases for backward compatibility.

type Role = auth.Role
type User = auth.User
type UserActivity = auth.UserActivity
type Session = auth.Session

const (
	RoleAdmin    = auth.RoleAdmin
	RoleOperator = auth.RoleOperator
	RoleViewer   = auth.RoleViewer
)
