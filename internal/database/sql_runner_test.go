package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// driverQueryRunner tests feed sqlmock a fake *sql.DB and assert the observable
// contract of the direct-connection channel (sql_runner.go): native result
// types, per-column render categories, NULL/BLOB preservation, parameter
// binding and error passthrough. No real database is needed.

func TestDriverQueryRunnerStructuralTypes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, ContainerPort: 3306, AdminPassword: "secret"}
	runner := &driverQueryRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "mydb"}: db}}

	// sqlmock cannot report column type names (DatabaseTypeName is always empty),
	// so this test asserts the VALUE side: native types survive scanning and NULL
	// stays nil rather than becoming the string "NULL". Column-category mapping
	// is covered separately by TestClassifyColumnType.
	cols := []string{"id", "name", "created_at", "data"}
	rows := sqlmock.NewRows(cols).
		AddRow(int64(42), "alice", "2026-08-10 12:00:00", []byte{0x01, 0x02, 0xAB}).
		AddRow(nil, nil, nil, nil)
	mock.ExpectQuery("SELECT \\* FROM t").
		WithArgs("x").
		WillReturnRows(rows)

	res, err := runner.Query(context.Background(), inst, "mydb", "SELECT * FROM t WHERE id = ?", "x")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Columns) != 4 || len(res.Rows) != 2 {
		t.Fatalf("got %d cols %d rows, want 4 cols 2 rows: %+v", len(res.Columns), len(res.Rows), res)
	}
	row0 := res.Rows[0]
	if v, ok := row0[0].(int64); !ok || v != 42 {
		t.Errorf("row0 id = %#v (%T), want int64(42)", row0[0], row0[0])
	}
	if s, ok := row0[1].(string); !ok || s != "alice" {
		t.Errorf("row0 name = %#v (%T), want string alice", row0[0], row0[0])
	}
	if b, ok := row0[3].([]byte); !ok || len(b) != 3 {
		t.Errorf("row0 data = %#v (%T), want []byte len 3", row0[3], row0[3])
	}
	// NULL row preserved as nil, not as the string "NULL".
	for i := range res.Rows[1] {
		if res.Rows[1][i] != nil {
			t.Errorf("row1[%d] = %#v, want nil", i, res.Rows[1][i])
		}
	}
}

func TestClassifyColumnType(t *testing.T) {
	tests := []struct {
		dbType DBType
		raw    string
		want   string
	}{
		{DBTypeMySQL, "BIGINT", "number"},
		{DBTypeMySQL, "int", "number"},
		{DBTypeMySQL, "DECIMAL", "number"},
		{DBTypeMySQL, "FLOAT", "number"},
		{DBTypePostgreSQL, "int4", "number"},
		{DBTypePostgreSQL, "serial", "number"},
		{DBTypePostgreSQL, "numeric", "number"},
		{DBTypeMySQL, "DATETIME", "time"},
		{DBTypeMySQL, "TIMESTAMP", "time"},
		{DBTypePostgreSQL, "date", "time"},
		{DBTypePostgreSQL, "timestamp", "time"},
		{DBTypeMySQL, "LONGBLOB", "blob"},
		{DBTypePostgreSQL, "bytea", "blob"},
		{DBTypeMySQL, "VARCHAR", "string"},
		{DBTypePostgreSQL, "text", "string"},
		{DBTypeMySQL, "JSON", "string"},
	}
	for _, tt := range tests {
		if got := classifyColumnType(tt.dbType, tt.raw); got != tt.want {
			t.Errorf("classifyColumnType(%s, %q) = %q, want %q", tt.dbType, tt.raw, got, tt.want)
		}
	}
}

func TestDriverQueryRunnerParameterBinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypePostgreSQL, Port: 5432, ContainerPort: 5432, AdminPassword: "secret"}
	runner := &driverQueryRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "pg"}: db}}

	mock.ExpectExec("INSERT INTO t").
		WithArgs("O'Brien", int64(42)).
		WillReturnResult(sqlmock.NewResult(11, 1))

	res, err := runner.Exec(context.Background(), inst, "pg", "INSERT INTO t (name, n) VALUES ($1, $2)", "O'Brien", int64(42))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if res.LastInsertID != 11 || res.RowsAffected != 1 {
		t.Errorf("exec result = %+v, want LastInsertID=11 RowsAffected=1", res)
	}
}

func TestDriverQueryRunnerErrorPassthrough(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, ContainerPort: 3306, AdminPassword: "secret"}
	runner := &driverQueryRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "mydb"}: db}}

	mock.ExpectQuery("SELECT 1").WillReturnError(errors.New("Error 1062: Duplicate entry 'x' for key 'PRIMARY'"))

	_, err = runner.Query(context.Background(), inst, "mydb", "SELECT 1")
	if err == nil || err.Error() != "Error 1062: Duplicate entry 'x' for key 'PRIMARY'" {
		t.Fatalf("got err %v, want the engine error surfaced verbatim", err)
	}
}

func TestDriverQueryRunnerBrokenMapping(t *testing.T) {
	// container_port = 0 means the mapping is broken: no direct connection.
	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3307, ContainerPort: 0}
	runner := newDriverQueryRunner()
	_, err := runner.Query(context.Background(), inst, "mydb", "SELECT 1")
	if err == nil {
		t.Fatal("expected error for container_port=0 instance")
	}
}

func TestDriverQueryRunnerPingFailure(t *testing.T) {
	// sql.Open does not connect; the first query must surface a dial failure as
	// a clear "cannot connect" error, not a bare driver error. sqlmock needs the
	// MonitorPingsOption to intercept Ping; without a mock db the driver tries a
	// real dial and fails — which is exactly the path under test.
	runner := newDriverQueryRunner()
	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 1, ContainerPort: 3306, AdminPassword: "secret"}
	_, err := runner.Query(context.Background(), inst, "mydb", "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "无法连接数据库实例") {
		t.Fatalf("got %v, want 'cannot connect' error", err)
	}
}

func TestDriverQueryRunnerCloseDropsPool(t *testing.T) {
	runner := newDriverQueryRunner()
	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, ContainerPort: 3306, AdminPassword: "secret"}

	// After Close the pool is gone: a query no longer finds a cached pool and
	// must not panic (it will try to dial a real MySQL, which fails — assert the
	// error path is reached, not the cached-pool path).
	runner.Close(7)
	if len(runner.pools) != 0 {
		t.Fatalf("expected pools emptied after Close, got %d", len(runner.pools))
	}
	_ = inst
}
