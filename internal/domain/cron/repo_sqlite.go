package cron

import (
	"context"
	"database/sql"

	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// sqliteRepo implements Repository for SQLite.
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed cron Repository.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

func (r *sqliteRepo) ListScripts(ctx context.Context) ([]Script, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, created_at, updated_at
		 FROM scripts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []Script
	for rows.Next() {
		var sc Script
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan script: %w", err)
		}
		sc.Path = scriptFilePath(sc.ID)
		scripts = append(scripts, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scripts: %w", err)
	}
	return scripts, nil
}

func (r *sqliteRepo) GetScript(ctx context.Context, id int64) (*Script, error) {
	var sc Script
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, created_at, updated_at
		 FROM scripts WHERE id = ?`, id,
	).Scan(&sc.ID, &sc.Name, &sc.Description, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sc.Path = scriptFilePath(id)
	return &sc, nil
}

func (r *sqliteRepo) CreateScript(ctx context.Context, script *Script) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO scripts (name, description) VALUES (?, ?)`,
		script.Name, script.Description,
	)
	if err != nil {
		return err
	}
	script.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	return nil
}

func (r *sqliteRepo) UpdateScript(ctx context.Context, script *Script) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE scripts SET name=?, description=?, updated_at=datetime('now') WHERE id=?`,
		script.Name, script.Description, script.ID)
	return err
}

func (r *sqliteRepo) DeleteScript(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM scripts WHERE id = ?", id)
	return err
}

// ReadScriptFile 读脚本落盘文件内容。文件不存在返回空串。
func (r *sqliteRepo) ReadScriptFile(id int64) (string, error) {
	data, err := os.ReadFile(scriptFilePath(id))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteScriptFile 原子写脚本落盘文件。
func (r *sqliteRepo) WriteScriptFile(id int64, content string) error {
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("创建脚本目录失败: %w", err)
	}
	path := scriptFilePath(id)
	tmpFile, err := os.CreateTemp(scriptsDir, "script-"+strconv.FormatInt(id, 10)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// DeleteScriptFile 删除脚本落盘文件。文件不存在视为成功。
func (r *sqliteRepo) DeleteScriptFile(id int64) error {
	err := os.Remove(scriptFilePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// scriptFilePath 返回脚本落盘路径。
func scriptFilePath(id int64) string {
	return filepath.Join(scriptsDir, strconv.FormatInt(id, 10))
}

// ScriptPath 导出脚本落盘路径，供 handler 判断任务是否引用某脚本。
func ScriptPath(id int64) string { return scriptFilePath(id) }
