package database

import (
	"context"
	"strings"
	"time"
)

// ColumnMeta describes one result column: its name plus a normalized type
// category. The category (not the raw engine type string) is what the front-end
// switches on to render NULL / BLOB / timestamps / numbers correctly instead of
// treating every cell as text — that is the whole point of the direct-connection
// channel. The CLI fallback channel cannot recover types from text output, so it
// reports everything as "string".
type ColumnMeta struct {
	Name string `json:"name"`
	Type string `json:"type"` // number | string | time | blob | boolean | null
}

// QueryResult is a structured query result — native Go values (int64, float64,
// time.Time, []byte, string, nil) with per-column types, as opposed to the CLI
// channel's tab/pipe-split text where everything is a string.
type QueryResult struct {
	Columns []ColumnMeta
	Rows    [][]any
}

// ExecResult reports what a non-query statement changed.
type ExecResult struct {
	RowsAffected int64
	LastInsertID int64
}

// SQLQueryRunner is the seam between database logic and how SQL actually runs
// against an instance. Two implementations:
//
//   - driverQueryRunner: direct database/sql connection over the mapped host
//     port (MySQL / PostgreSQL). Structured types, parameter binding, pool reuse.
//   - cliQueryRunner: docker/podman exec of the engine CLI + text parsing. Used
//     for Redis (no driver in scope) and for MySQL/PostgreSQL instances whose
//     host port mapping is broken (container_port = 0), which direct connection
//     cannot reach.
//
// The Service picks per instance via runnerFor (see service.go). Callers pass
// placeholder SQL (mysql `?`, postgres `$n`) plus args; the driver binds them,
// the CLI runner interpolates them into the exec string.
type SQLQueryRunner interface {
	Query(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*QueryResult, error)
	Exec(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*ExecResult, error)
	Close(instanceID int64)
}

// systemDBName is the database a connection binds to for instance-level
// operations (list databases/users) that don't target a specific database. The
// engine's system database always exists.
func systemDBName(dbType DBType) string {
	switch dbType {
	case DBTypePostgreSQL:
		return "postgres"
	default:
		return "mysql"
	}
}

// columnTypeCategories normalizes a driver column type name into one of the
// front-end render categories.
func classifyColumnType(dbType DBType, rawType string) string {
	// Driver type names vary by engine; match on substring so version-specific
	// variants (tinyint(1), int2, serial4, ...) all land in the right bucket.
	rt := strings.ToLower(rawType)
	if isNumberType(dbType, rt) {
		return "number"
	}
	if strings.Contains(rt, "blob") || rt == "bytea" || strings.Contains(rt, "binary") {
		return "blob"
	}
	if strings.Contains(rt, "date") || strings.Contains(rt, "time") || strings.Contains(rt, "year") {
		return "time"
	}
	if rt == "bool" || rt == "boolean" || rt == "bit" {
		return "boolean"
	}
	if rt == "" {
		return "string"
	}
	return "string"
}

func isNumberType(dbType DBType, rt string) bool {
	switch dbType {
	case DBTypeMySQL:
		return strings.Contains(rt, "int") || strings.Contains(rt, "decimal") ||
			strings.Contains(rt, "float") || strings.Contains(rt, "double") ||
			rt == "numeric" || rt == "real"
	default:
		return strings.HasPrefix(rt, "int") || strings.HasPrefix(rt, "float") ||
			strings.HasPrefix(rt, "numeric") || strings.HasPrefix(rt, "serial") ||
			strings.HasPrefix(rt, "real") || strings.HasPrefix(rt, "money")
	}
}

// classifyValue returns the render category for a driver-scanned value, used as
// a fallback when the column type is not reported.
func classifyValue(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case int64, int32, float64, float32, int, uint64:
		return "number"
	case time.Time:
		return "time"
	case []byte:
		return "blob"
	}
	return "string"
}
