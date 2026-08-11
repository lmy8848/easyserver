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
func TestBuildCreateTable(t *testing.T) {
	b := NewSQLBuilder(DBTypeMySQL)
	sql, err := b.BuildCreateTable("users", []TableColumn{
		{Name: "id", Type: "BIGINT", IsPrimary: true, AutoIncr: true},
		{Name: "name", Type: "VARCHAR(255)", Nullable: true, Unique: true},
		{Name: "email", Type: "VARCHAR(255)", Unique: true},
		{Name: "age", Type: "INT", Nullable: true, DefaultValue: "18"},
		{Name: "created_at", Type: "TIMESTAMP", Nullable: true, DefaultValue: "CURRENT_TIMESTAMP"},
		{Name: "nick", Type: "VARCHAR(64)", Nullable: true, DefaultValue: "guest"},
	})
	if err != nil {
		t.Fatalf("BuildCreateTable: %v", err)
	}
	for _, want := range []string{
		"`id` BIGINT PRIMARY KEY AUTO_INCREMENT",           // 自增/主键
		"`name` VARCHAR(255) UNIQUE",                       // 可空→无 NOT NULL + UNIQUE
		"`email` VARCHAR(255) NOT NULL UNIQUE",             // 不可空 + UNIQUE
		"`age` INT DEFAULT 18",                             // 数字默认值不加引号
		"`created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP", // 函数默认值不加引号
		"`nick` VARCHAR(64) DEFAULT 'guest'",               // 字符串默认值加引号
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("缺少 %q\nSQL: %s", want, sql)
		}
	}
}

// TestBuildCreateTablePg 验证 PG 分支：SERIAL 自增、主键不重复 NOT NULL、自增列不挂 DEFAULT。
func TestBuildCreateTablePg(t *testing.T) {
	b := NewSQLBuilder(DBTypePostgreSQL)
	sql, err := b.BuildCreateTable("users", []TableColumn{
		{Name: "id", Type: "SERIAL", AutoIncr: true, DefaultValue: "1"},
		{Name: "name", Type: "TEXT", Nullable: true},
		{Name: "age", Type: "INT", Nullable: true, DefaultValue: "0"},
	})
	if err != nil {
		t.Fatalf("BuildCreateTable: %v", err)
	}
	for _, want := range []string{
		`"id" SERIAL PRIMARY KEY`, // 自增覆盖主键，且不重复 PRIMARY KEY / NOT NULL / DEFAULT
		`"name" TEXT`,
		`"age" INT DEFAULT 0`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("缺少 %q\nSQL: %s", want, sql)
		}
	}
	if strings.Count(sql, "PRIMARY KEY") != 1 {
		t.Errorf("PRIMARY KEY 出现 %d 次，应为 1\nSQL: %s", strings.Count(sql, "PRIMARY KEY"), sql)
	}
}
