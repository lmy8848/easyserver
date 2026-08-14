package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"easyserver/internal/infra/apperror"

	_ "modernc.org/sqlite"
)

func TestWrap_UniqueConstraintMappedToConflict(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE)`); err != nil {
		t.Fatal(err)
	}

	wrapped := Wrap(db)

	// 首次插入成功
	if _, err := wrapped.ExecContext(ctx, "INSERT INTO users (email) VALUES (?)", "a@b.com"); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}

	// 重复插入：驱动返回 UNIQUE constraint failed → 映射为 ErrConflict
	_, err = wrapped.ExecContext(ctx, "INSERT INTO users (email) VALUES (?)", "a@b.com")
	if err == nil {
		t.Fatal("expected duplicate insert to fail")
	}
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("expected ErrConflict, got: %v", err)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got: %T", err)
	}
	if appErr.HTTPStatus != 409 {
		t.Fatalf("expected HTTP 409, got %d", appErr.HTTPStatus)
	}
	// 原始驱动错误留在链上
	if appErr.Unwrap() == nil {
		t.Fatalf("expected underlying error preserved, got nil")
	}
}

func TestWrap_NonUniqueErrorPassedThrough(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	wrapped := Wrap(db)

	// 表不存在 → 驱动错误原样透传（不是 UNIQUE，不映射）
	_, err = wrapped.ExecContext(ctx, "INSERT INTO no_such_table (x) VALUES (1)")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("non-unique error should not map to ErrConflict: %v", err)
	}
}

func TestIsUniqueConstraint(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"UNIQUE constraint failed: users.email", true},
		{"constraint failed", true},
		{"UNIQUE constraint failed: users.email: foreign key mismatch", true},
		{"no such table: users", false},
		{"database is locked", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsUniqueConstraint(errors.New(c.msg)); got != c.want {
			t.Errorf("IsUniqueConstraint(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}
