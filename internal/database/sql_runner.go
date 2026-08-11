package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
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
// time.Time, []byte, string, nil) with per-column types.
type QueryResult struct {
	Columns []ColumnMeta
	Rows    [][]any
}

// ExecResult reports what a non-query statement changed.
type ExecResult struct {
	RowsAffected int64
	LastInsertID int64
}

// SQLRunner is the seam between database logic and how SQL actually runs
// against an instance. The Service operates on this interface; driverSQLRunner
// is its sole implementation — a direct database/sql connection over the mapped
// host port (MySQL / PostgreSQL). Callers pass placeholder SQL (mysql `?`,
// postgres `$n`) plus args; the driver binds them.
type SQLRunner interface {
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

// driverSQLRunner runs SQL over a direct database connection through the
// container's mapped host port. It is the only SQL channel for
// MySQL/PostgreSQL: native result types, parameter binding and connection-pool
// reuse. Redis has no driver in scope.
type driverSQLRunner struct {
	// pools is keyed by (instance, database): PostgreSQL cannot reference
	// tables in another database, so each database the panel operates on needs
	// its own pool; the system database (postgres/mysql) hosts admin queries.
	mu    sync.Mutex
	pools map[poolKey]*sql.DB
}

type poolKey struct {
	instanceID int64
	db         string
}

func newDriverSQLRunner() *driverSQLRunner {
	return &driverSQLRunner{pools: make(map[poolKey]*sql.DB)}
}

// Close closes every pool belonging to an instance (called when the instance is
// uninstalled/destroyed).
func (r *driverSQLRunner) Close(instanceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, db := range r.pools {
		if key.instanceID == instanceID {
			db.Close()
			delete(r.pools, key)
		}
	}
}

func (r *driverSQLRunner) poolFor(ctx context.Context, inst *DBInstance, dbName string) (*sql.DB, error) {
	key := poolKey{instanceID: inst.ID, db: dbName}

	r.mu.Lock()
	if db, ok := r.pools[key]; ok {
		r.mu.Unlock()
		return db, nil
	}
	db, err := openDirectDB(inst, dbName)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	r.pools[key] = db
	r.mu.Unlock()

	// Validate the connection lazily on first use. sql.Open does not connect;
	// PingContext establishes the first connection so a dead container / wrong
	// mapping surfaces here as a clear error, not on the first query.
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("无法连接数据库实例: %w", err)
	}
	return db, nil
}

func openDirectDB(inst *DBInstance, dbName string) (*sql.DB, error) {
	// Always dial the host loopback: the panel accepts only 127.0.0.1 or 0.0.0.0
	// bindings, and both are reachable on loopback from this same host.
	addr := fmt.Sprintf("127.0.0.1:%d", inst.Port)
	var driver, dsn string
	switch inst.DBType {
	case DBTypeMySQL:
		driver = "mysql"
		// allowPublicKeyRetrieval 是 JDBC 参数，go-sql-driver/mysql 不识别——会被
		// 驱动当服务器变量 SET，MySQL 8.0 报 Unknown system variable。go 驱动原生
		// 支持 caching_sha2_password，不需要它。
		dsn = fmt.Sprintf("root:%s@tcp(%s)/%s?multiStatements=true&parseTime=true&loc=Local&charset=utf8mb4&timeout=5s",
			inst.AdminPassword, addr, dbName)
	case DBTypePostgreSQL:
		driver = "pgx"
		dsn = fmt.Sprintf("postgres://postgres:%s@%s/%s?sslmode=disable", inst.AdminPassword, addr, dbName)
	default:
		return nil, fmt.Errorf("engine %q has no direct driver", inst.DBType)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s connection: %w", inst.DBType, err)
	}
	// Idle pools for stopped/unused databases must not hold connections forever.
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetMaxIdleConns(2)
	db.SetMaxOpenConns(10)
	return db, nil
}

func (r *driverSQLRunner) Query(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*QueryResult, error) {
	db, err := r.poolFor(ctx, inst, dbName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("read column types: %w", err)
	}
	cols := make([]ColumnMeta, len(colTypes))
	for i, ct := range colTypes {
		cols[i] = ColumnMeta{Name: ct.Name(), Type: classifyColumnType(inst.DBType, ct.DatabaseTypeName())}
	}

	var result [][]any
	for rows.Next() {
		vals := make([]any, len(colTypes))
		targets := make([]any, len(colTypes))
		for i := range targets {
			targets[i] = &vals[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		// Text columns come back as []byte (driver scans VARCHAR/TEXT into bytes).
		// The HTTP layer JSON-encodes results, and encoding/json turns []byte into
		// base64 — a VARCHAR "kairo" would read back as "a2Fpcm8=". Convert every
		// non-blob cell to string so JSON carries the literal text. Numeric DECIMAL
		// cells keep their existing []byte→string conversion too (already covered
		// here since the category is "number").
		for i, v := range vals {
			if b, ok := v.([]byte); ok && cols[i].Type != "blob" {
				vals[i] = string(b)
			}
		}
		result = append(result, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &QueryResult{Columns: cols, Rows: result}, nil
}

func (r *driverSQLRunner) Exec(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*ExecResult, error) {
	db, err := r.poolFor(ctx, inst, dbName)
	if err != nil {
		return nil, err
	}
	result, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	execResult := &ExecResult{}
	if ra, err := result.RowsAffected(); err == nil {
		execResult.RowsAffected = ra
	}
	if li, err := result.LastInsertId(); err == nil {
		execResult.LastInsertID = li
	}
	return execResult, nil
}
