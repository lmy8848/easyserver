package model

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

// DBServer represents a database software type (MySQL, PostgreSQL, Redis)
type DBServer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`         // mysql, postgresql, redis
	DisplayName string `json:"display_name"` // MySQL, PostgreSQL, Redis
	Description string `json:"description"`
	DefaultPort int    `json:"default_port"`
	Status      string `json:"status"`  // not_installed, running, stopped, partial
	Version     string `json:"version"` // summary of installed versions
	CreatedAt   string `json:"created_at"`
}

// DBVersion represents an installed version of a database
type DBVersion struct {
	ID          int64  `json:"id"`
	DBServerID  int64  `json:"db_server_id"`
	Version     string `json:"version"`      // 5.7, 8.0, 13, 15, etc.
	ServiceName string `json:"service_name"` // mysql, mysql@5.7, postgresql@13
	ConfigFile  string `json:"config_file"`
	DataDir     string `json:"data_dir"`
	Port        int    `json:"port"`
	Status      string `json:"status"` // running, stopped
	CreatedAt   string `json:"created_at"`
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

type CreateDBVersionRequest struct {
	Version string `json:"version" binding:"required"`
	Port    int    `json:"port"`
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

// Version templates for each database type
type VersionTemplate struct {
	Version     string `json:"version"`
	Package     string `json:"package"`
	Description string `json:"description"`
}

func GetVersionTemplates(dbName string) []VersionTemplate {
	switch dbName {
	case "mysql":
		return []VersionTemplate{
			{Version: "10.11", Package: "default-mysql-server", Description: "MariaDB 10.11（Debian 默认）"},
			{Version: "10.5", Package: "mariadb-server-10.5", Description: "MariaDB 10.5（稳定版）"},
			{Version: "8.0", Package: "mysql-server-8.0", Description: "MySQL 8.0（需额外源）"},
		}
	case "postgresql":
		return []VersionTemplate{
			{Version: "15", Package: "postgresql-15", Description: "PostgreSQL 15（推荐）"},
			{Version: "13", Package: "postgresql-13", Description: "PostgreSQL 13（稳定）"},
		}
	case "redis":
		return []VersionTemplate{
			{Version: "7", Package: "redis-server", Description: "Redis 7（最新稳定）"},
			{Version: "6", Package: "redis-server", Description: "Redis 6"},
		}
	}
	return nil
}

// PredefinedDBServers returns the default database entries
func PredefinedDBServers() []DBServer {
	return []DBServer{
		{
			Name:        "mysql",
			DisplayName: "MySQL",
			Description: "最流行的关系型数据库，广泛用于 Web 应用",
			DefaultPort: 3306,
		},
		{
			Name:        "postgresql",
			DisplayName: "PostgreSQL",
			Description: "功能强大的开源关系型数据库",
			DefaultPort: 5432,
		},
		{
			Name:        "redis",
			DisplayName: "Redis",
			Description: "高性能内存数据库，用于缓存和消息队列",
			DefaultPort: 6379,
		},
	}
}
