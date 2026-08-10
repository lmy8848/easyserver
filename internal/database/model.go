package database

// DBType is the database engine enum.
type DBType string

const (
	DBTypeMySQL      DBType = "mysql"
	DBTypePostgreSQL DBType = "postgresql"
	DBTypeRedis      DBType = "redis"
)

// DBInstance is a container-backed Database Instance — the top-level resource of
// the database module. The container is addressed by ContainerID only; each
// instance owns one managed container, one named data volume, an instance-level
// config dir, a fixed image and a fixed runtime.
type DBInstance struct {
	ID              int64  `json:"id"`
	DBType          DBType `json:"db_type"` // mysql, postgresql, redis
	Version         string `json:"version"` // 5.7, 8.0, 13, 15, etc.
	Port            int    `json:"port"`
	Status          string `json:"status"` // running, stopped, unhealthy
	CreatedAt       string `json:"created_at"`
	ContainerEngine string `json:"container_engine"`
	Image           string `json:"image"`
	ContainerID     string `json:"container_id"`
	VolumeName      string `json:"volume_name"`
	ConfigDir       string `json:"config_dir"`
	BindAddress     string `json:"bind_address"`
	AdminPassword   string `json:"-"`
}

// CreateDBInstanceRequest is the request for installing a new database instance.
// Image is supplied by the client — the version/image catalogue lives in the
// front-end, not the backend.
type CreateDBInstanceRequest struct {
	Version         string `json:"version" binding:"required"`
	Image           string `json:"image" binding:"required"`
	Port            int    `json:"port" binding:"required"`
	ContainerEngine string `json:"container_engine"`
	BindAddress     string `json:"bind_address"`
}

// IsValidDBType reports whether t is one of the supported engine enums.
func IsValidDBType(t DBType) bool {
	switch t {
	case DBTypeMySQL, DBTypePostgreSQL, DBTypeRedis:
		return true
	}
	return false
}

// dockerImageBase returns the Docker Hub official image name for an engine. It
// is the only user-facing knob for the "更多版本" flow — the user picks a tag of
// this official image; the panel never builds arbitrary image names.
func dockerImageBase(dbType DBType) string {
	switch dbType {
	case DBTypeMySQL:
		return "mysql"
	case DBTypePostgreSQL:
		return "postgres"
	case DBTypeRedis:
		return "redis"
	}
	return ""
}

// Database is a logical database inside an instance, queried live from the
// engine — it is never persisted (the engine owns its databases; the panel only
// reflects them). Charset is fetched alongside the name in one query.
type Database struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

// DBUser is a database user, queried live from the engine. Privileges are not
// summarized here (grant/revoke are per-object); the grant flow adds new grants.
type DBUser struct {
	Username   string `json:"username"`
	Host       string `json:"host"`
	Privileges string `json:"privileges,omitempty"`
}

// DBBackup represents a database backup record.
type DBBackup struct {
	ID           int64  `json:"id"`
	DBType       DBType `json:"db_type"`
	DBInstanceID int64  `json:"db_version_id"`
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
	Name        string `json:"name" binding:"required"`
	Charset     string `json:"charset"`
	Description string `json:"description"`
}

// CreateDBUserRequest is the request for creating a database user.
type CreateDBUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Host     string `json:"host"`
}

// GrantRequest is the request for granting privileges to a user.
type GrantRequest struct {
	Database   string `json:"database" binding:"required"`
	Privileges string `json:"privileges" binding:"required"`
}

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
