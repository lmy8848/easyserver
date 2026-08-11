package database

// DBType is the database type enum.
type DBType string

const (
	DBTypeMySQL      DBType = "mysql"
	DBTypePostgreSQL DBType = "postgresql"
	DBTypeRedis      DBType = "redis"
)

// DBInstance is a container-backed Database Instance — the top-level resource of
// the database module. The container is addressed by ContainerName only; each
// instance owns one managed container, one named data volume, an instance-level
// config dir, a fixed image and a fixed runtime.
type DBInstance struct {
	ID              int64  `json:"id"`
	DBType          DBType `json:"db_type"` // mysql, postgresql, redis
	Version         string `json:"version"` // 5.7, 8.0, 13, 15, etc.
	Port            int    `json:"port"`    // host mapping port, chosen by the user
	Status          string `json:"status"`  // running, stopped, unhealthy
	CreatedAt       string `json:"created_at"`
	ContainerEngine string `json:"container_engine"`
	Image           string `json:"image"`
	ContainerName   string `json:"container_name"`
	VolumeName      string `json:"volume_name"`
	ConfigDir       string `json:"config_dir"`
	BindAddress     string `json:"bind_address"`
	AdminPassword   string `json:"-"`
}

// CreateDBInstanceRequest is the request for installing a new database instance.
// Image is supplied by the client — the version/image catalogue lives in the
// front-end, not the backend. ContainerName is optional; empty falls back to the
// deterministic default "easyserver-db-<type>-<version>".
type CreateDBInstanceRequest struct {
	DBType          DBType `json:"dbtype" binding:"required"`
	Version         string `json:"version" binding:"required"`
	Image           string `json:"image" binding:"required"`
	Port            int    `json:"port" binding:"required"`
	ContainerEngine string `json:"container_engine"`
	BindAddress     string `json:"bind_address"`
	ContainerName   string `json:"container_name"`
}

// IsValidDBType reports whether t is one of the supported database type enums.
func IsValidDBType(t DBType) bool {
	switch t {
	case DBTypeMySQL, DBTypePostgreSQL, DBTypeRedis:
		return true
	}
	return false
}

// dockerImageBase returns the Docker Hub official image name for a database type. It
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
// database server — it is never persisted (the server owns its databases; the panel only
// reflects them). Charset is fetched alongside the name in one query.
type Database struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

// DBUser is a database user, queried live from the database server. Privileges are not
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
	Status       string `json:"status"` // running, success, failed
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
	Headers     []string        `json:"headers"`
	ColumnTypes []string        `json:"column_types"`
	Rows        [][]interface{} `json:"rows"`
	Total       int             `json:"total"`
	Page        int             `json:"page"`
	PageSize    int             `json:"page_size"`
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

// ParamMeta defines UI metadata for common configuration parameters. Values are
// always strings (driven by the engine's own config semantics), so there is no
// type — the editor renders Select when Options are present, a text input
// otherwise. Key is the actual engine parameter name (zero conversion: the same
// string goes into SHOW VARIABLES filter / SET PERSIST / ALTER SYSTEM /
// CONFIG SET).
type ParamMeta struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`              // MB, GB, etc.
	Options     []string `json:"options,omitempty"` // select options (presence switches the editor to a Select)
}

// InstanceConfigView is the structured config of one instance (GET /config):
// Params is the engine's current values for the panel-managed parameters
// (read back from the driver channel), Meta the editor metadata. There is no
// multi-section shape — the driver channel addresses a single config namespace
// per engine.
type InstanceConfigView struct {
	Params map[string]string `json:"params"`
	Meta   []ParamMeta       `json:"meta"`
}

// RedisDB describes one logical Redis database (0-15) that holds data.
type RedisDB struct {
	Index int   `json:"index"`
	Size  int64 `json:"size"` // DBSIZE: number of keys
}

// RestoreStatus is the in-memory state of one restore task (GET /backups/:bid/restore-status).
// Restore is a pure in-memory operation — it never touches the backup row's status.
type RestoreStatus struct {
	Status    string `json:"status"` // running, success, failed
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at"`
}

// RedisKey is one key in a logical database, with the display metadata the
// front-end key browser shows per row (type / TTL / size).
type RedisKey struct {
	Name string `json:"name"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`  // seconds; -1 = no expiry, -2 = key gone
	Size int64  `json:"size"` // bytes (MEMORY USAGE)
}

// RedisValue is a key's decoded value, shaped by its type. Value is a string
// for string keys, map[string]string for hash, []string for list/set, and
// []RedisZMember for sorted sets.
type RedisValue struct {
	Type  string      `json:"type"` // string | hash | list | set | zset
	Value interface{} `json:"value"`
}

// RedisZMember is one sorted-set entry (score kept separate from member so the
// front-end can render both).
type RedisZMember struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}
