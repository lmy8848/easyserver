package cron

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// sqliteRepo implements Repository for SQLite.
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed cron Repository.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// GetRuntimeVersionStatus reads the status of a runtime_version row.
func (r *sqliteRepo) GetRuntimeVersionStatus(ctx context.Context, runtimeVersionID int64) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		"SELECT status FROM runtime_version WHERE id = ?", runtimeVersionID).Scan(&status)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("runtime_version %d not found", runtimeVersionID)
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

// GetRuntime 返回 runtime_version 行的 lang/exact/status。
func (r *sqliteRepo) GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error) {
	err = r.db.QueryRowContext(ctx,
		"SELECT lang, exact, status FROM runtime_version WHERE id = ?", id).
		Scan(&lang, &exact, &status)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("runtime_version %d not found", id)
	}
	if err != nil {
		return "", "", "", err
	}
	return lang, exact, status, nil
}

func (r *sqliteRepo) ListScripts(ctx context.Context) ([]Script, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, language, created_at, updated_at
		 FROM scripts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []Script
	for rows.Next() {
		var sc Script
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Language, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan script: %w", err)
		}
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
		`SELECT id, name, description, language, created_at, updated_at
		 FROM scripts WHERE id = ?`, id,
	).Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Language, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (r *sqliteRepo) CreateScript(ctx context.Context, script *Script) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO scripts (name, description, language) VALUES (?, ?, ?)`,
		script.Name, script.Description, script.Language,
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
		`UPDATE scripts SET name=?, description=?, language=?, updated_at=datetime('now') WHERE id=?`,
		script.Name, script.Description, script.Language, script.ID)
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
	tmpFile, err := os.CreateTemp(scriptsDir, "script-"+fmt.Sprint(id)+".*.tmp")
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

func (r *sqliteRepo) ListDocs(ctx context.Context) ([]CronDoc, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []CronDoc
	for rows.Next() {
		var d CronDoc
		if err := rows.Scan(&d.ID, &d.Title, &d.Content, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cron doc: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron docs: %w", err)
	}
	return docs, nil
}

func (r *sqliteRepo) GetDoc(ctx context.Context, id int64) (*CronDoc, error) {
	var d CronDoc
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs WHERE id = ?", id,
	).Scan(&d.ID, &d.Title, &d.Content, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *sqliteRepo) CreateDoc(ctx context.Context, doc *CronDoc) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_docs (title, content, sort_order) VALUES (?, ?, ?)`,
		doc.Title, doc.Content, doc.SortOrder)
	if err != nil {
		return err
	}
	doc.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	return nil
}

func (r *sqliteRepo) UpdateDoc(ctx context.Context, doc *CronDoc) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_docs SET title=?, content=?, sort_order=?, updated_at=datetime('now') WHERE id=?`,
		doc.Title, doc.Content, doc.SortOrder, doc.ID)
	return err
}

func (r *sqliteRepo) DeleteDoc(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM cron_docs WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) CountDocs(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cron_docs").Scan(&count); err != nil {
		return 0, fmt.Errorf("count cron docs: %w", err)
	}
	return count, nil
}

func (r *sqliteRepo) BatchCreateDocs(ctx context.Context, docs []CronDoc) error {
	for _, doc := range docs {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO cron_docs (title, content, sort_order) VALUES (?, ?, ?)`,
			doc.Title, doc.Content, doc.SortOrder)
		if err != nil {
			return err
		}
	}
	return nil
}

// scriptFilePath 返回脚本落盘路径。
func scriptFilePath(id int64) string {
	return filepath.Join(scriptsDir, fmt.Sprint(id))
}

// scriptsDir 是脚本落盘目录（脚本内容存储）。
const scriptsDir = "/opt/easyserver/scripts"
