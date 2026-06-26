package repository

import (
	"context"
	"database/sql"
	"time"

	"easyserver/internal/model"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, offset, limit int) ([]model.User, int64, error)
	UpdateLoginAttempts(ctx context.Context, id int64, attempts int, lockedUntil *time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	SetMustChangePass(ctx context.Context, id int64, mustChange bool) error
	IncrementLoginAttempts(ctx context.Context, id int64) error
	IncrementLoginAttemptsWithLock(ctx context.Context, id int64, maxAttempts int, lockoutSeconds int) error
	ResetLoginState(ctx context.Context, id int64, ip string) error
	UpdateLastLoginIP(ctx context.Context, id int64, ip string) error
	SetAccountExpiry(ctx context.Context, id int64, expiresAt *time.Time) error
	GetAccountExpiry(ctx context.Context, id int64) (sql.NullTime, error)
	SetIPWhitelist(ctx context.Context, id int64, whitelist string) error
	GetIPWhitelist(ctx context.Context, id int64) (string, error)
}

// SessionRepository defines the interface for session data access
type SessionRepository interface {
	Create(ctx context.Context, session *model.Session) error
	GetByToken(ctx context.Context, token string) (*model.Session, error)
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	DeleteInactive(ctx context.Context, inactiveSince time.Time) error
	DeleteByUserIDExcept(ctx context.Context, userID int64, exceptToken string) error
	IsValid(ctx context.Context, token string) (bool, error)
	GetActiveByUserID(ctx context.Context, userID int64) ([]model.Session, error)
	GetActive(ctx context.Context) ([]model.Session, error)
	UpdateActivity(ctx context.Context, token string) error
	Count(ctx context.Context) (int, error)
}

// TokenBlacklistRepository defines the interface for token blacklist data access
type TokenBlacklistRepository interface {
	Add(ctx context.Context, userID int64, token string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
	AddUserInvalidation(ctx context.Context, userID int64) error
	IsUserInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error)
	Clean(ctx context.Context) error
}

// SignedAuditEntry represents an audit log entry with HMAC signature,
// used by AuditWriter.flush and VerifySignature.
type SignedAuditEntry struct {
	ID        int64
	UserID    int64
	Username  string
	Action    string
	Resource  string
	Detail    string
	IP        string
	UserAgent string
	CreatedAt time.Time
	Signature string
}

// AuditRepository defines the interface for audit log data access
type AuditRepository interface {
	Log(ctx context.Context, entry *model.AuditLog) error
	Query(ctx context.Context, filter AuditFilter) (int64, []model.AuditLog, error)
	GetActions(ctx context.Context) ([]string, error)
	Clean(ctx context.Context, before time.Time) (int64, error)

	// AppendSignedBatch inserts a batch of signed audit entries in a single transaction.
	AppendSignedBatch(ctx context.Context, entries []SignedAuditEntry) error
	// GetSignedEntry returns a single signed audit entry by ID (including signature).
	GetSignedEntry(ctx context.Context, id int64) (*SignedAuditEntry, error)
	// ListIDsForVerification returns up to limit audit log IDs ordered by id DESC.
	ListIDsForVerification(ctx context.Context, limit int) ([]int64, error)
}

// AuditFilter defines the filter criteria for audit log queries
type AuditFilter struct {
	Username  string
	Action    string
	Resource  string
	IP        string
	StartDate string
	EndDate   string
	Offset    int
	Limit     int
}

// MonitorRepository defines the interface for monitor data access
type MonitorRepository interface {
	EnsureIndexes(ctx context.Context) error
	Save(ctx context.Context, point *model.MonitorPoint) error
	SaveBatch(ctx context.Context, points []*model.MonitorPoint) error
	GetLatest(ctx context.Context) (*model.MonitorPoint, error)
	GetHistory(ctx context.Context, start, end time.Time) ([]model.MonitorPoint, error)
	Clean(ctx context.Context, before time.Time) (int64, error)
}

// NotificationRepository defines the interface for notification data access
type NotificationRepository interface {
	List(ctx context.Context, unreadOnly bool, limit int) ([]model.Notification, error)
	CountUnread(ctx context.Context) (int, error)
	Create(ctx context.Context, req model.CreateNotificationRequest) error
	CreateIfNotExists(ctx context.Context, req model.CreateNotificationRequest) error
	MarkAsRead(ctx context.Context, id int64) error
	MarkAllAsRead(ctx context.Context) error
	Delete(ctx context.Context, id int64) error
	CleanOld(ctx context.Context, days int) (int64, error)
}

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

// CronRepository defines the interface for cron task data access
type CronRepository interface {
	// Task CRUD
	ListTasks(ctx context.Context) ([]model.CronTask, error)
	GetTask(ctx context.Context, id int64) (*model.CronTask, error)
	CreateTask(ctx context.Context, task *model.CronTask) error
	UpdateTask(ctx context.Context, task *model.CronTask) error
	DeleteTask(ctx context.Context, id int64) error

	// Task status management
	ListEnabledTasks(ctx context.Context) ([]model.CronTask, error)
	EnableTask(ctx context.Context, id int64) error
	DisableTask(ctx context.Context, id int64) error
	SetTaskRunning(ctx context.Context, id int64) (bool, error)
	UpdateTaskResult(ctx context.Context, id int64, status string, lastResult string) error

	// Logs
	CreateLog(ctx context.Context, taskID int64, status string, output string, duration int) error
	GetLogs(ctx context.Context, taskID int64, limit int) ([]model.CronLog, error)

	// Scripts
	ListScripts(ctx context.Context) ([]model.Script, error)
	GetScript(ctx context.Context, id int64) (*model.Script, error)
	CreateScript(ctx context.Context, script *model.Script) error
	UpdateScript(ctx context.Context, script *model.Script) error
	DeleteScript(ctx context.Context, id int64) error

	// Documentation
	ListDocs(ctx context.Context) ([]model.CronDoc, error)
	GetDoc(ctx context.Context, id int64) (*model.CronDoc, error)
	CreateDoc(ctx context.Context, doc *model.CronDoc) error
	UpdateDoc(ctx context.Context, doc *model.CronDoc) error
	DeleteDoc(ctx context.Context, id int64) error
	CountDocs(ctx context.Context) (int, error)
	BatchCreateDocs(ctx context.Context, docs []model.CronDoc) error
}

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

// WebServerRepository defines the interface for web server data access
type WebServerRepository interface {
	List(ctx context.Context) ([]model.WebServer, error)
	Get(ctx context.Context, id int64) (*model.WebServer, error)
	Create(ctx context.Context, ws *model.WebServer) error
	Delete(ctx context.Context, id int64) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateStatusAndVersion(ctx context.Context, id int64, status, version string) error
	CountWebsitesByServerID(ctx context.Context, serverID int64) (int, error)
}

// WebsiteRepository defines the interface for website data access
type WebsiteRepository interface {
	List(ctx context.Context, webServerID int64) ([]model.Website, error)
	Get(ctx context.Context, webServerID, id int64) (*model.Website, error)
	Create(ctx context.Context, w *model.Website) (int64, error)
	Update(ctx context.Context, w *model.Website) error
	Delete(ctx context.Context, webServerID, id int64) error
	UpdateStatus(ctx context.Context, webServerID, id int64, status string) error
	UpdateSSL(ctx context.Context, id int64, certPath, keyPath string) error
	CountByDomain(ctx context.Context, domain string) (int, error)
	CountByDomainExcludingID(ctx context.Context, domain string, excludeID int64) (int, error)
}

// DeployRepository defines the interface for deploy data access
type DeployRepository interface {
	// Server CRUD
	ListServers(ctx context.Context) ([]model.DeployServer, error)
	GetServer(ctx context.Context, id int64) (*model.DeployServer, error)
	GetServerAuthData(ctx context.Context, id int64) (string, error)
	CreateServer(ctx context.Context, srv *model.DeployServer) error
	UpdateServer(ctx context.Context, srv *model.DeployServer) error
	DeleteServer(ctx context.Context, id int64) error
	UpdateServerStatus(ctx context.Context, id int64, status string, lastPing string) error
	CountServerTasks(ctx context.Context, serverID int64) (int, error)
	CountServerVersions(ctx context.Context, serverID int64) (int, error)

	// Task CRUD
	ListTasks(ctx context.Context) ([]model.DeployTask, error)
	GetTask(ctx context.Context, id int64) (*model.DeployTask, error)
	ServerExists(ctx context.Context, id int64) (bool, error)
	CreateTask(ctx context.Context, task *model.DeployTask) error
	DeleteTask(ctx context.Context, id int64) error
	UpdateTaskStatus(ctx context.Context, id int64, status string, result string) error

	// Version CRUD
	ListVersions(ctx context.Context, serverID int64) ([]model.DeployVersion, error)
	GetVersion(ctx context.Context, id int64) (*model.DeployVersion, error)
	CreateVersion(ctx context.Context, ver *model.DeployVersion) error
}

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

// ActivityRepository defines the interface for user activity log data access
type ActivityRepository interface {
	Log(ctx context.Context, entry *model.UserActivity) error
	GetByUserID(ctx context.Context, userID int64, limit int) ([]model.UserActivity, error)
	GetAll(ctx context.Context, limit int) ([]model.UserActivity, error)
}

// PackageRepository defines the interface for package data access
type PackageRepository interface {
	List(ctx context.Context, runtimeID int64) ([]model.Package, error)
	Upsert(ctx context.Context, runtimeID int64, name, version, scope, source string) error
	Delete(ctx context.Context, runtimeID int64, name, scope string) error
}

// ProcessRepository defines the interface for process/process-group/process-log data access
type ProcessRepository interface {
	// Process CRUD
	ListProcesses(ctx context.Context) ([]model.Process, error)
	GetProcessByID(ctx context.Context, id int64) (*model.Process, error)
	CreateProcess(ctx context.Context, p *model.Process) (int64, error)
	UpdateProcess(ctx context.Context, id int64, req *model.UpdateProcessRequest) error
	DeleteProcess(ctx context.Context, id int64) error
	GetAutoStartIDs(ctx context.Context) ([]int64, error)

	// Process status
	UpsertStatus(ctx context.Context, processID int64, status string, pid int, exitCode int, lastError string) error
	GetStatus(ctx context.Context, processID int64) (*model.ProcessStatus, error)
	IncrementRestarts(ctx context.Context, processID int64) error
	ClearExitInfo(ctx context.Context, processID int64) error

	// Process logs
	AppendLog(ctx context.Context, processID int64, logType, content string) error
	ListLogs(ctx context.Context, processID int64, limit, offset int) ([]model.ProcessLog, int, error)

	// Process groups
	ListGroups(ctx context.Context) ([]model.ProcessGroup, error)
	GetGroup(ctx context.Context, id int64) (*model.ProcessGroup, error)
	CreateGroup(ctx context.Context, name, description string) (int64, error)
	UpdateGroup(ctx context.Context, id int64, req *model.UpdateProcessGroupRequest) error
	DeleteGroup(ctx context.Context, id int64) error
}

