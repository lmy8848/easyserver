package model

import "easyserver/internal/dbserver"

// DBServer is now defined in internal/dbserver. Alias for backward compatibility.
type DBServer = dbserver.DBServer

// DBVersion is now defined in internal/dbserver. Alias for backward compatibility.
type DBVersion = dbserver.DBVersion

// CreateDBVersionRequest is now defined in internal/dbserver. Alias for backward compatibility.
type CreateDBVersionRequest = dbserver.CreateDBVersionRequest

// VersionTemplate is now defined in internal/dbserver. Alias for backward compatibility.
type VersionTemplate = dbserver.VersionTemplate

// GetVersionTemplates delegates to dbserver.GetVersionTemplates.
var GetVersionTemplates = dbserver.GetVersionTemplates

// PredefinedDBServers delegates to dbserver.PredefinedDBServers.
var PredefinedDBServers = dbserver.PredefinedDBServers

// DBBackup represents a database backup record
type DBBackup struct {
	ID           int64  `json:"id"`
	DBServerID   int64  `json:"db_server_id"`
	DBVersionID  int64  `json:"db_version_id"`
	DatabaseID   int64  `json:"database_id"`
	DatabaseName string `json:"database_name"`
	BackupType   string `json:"backup_type"` // manual, scheduled
	FilePath     string `json:"file_path"`
	FileSize     int64  `json:"file_size"`
	Status       string `json:"status"` // pending, completed, failed
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
}

// Database represents a database instance
type Database struct {
	ID          int64  `json:"id"`
	DBServerID  int64  `json:"db_server_id"`
	DBVersionID int64  `json:"db_version_id"`
	Name        string `json:"name"`
	Charset     string `json:"charset"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	Status      string `json:"status"`
	Version     string `json:"version"` // which version this DB belongs to
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DBUser represents a database user (shared across versions)
type DBUser struct {
	ID         int64  `json:"id"`
	DBServerID int64  `json:"db_server_id"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	Host       string `json:"host"`
	Privileges string `json:"privileges"`
	CreatedAt  string `json:"created_at"`
}

type CreateDatabaseRequest struct {
	DBVersionID int64  `json:"db_version_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Charset     string `json:"charset"`
	Description string `json:"description"`
}

type CreateDBUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Host     string `json:"host"`
}

type GrantRequest struct {
	DBVersionID int64  `json:"db_version_id" binding:"required"`
	Database    string `json:"database" binding:"required"`
	Privileges  string `json:"privileges" binding:"required"`
}
