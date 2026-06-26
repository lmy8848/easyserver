package repository

import (
	"context"

	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/cron"
	"easyserver/internal/database_mgmt"
	"easyserver/internal/deploy"
	"easyserver/internal/envconfig"
	"easyserver/internal/monitor"
	"easyserver/internal/notification"
	"easyserver/internal/process"
	"easyserver/internal/systemprocess"
	"easyserver/internal/web"
)

// Auth domain interfaces migrated to internal/auth; kept as aliases.

type UserRepository = auth.UserRepo
type SessionRepository = auth.SessionRepo
type TokenBlacklistRepository = auth.TokenBlacklistRepo
type ActivityRepository = auth.ActivityRepo

// SignedAuditEntry, AuditRepository, AuditFilter are now defined in internal/audit; kept as aliases.
type SignedAuditEntry = audit.SignedAuditEntry
type AuditRepository = audit.Repository
type AuditFilter = audit.AuditFilter

// MonitorRepository is now defined in easyserver/internal/monitor.Repository.
// Kept as alias for backward compatibility.
type MonitorRepository = monitor.Repository

// NotificationRepository is now defined in easyserver/internal/notification.Repository.
// Kept as alias for backward compatibility.
type NotificationRepository = notification.Repository

// TOTPRepository defines the interface for TOTP data access
type TOTPRepository interface {
	EnableTOTP(ctx context.Context, userID int64, secret string, hashedCodesJSON string) error
	DisableTOTP(ctx context.Context, userID int64) error
	GetPasswordHash(ctx context.Context, userID int64) (string, error)
	GetBackupCodes(ctx context.Context, userID int64) (string, error)
	UpdateBackupCodes(ctx context.Context, userID int64, codesJSON string) error
	IsTOTPEnabled(ctx context.Context, userID int64) (bool, error)
	GetTOTPSecret(ctx context.Context, userID int64) (string, error)
	GetPendingSecret(ctx context.Context, userID int64) (string, error)
	StorePendingSecret(ctx context.Context, userID int64, secret string) error
}

// CronRepository is now defined in easyserver/internal/cron.Repository.
// Kept as alias for backward compatibility.
type CronRepository = cron.Repository

// DatabaseMgmtRepository is now defined in easyserver/internal/database_mgmt.Repository.
// Kept as alias for backward compatibility.
type DatabaseMgmtRepository = database_mgmt.Repository

// WebServerRepository is now defined in internal/web.ServerRepository.
// Kept as alias for backward compatibility.
type WebServerRepository = web.ServerRepository

// WebsiteRepository is now defined in internal/web.WebsiteRepository.
// Kept as alias for backward compatibility.
type WebsiteRepository = web.WebsiteRepository

// DeployRepository is now defined in easyserver/internal/deploy.Repository.
// Kept as alias for backward compatibility.
type DeployRepository = deploy.Repository

// DBBackupRepository is now defined in easyserver/internal/database_mgmt.Repository.
// Kept as alias for backward compatibility.
type DBBackupRepository = database_mgmt.Repository

// EnvConfigRepository is now defined in easyserver/internal/envconfig.Repository.
// Kept as alias for backward compatibility.
type EnvConfigRepository = envconfig.Repository

// ServiceWhitelistRepository is now defined in easyserver/internal/systemprocess.Repository.
// Kept as alias for backward compatibility.
type ServiceWhitelistRepository = systemprocess.Repository

// ActivityRepository is now aliased above (auth.ActivityRepo).


// ProcessRepository is now defined in easyserver/internal/process.Repository.
// Kept as alias for backward compatibility.
type ProcessRepository = process.Repository
