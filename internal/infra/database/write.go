package database

import (
	"context"
	"database/sql"
	"strings"

	"easyserver/internal/infra/apperror"
)

// DB 是 repo 层使用的数据库访问接口（*sql.DB 的方法子集）。
// repo 字段用它而非 *sql.DB，以便通过 writeDB 在写路径统一处理
// SQLite 驱动级错误（UNIQUE 约束冲突 → ErrConflict），无需每个
// repo 方法自行判断。
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	Close() error
}

// writeDB 包装 *sql.DB：拦截写操作错误，把 SQLite 的 UNIQUE 约束
// 冲突归类为 ErrConflict（语义：资源冲突，409），其余错误原样透传。
type writeDB struct {
	*sql.DB
}

// Wrap 包装一个 *sql.DB 供 repo 使用（Init 的产物直接传给各
// NewSQLiteRepository 即可，签名不变）。
func Wrap(db *sql.DB) DB {
	return &writeDB{DB: db}
}

// ExecContext 检测 UNIQUE constraint failed（SQLite 驱动固定文本，
// 无代码字面量产生点，故在统一写路径处理）。
func (d *writeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := d.DB.ExecContext(ctx, query, args...)
	if err != nil && isUniqueConstraint(err) {
		return res, apperror.ErrConflict.WrapMessage(err)
	}
	return res, err
}

// IsUniqueConstraint reports whether err is a SQLite UNIQUE constraint violation.
// 事务内的写（tx.Exec）不经过 writeDB，repo 可用它自行判断。
func IsUniqueConstraint(err error) bool { return isUniqueConstraint(err) }

func isUniqueConstraint(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed")
}
