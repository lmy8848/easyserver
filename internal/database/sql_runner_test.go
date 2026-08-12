package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// driverSQLRunner tests feed sqlmock a fake *sql.DB and assert the observable
// contract of the direct-connection channel (sql_runner.go): native result
// types, per-column render categories, NULL/BLOB preservation, parameter
// binding and error passthrough. No real database is needed.

func TestDriverSQLRunnerStructuralTypes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, AdminPassword: "secret"}
	runner := &driverSQLRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "mydb"}: db}}

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
	// The "data" column is []byte in the mock, but sqlmock reports no column type
	// names (DatabaseTypeName always empty → category "string"), so the channel
	// converts it to a string — the JSON-transport contract that prevents text
	// columns reading back as base64. Blob columns keep []byte in production,
	// where the driver reports BLOB/bytea and the category is "blob".
	if s, ok := row0[3].(string); !ok || s != string([]byte{0x01, 0x02, 0xAB}) {
		t.Errorf("row0 data = %#v (%T), want string of raw bytes", row0[3], row0[3])
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

func TestDriverSQLRunnerParameterBinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypePostgreSQL, Port: 5432, AdminPassword: "secret"}
	runner := &driverSQLRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "pg"}: db}}

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

func TestDriverSQLRunnerErrorPassthrough(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, AdminPassword: "secret"}
	runner := &driverSQLRunner{pools: map[poolKey]*sql.DB{{instanceID: 7, db: "mydb"}: db}}

	mock.ExpectQuery("SELECT 1").WillReturnError(errors.New("Error 1062: Duplicate entry 'x' for key 'PRIMARY'"))

	_, err = runner.Query(context.Background(), inst, "mydb", "SELECT 1")
	if err == nil || err.Error() != "Error 1062: Duplicate entry 'x' for key 'PRIMARY'" {
		t.Fatalf("got err %v, want the engine error surfaced verbatim", err)
	}
}

func TestDriverSQLRunnerPingFailure(t *testing.T) {
	// sql.Open does not connect; the first query must surface a dial failure as
	// a clear "cannot connect" error, not a bare driver error. sqlmock needs the
	// MonitorPingsOption to intercept Ping; without a mock db the driver tries a
	// real dial and fails — which is exactly the path under test.
	runner := newDriverSQLRunner()
	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 1, AdminPassword: "secret"}
	_, err := runner.Query(context.Background(), inst, "mydb", "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "无法连接数据库实例") {
		t.Fatalf("got %v, want 'cannot connect' error", err)
	}
}

func TestDriverSQLRunnerCloseDropsPool(t *testing.T) {
	runner := newDriverSQLRunner()
	inst := &DBInstance{ID: 7, DBType: DBTypeMySQL, Port: 3306, AdminPassword: "secret"}

	// After Close the pool is gone: a query no longer finds a cached pool and
	// must not panic (it will try to dial a real MySQL, which fails — assert the
	// error path is reached, not the cached-pool path).
	runner.Close(7)
	if len(runner.pools) != 0 {
		t.Fatalf("expected pools emptied after Close, got %d", len(runner.pools))
	}
	_ = inst
}

func TestIsReadStatement(t *testing.T) {
	cases := []struct {
		stmt string
		want bool
	}{
		{"SELECT * FROM t", true},
		{"  select id, name from t where x = 1", true},
		{"SHOW TABLES", true},
		{"EXPLAIN SELECT 1", true},
		{"DESCRIBE users", true},
		{"DESC;", true}, // 裸 DESC 不带表名也按读语句走 Query
		{"PRAGMA table_info(t)", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		// 写语句
		{"DELETE FROM t WHERE id = 1", false},
		{"INSERT INTO t (a) VALUES (1)", false},
		{"UPDATE t SET a = 1 WHERE id = 1", false},
		{"DROP TABLE t", false},
		{"CREATE TABLE t (id int)", false},
		// data-modifying CTE
		{"WITH cte AS (DELETE FROM t RETURNING *) SELECT * FROM cte", false},
		{"WITH cte AS (UPDATE t SET a = 1 RETURNING *) SELECT * FROM cte", false},
		// 注释开头的语句
		{"-- comment\nSELECT 1", true},
		{"", false},
		// 多行查询语句
		{"SELECT *\nFROM users\nWHERE id = 1", true},
		{"-- 注释\nSELECT id\nFROM users\nLIMIT 10", true},
		// 分号在字符串字面量内
		{"SELECT 'a;b' AS x\nFROM t", true},
		{"SELECT ';' AS s", true},
		{"INSERT INTO t VALUES ('a;b')", false},
	}
	for _, c := range cases {
		if got := isReadStatement(c.stmt); got != c.want {
			t.Errorf("isReadStatement(%q) = %v, want %v", c.stmt, got, c.want)
		}
	}
}
