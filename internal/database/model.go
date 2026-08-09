package database

// DBServer is the API representation of a database engine catalog entry.
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

// DBInstance is a container-backed Database Instance — the top-level resource of
// the database module. Each instance owns one managed container, one named data
// volume, an instance-level config dir, a fixed image and a fixed runtime.
type DBInstance struct {
	ID                 int64  `json:"id"`
	DBServerID         int64  `json:"db_server_id"`
	Version            string `json:"version"`        // 5.7, 8.0, 13, 15, etc.
	ContainerName      string `json:"container_name"` // managed container name
	Port               int    `json:"port"`
	Status             string `json:"status"` // running, stopped, unhealthy
	CreatedAt          string `json:"created_at"`
	Runtime            string `json:"runtime"`
	Image              string `json:"image"`
	ContainerID        string `json:"container_id"`
	VolumeName         string `json:"volume_name"`
	ConfigDir          string `json:"config_dir"`
	BindAddress        string `json:"bind_address"`
	AdminUser          string `json:"admin_user"`
	AdminPassword      string `json:"-"`
	AdminPasswordPlain string `json:"admin_password,omitempty"`
	HealthStatus       string `json:"health_status"`
}

// CreateDBInstanceRequest is the request for installing a new database instance
type CreateDBInstanceRequest struct {
	Version     string `json:"version" binding:"required"`
	Port        int    `json:"port"`
	Runtime     string `json:"runtime"`
	BindAddress string `json:"bind_address"`
}

// VersionTemplate describes a package template for a database version
type VersionTemplate struct {
	Version     string `json:"version"`
	Package     string `json:"package"`
	Image       string `json:"image"`
	Description string `json:"description"`
}

// GetVersionTemplates returns available version templates for a database type
func GetVersionTemplates(dbName string) []VersionTemplate {
	switch dbName {
	case "mysql":
		return []VersionTemplate{
			{Version: "8.0", Image: "mysql:8.0", Description: "MySQL 8.0"},
			{Version: "8.4", Image: "mysql:8.4", Description: "MySQL 8.4 LTS"},
		}
	case "postgresql":
		return []VersionTemplate{
			{Version: "15", Image: "postgres:15", Description: "PostgreSQL 15"},
			{Version: "16", Image: "postgres:16", Description: "PostgreSQL 16"},
		}
	case "redis":
		return []VersionTemplate{
			{Version: "7", Image: "redis:7-alpine", Description: "Redis 7"},
			{Version: "6", Image: "redis:6-alpine", Description: "Redis 6"},
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

// Database represents a logical database inside an instance.
type Database struct {
	ID           int64  `json:"id"`
	DBServerID   int64  `json:"db_server_id"`
	DBInstanceID int64  `json:"db_version_id"`
	Name         string `json:"name"`
	Charset      string `json:"charset"`
	Description  string `json:"description"`
	SizeBytes    int64  `json:"size_bytes"`
	Status       string `json:"status"`
	Version      string `json:"version"` // which version this DB belongs to
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// DBUser represents a database user (shared across instances).
type DBUser struct {
	ID         int64  `json:"id"`
	DBServerID int64  `json:"db_server_id"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	Host       string `json:"host"`
	Privileges string `json:"privileges"`
	CreatedAt  string `json:"created_at"`
}

// DBBackup represents a database backup record.
type DBBackup struct {
	ID           int64  `json:"id"`
	DBServerID   int64  `json:"db_server_id"`
	DBInstanceID int64  `json:"db_version_id"`
	DatabaseID   int64  `json:"database_id"`
	DatabaseName string `json:"database_name"`
	BackupType   string `json:"backup_type"` // manual, scheduled
	FilePath     string `json:"file_path"`
	FileSize     int64  `json:"file_size"`
	Status       string `json:"status"` // pending, completed, failed
	ErrorMessage string `json:"error_message"`
	CreatedAt    string `json:"created_at"`
}

// CreateDatabaseRequest is the request for creating a database.
type CreateDatabaseRequest struct {
	DBInstanceID int64  `json:"db_version_id" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Charset      string `json:"charset"`
	Description  string `json:"description"`
}

// CreateDBUserRequest is the request for creating a database user.
type CreateDBUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Host     string `json:"host"`
}

// GrantRequest is the request for granting privileges.
type GrantRequest struct {
	DBInstanceID int64  `json:"db_version_id" binding:"required"`
	Database     string `json:"database" binding:"required"`
	Privileges   string `json:"privileges" binding:"required"`
}

// DBType represents the database engine type.
type DBType string

const (
	DBTypeMySQL      DBType = "mysql"
	DBTypePostgreSQL DBType = "postgresql"
	DBTypeRedis      DBType = "redis"
)

// ColumnInfo represents a column's metadata.
type ColumnInfo struct {
	Name         string
	Type         string
	IsPrimaryKey bool
	IsAutoIncr   bool
	HasDefault   bool
	DefaultValue string
	IsNullable   bool
}

// TableInfo represents a table's metadata.
type TableInfo struct {
	Name       string
	PrimaryKey string
	Columns    []ColumnInfo
}

// DMLResult is the response for ExecuteSQL / Insert / Update / Delete.
type DMLResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
	SQL     string `json:"sql,omitempty"`
}

// PagedQueryResult is the response for QueryTable.
type PagedQueryResult struct {
	Headers  []string        `json:"headers"`
	Rows     [][]interface{} `json:"rows"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// DescribeResult is the response for DescribeTable.
type DescribeResult struct {
	TableName  string                   `json:"table_name"`
	PrimaryKey string                   `json:"primary_key"`
	Columns    []map[string]interface{} `json:"columns"`
}

// TableColumn describes a column for CreateTable.
type TableColumn struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	IsPrimary bool   `json:"is_primary"`
	AutoIncr  bool   `json:"auto_incr"`
}

// ValidationResult represents a validation result.
type ValidationResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
	SQL     string `json:"sql,omitempty"`
}

// ConfigSection represents a section in a config file (e.g., [mysqld]).
type ConfigSection struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}

// DBConfig represents a parsed database config file.
type DBConfig struct {
	FilePath string          `json:"file_path"`
	Sections []ConfigSection `json:"sections"`
}

// ParamMeta defines UI metadata for common configuration parameters.
type ParamMeta struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Type        string   `json:"type"`              // text, number, select, boolean
	Unit        string   `json:"unit"`              // MB, GB, etc.
	Options     []string `json:"options,omitempty"` // for select type
	Default     string   `json:"default"`
}
