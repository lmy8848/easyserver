package database

import (
	"strings"
	"testing"
)

func TestIsTruthy(t *testing.T) {
	cases := []struct {
		row  []any
		i    int
		want bool
	}{
		{[]any{"no"}, 0, false},
		{[]any{"OFF"}, 0, false},
		{[]any{"0"}, 0, false},
		{[]any{""}, 0, false},
		{[]any{"yes"}, 0, true},
		{[]any{"YES"}, 0, true},
		{[]any{"pri"}, 0, true},
		{[]any{[]byte("no")}, 0, false},
		{[]any{[]byte("yes")}, 0, true},
		{[]any{[]byte("pri")}, 0, true},
		{[]any{false}, 0, false},
		{[]any{true}, 0, true},
		{[]any{nil}, 0, false},
		{[]any{"x"}, 99, false}, // 越界
	}
	for _, c := range cases {
		if got := isTruthy(c.row, c.i); got != c.want {
			t.Fatalf("isTruthy(%v, %d) = %v, want %v", c.row, c.i, got, c.want)
		}
	}
}

// TestBuildCreateTable 覆盖建表选项：可空/唯一/默认值/自增与主键的组合，主键单行、
// 非整数类型不挂自增。
// TestBuildInsertEmptyData 验证空数据也能插入：全列可空且都不填（或只剩自增列）时，
// 生成只写默认值的 INSERT，而不是报 "no data to insert"。
func TestBuildInsertEmptyData(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		b := NewSQLBuilder(DBTypeMySQL)
		sql, err := b.BuildInsert("users", map[string]interface{}{}, nil)
		if err != nil {
			t.Fatalf("BuildInsert: %v", err)
		}
		if want := "INSERT INTO `users` () VALUES ();"; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		b := NewSQLBuilder(DBTypePostgreSQL)
		sql, args, err := b.BuildInsertParams("users", map[string]interface{}{}, nil)
		if err != nil {
			t.Fatalf("BuildInsertParams: %v", err)
		}
		if want := `INSERT INTO "users" DEFAULT VALUES;`; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
		if len(args) != 0 {
			t.Errorf("expected no args, got %v", args)
		}
	})
	// 只剩自增列（值为空）也应落到 defaults-insert。
	t.Run("autoincr-only-skipped", func(t *testing.T) {
		b := NewSQLBuilder(DBTypeMySQL)
		sql, err := b.BuildInsert("users", map[string]interface{}{"id": ""},
			&TableInfo{Columns: []ColumnInfo{{Name: "id", IsAutoIncr: true}}})
		if err != nil {
			t.Fatalf("BuildInsert: %v", err)
		}
		if want := "INSERT INTO `users` () VALUES ();"; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
	})
}

func TestBuildCreateTable(t *testing.T) {
	b := NewSQLBuilder(DBTypeMySQL)
	sql, err := b.BuildCreateTable("users", []TableColumn{
		{Name: "id", Type: "BIGINT", IsPrimary: true, AutoIncr: true},
		{Name: "name", Type: "VARCHAR(255)", Nullable: true, Unique: true},
		{Name: "email", Type: "VARCHAR(255)", Unique: true},
		{Name: "age", Type: "INT", Nullable: true, DefaultValue: "18"},
		{Name: "created_at", Type: "TIMESTAMP", Nullable: true, DefaultValue: "CURRENT_TIMESTAMP"},
		{Name: "nick", Type: "VARCHAR(64)", Nullable: true, DefaultValue: "guest"},
	}, "utf8mb4", "utf8mb4_unicode_ci")
	if err != nil {
		t.Fatalf("BuildCreateTable: %v", err)
	}
	for _, want := range []string{
		"`id` BIGINT PRIMARY KEY AUTO_INCREMENT",             // 自增/主键
		"`name` VARCHAR(255) UNIQUE",                         // 可空→无 NOT NULL + UNIQUE
		"`email` VARCHAR(255) NOT NULL UNIQUE",               // 不可空 + UNIQUE
		"`age` INT DEFAULT 18",                               // 数字默认值不加引号
		"`created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP",   // 函数默认值不加引号
		"`nick` VARCHAR(64) DEFAULT 'guest'",                 // 字符串默认值加引号
		"DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", // 字符集/排序规则
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("缺少 %q\nSQL: %s", want, sql)
		}
	}
}

// TestBuildCreateTablePg 验证 PG 分支：SERIAL 自增、主键不重复 NOT NULL、自增列不挂 DEFAULT、
// 排序规则只挂到字符串列。
func TestBuildCreateTablePg(t *testing.T) {
	b := NewSQLBuilder(DBTypePostgreSQL)
	sql, err := b.BuildCreateTable("users", []TableColumn{
		{Name: "id", Type: "SERIAL", AutoIncr: true, DefaultValue: "1"},
		{Name: "name", Type: "TEXT", Nullable: true},
		{Name: "age", Type: "INT", Nullable: true, DefaultValue: "0"},
	}, "", "C.UTF-8")
	if err != nil {
		t.Fatalf("BuildCreateTable: %v", err)
	}
	for _, want := range []string{
		`"id" SERIAL PRIMARY KEY`,       // 自增覆盖主键，且不重复 PRIMARY KEY / NOT NULL / DEFAULT
		`"name" TEXT COLLATE "C.UTF-8"`, // 字符串列挂排序规则
		`"age" INT DEFAULT 0`,           // 数字列不挂 COLLATE
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("缺少 %q\nSQL: %s", want, sql)
		}
	}
	if strings.Contains(sql, "CHARSET") {
		t.Errorf("PG 不应有 CHARSET 子句\nSQL: %s", sql)
	}
	if strings.Count(sql, "PRIMARY KEY") != 1 {
		t.Errorf("PRIMARY KEY 出现 %d 次，应为 1\nSQL: %s", strings.Count(sql, "PRIMARY KEY"), sql)
	}
}

func TestBuildCreateTableWithLength(t *testing.T) {
	b := NewSQLBuilder(DBTypeMySQL)
	sql, err := b.BuildCreateTable("products", []TableColumn{
		{Name: "id", Type: "BIGINT", IsPrimary: true, AutoIncr: true},
		{Name: "title", Type: "VARCHAR", Length: "128", Nullable: false},
		{Name: "code", Type: "CHAR", Length: "32", Nullable: false},
		{Name: "price", Type: "DECIMAL", Length: "10, 2", Nullable: false},
	}, "utf8mb4", "")
	if err != nil {
		t.Fatalf("BuildCreateTable failed: %v", err)
	}

	for _, want := range []string{
		"`title` VARCHAR(128) NOT NULL",
		"`code` CHAR(32) NOT NULL",
		"`price` DECIMAL(10, 2) NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("缺少 %q\nSQL: %s", want, sql)
		}
	}

	// 校验非法长度注入
	_, err = b.BuildCreateTable("test", []TableColumn{
		{Name: "col", Type: "VARCHAR", Length: "255); DROP TABLE products;--"},
	}, "utf8mb4", "")
	if err == nil {
		t.Errorf("应拒绝注入格式的 Length 参数")
	}
}

func TestBuildDescribeTable(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		b := NewSQLBuilder(DBTypeMySQL)
		sql, err := b.BuildDescribeTable("test")
		if err != nil {
			t.Fatalf("BuildDescribeTable failed: %v", err)
		}
		if want := "DESCRIBE `test`;"; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		b := NewSQLBuilder(DBTypePostgreSQL)
		sql, err := b.BuildDescribeTable("test")
		if err != nil {
			t.Fatalf("BuildDescribeTable failed: %v", err)
		}
		if !strings.Contains(sql, "$1::regclass") || !strings.Contains(sql, "WHERE table_name = $1") {
			t.Errorf("BuildDescribeTable for PG must bind the table name via $1: %s", sql)
		}
		if strings.Contains(sql, "'test'") || strings.Contains(sql, "\"test\"") {
			t.Errorf("BuildDescribeTable for PG must not inline the table name: %s", sql)
		}
	})
}

func TestBuildResetPassword(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		b := NewSQLBuilder(DBTypeMySQL)
		sql, err := b.BuildResetPassword("user1", "Pass123456", "%")
		if err != nil {
			t.Fatalf("BuildResetPassword failed: %v", err)
		}
		if want := "ALTER USER 'user1'@'%' IDENTIFIED BY 'Pass123456';"; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
	})
	t.Run("postgres", func(t *testing.T) {
		b := NewSQLBuilder(DBTypePostgreSQL)
		sql, err := b.BuildResetPassword("user1", "Pass123456", "")
		if err != nil {
			t.Fatalf("BuildResetPassword failed: %v", err)
		}
		if want := `ALTER USER "user1" WITH PASSWORD 'Pass123456';`; sql != want {
			t.Errorf("got %q, want %q", sql, want)
		}
	})
}
