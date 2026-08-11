package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// driverQueryRunner runs SQL over a direct database connection through the
// container's mapped host port. It is the only SQL channel for
// MySQL/PostgreSQL: native result types, parameter binding and connection-pool
// reuse. Redis has no driver in scope; instances with a broken port mapping
// (container_port = 0) surface a clear error instead of falling back.
type driverQueryRunner struct {
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

func newDriverQueryRunner() *driverQueryRunner {
	return &driverQueryRunner{pools: make(map[poolKey]*sql.DB)}
}

// Close closes every pool belonging to an instance (called when the instance is
// uninstalled/destroyed).
func (r *driverQueryRunner) Close(instanceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, db := range r.pools {
		if key.instanceID == instanceID {
			db.Close()
			delete(r.pools, key)
		}
	}
}

func (r *driverQueryRunner) poolFor(ctx context.Context, inst *DBInstance, dbName string) (*sql.DB, error) {
	if inst.ContainerPort <= 0 {
		return nil, fmt.Errorf("实例端口映射异常（container_port=%d），无法直连，请改端口重建", inst.ContainerPort)
	}
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
		dsn = fmt.Sprintf("root:%s@tcp(%s)/%s?multiStatements=true&allowPublicKeyRetrieval=true&parseTime=true&loc=Local&charset=utf8mb4&timeout=5s",
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

func (r *driverQueryRunner) Query(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*QueryResult, error) {
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
		// Numeric columns may come back as []byte (e.g. DECIMAL) — turn them into
		// strings so JSON carries "123.45", not a base64 blob, while the column
		// category still says "number" for right-aligned rendering.
		for i, v := range vals {
			if b, ok := v.([]byte); ok && cols[i].Type == "number" {
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

func (r *driverQueryRunner) Exec(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*ExecResult, error) {
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
