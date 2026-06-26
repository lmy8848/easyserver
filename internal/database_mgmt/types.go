package database_mgmt

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
