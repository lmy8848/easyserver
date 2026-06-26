package repository

import (
	"context"

	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/cron"
	"easyserver/internal/deploy"
	"easyserver/internal/model"
	"easyserver/internal/monitor"
	"easyserver/internal/notification"
	"easyserver/internal/process"
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

// DatabaseMgmtRepository defines the interface for database management data access
type DatabaseMgmtRepository interface {
	// Database operations
	ListDatabases(ctx context.Context, dbServerID int64) ([]model.Database, error)
	GetDatabase(ctx context.Context, dbServerID, id int64) (*model.Database, error)
	GetDatabaseByID(ctx context.Context, id int64) (*model.Database, error)
	CreateDatabase(ctx context.Context, dbServerID, dbVersionID int64, name, charset, description string) (int64, error)
	DeleteDatabase(ctx context.Context, dbServerID, id int64) error

	// DB User operations
	ListDBUsers(ctx context.Context, dbServerID int64) ([]model.DBUser, error)
	GetDBUser(ctx context.Context, dbServerID, id int64) (*model.DBUser, error)
	CreateDBUser(ctx context.Context, dbServerID int64, username, hashedPassword, host string) (int64, error)
	DeleteDBUser(ctx context.Context, dbServerID, id int64) error
	UpdateDBUserPrivileges(ctx context.Context, id int64, privileges string) error

	// Lookup helpers (lightweight queries)
	GetServer(ctx context.Context, id int64) (*model.DBServer, error)
	GetVersion(ctx context.Context, id int64) (*model.DBVersion, error)
	ListVersions(ctx context.Context, dbServerID int64) ([]model.DBVersion, error)
}

// WebServerRepository is now defined in internal/web.ServerRepository.
// Kept as alias for backward compatibility.
type WebServerRepository = web.ServerRepository

// WebsiteRepository is now defined in internal/web.WebsiteRepository.
// Kept as alias for backward compatibility.
type WebsiteRepository = web.WebsiteRepository

// DeployRepository is now defined in easyserver/internal/deploy.Repository.
// Kept as alias for backward compatibility.
type DeployRepository = deploy.Repository

// DBBackupRepository defines the interface for database backup data access
type DBBackupRepository interface {
	CreateBackup(ctx context.Context, backup *model.DBBackup) (int64, error)
	UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error
	ListBackups(ctx context.Context, databaseID int64) ([]model.DBBackup, error)
	GetBackup(ctx context.Context, id int64) (*model.DBBackup, error)
	DeleteBackup(ctx context.Context, id int64) error
}

// EnvConfigRepository defines the interface for environment config data access
type EnvConfigRepository interface {
	// EnvConfig CRUD
	ListEnvConfigs(ctx context.Context, runtimeID int64) ([]model.EnvConfig, error)
	GetEnvConfig(ctx context.Context, id int64) (*model.EnvConfig, error)
	CreateEnvConfig(ctx context.Context, config *model.EnvConfig) error
	UpdateEnvConfig(ctx context.Context, config *model.EnvConfig) error
	DeleteEnvConfig(ctx context.Context, id int64) error

	// PathEntry CRUD
	ListPathEntries(ctx context.Context, runtimeID int64) ([]model.PathEntry, error)
	CreatePathEntry(ctx context.Context, entry *model.PathEntry) error
	DeletePathEntry(ctx context.Context, id int64) error
	ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error

	// GlobalConfig CRUD
	ListGlobalConfigs(ctx context.Context, category string) ([]model.GlobalConfig, error)
	GetGlobalConfig(ctx context.Context, id int64) (*model.GlobalConfig, error)
	CreateGlobalConfig(ctx context.Context, config *model.GlobalConfig) error
	UpdateGlobalConfig(ctx context.Context, config *model.GlobalConfig) error
	DeleteGlobalConfig(ctx context.Context, id int64) error
	CreateGlobalConfigIfNotExists(ctx context.Context, config *model.GlobalConfig) error
}

// ServiceWhitelistRepository defines the interface for service whitelist data access
type ServiceWhitelistRepository interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]model.ServiceWhitelistEntry, error)
	Add(ctx context.Context, name string) error
	Delete(ctx context.Context, name string) error
}

// ActivityRepository is now aliased above (auth.ActivityRepo).


// ProcessRepository is now defined in easyserver/internal/process.Repository.
// Kept as alias for backward compatibility.
type ProcessRepository = process.Repository

