package plugin

import (
	"context"
	"database/sql"

	"easyserver/internal/infra/errx"
)

// sourceRepo 实现 SourceRepository（SQLite）。
type sourceRepo struct {
	db *sql.DB
}

// NewSourceRepository 创建插件源仓储。
func NewSourceRepository(db *sql.DB) SourceRepository {
	return &sourceRepo{db: db}
}

func (r *sourceRepo) ListSources(ctx context.Context) ([]PluginSource, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, url, enabled, created_at, updated_at
		FROM plugin_sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []PluginSource{}
	for rows.Next() {
		var s PluginSource
		var enabled int
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sourceRepo) AddSource(ctx context.Context, name, url string) (*PluginSource, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO plugin_sources (name, url) VALUES (?, ?)`, name, url)
	if err != nil {
		return nil, errx.Conflict("添加插件源失败（可能同名已存在）: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	// 回读完整记录（含 DB 生成的时间戳），避免返回零值时间
	var s PluginSource
	var enabled int
	err = r.db.QueryRowContext(ctx, `
		SELECT id, name, url, enabled, created_at, updated_at
		FROM plugin_sources WHERE id = ?`, id).
		Scan(&s.ID, &s.Name, &s.URL, &enabled, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled == 1
	return &s, nil
}

func (r *sourceRepo) DeleteSource(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM plugin_sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errx.NotFound("插件源不存在")
	}
	return nil
}

func (r *sourceRepo) SetSourceEnabled(ctx context.Context, id int64, enabled bool) error {
	res, err := r.db.ExecContext(ctx, `UPDATE plugin_sources SET enabled = ? WHERE id = ?`, boolInt(enabled), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errx.NotFound("插件源不存在")
	}
	return nil
}

// 确保编译期类型断言。
var _ SourceRepository = (*sourceRepo)(nil)
