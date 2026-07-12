package fileshare

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type sqliteShareRepo struct {
	db *sql.DB
}

func NewSQLiteShareRepository(db *sql.DB) Repository {
	return &sqliteShareRepo{db: db}
}

func (r *sqliteShareRepo) Create(ctx context.Context, share *FileShare) (int64, error) {
	result, err := r.db.ExecContext(ctx, `INSERT INTO file_shares
		(file_path, file_name, file_size, token, password, expires_at, max_downloads, download_count, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		share.FilePath, share.FileName, share.FileSize, share.Token,
		share.Password, share.ExpiresAt, share.MaxDownloads, share.DownloadCount, share.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("create file share: %w", err)
	}
	return result.LastInsertId()
}

func (r *sqliteShareRepo) GetByToken(ctx context.Context, token string) (*FileShare, error) {
	s := &FileShare{}
	err := r.db.QueryRowContext(ctx, `SELECT id, file_path, file_name, file_size, token,
		COALESCE(password,''), COALESCE(expires_at,''), max_downloads, download_count,
		created_by, created_at, updated_at
		FROM file_shares WHERE token = ?`, token).Scan(
		&s.ID, &s.FilePath, &s.FileName, &s.FileSize, &s.Token,
		&s.Password, &s.ExpiresAt, &s.MaxDownloads, &s.DownloadCount,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file share by token: %w", err)
	}
	return s, nil
}

func (r *sqliteShareRepo) GetByID(ctx context.Context, id int64) (*FileShare, error) {
	s := &FileShare{}
	err := r.db.QueryRowContext(ctx, `SELECT id, file_path, file_name, file_size, token,
		COALESCE(password,''), COALESCE(expires_at,''), max_downloads, download_count,
		created_by, created_at, updated_at
		FROM file_shares WHERE id = ?`, id).Scan(
		&s.ID, &s.FilePath, &s.FileName, &s.FileSize, &s.Token,
		&s.Password, &s.ExpiresAt, &s.MaxDownloads, &s.DownloadCount,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get file share by id: %w", err)
	}
	return s, nil
}

// Update modifies only the access-control fields of a share. File path and
// token are never changed. A nil Password leaves the stored password intact;
// a non-nil pointer (including empty string) replaces it. ClearExpiry sets
// expires_at to ”. MaxDownloads is applied only when non-nil.
func (r *sqliteShareRepo) Update(ctx context.Context, id int64, req *UpdateShareRequest) error {
	sets := []string{"updated_at = datetime('now')"}
	args := []interface{}{}
	if req.Password != nil {
		sets = append(sets, "password = ?")
		args = append(args, *req.Password)
	}
	if req.ClearExpiry {
		sets = append(sets, "expires_at = ''")
	} else if req.ExpiresAt != "" {
		sets = append(sets, "expires_at = ?")
		args = append(args, req.ExpiresAt)
	}
	if req.MaxDownloads != nil {
		sets = append(sets, "max_downloads = ?")
		args = append(args, *req.MaxDownloads)
	}
	args = append(args, id)
	q := "UPDATE file_shares SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	_, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update file share: %w", err)
	}
	return nil
}

func (r *sqliteShareRepo) List(ctx context.Context, createdBy int64) ([]FileShare, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, file_path, file_name, file_size, token,
		COALESCE(password,''), COALESCE(expires_at,''), max_downloads, download_count,
		created_by, created_at, updated_at
		FROM file_shares WHERE created_by = ? ORDER BY id DESC`, createdBy)
	if err != nil {
		return nil, fmt.Errorf("list file shares: %w", err)
	}
	defer rows.Close()

	var shares []FileShare
	for rows.Next() {
		var s FileShare
		if err := rows.Scan(&s.ID, &s.FilePath, &s.FileName, &s.FileSize, &s.Token,
			&s.Password, &s.ExpiresAt, &s.MaxDownloads, &s.DownloadCount,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		shares = append(shares, s)
	}
	return shares, nil
}

func (r *sqliteShareRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM file_shares WHERE id = ?", id)
	return err
}

func (r *sqliteShareRepo) IncrementDownloads(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE file_shares SET download_count = download_count + 1, updated_at = datetime('now') WHERE id = ?", id)
	return err
}

func (r *sqliteShareRepo) DeleteExpired(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM file_shares WHERE
		(expires_at != '' AND expires_at <= datetime('now')) OR
		(max_downloads > 0 AND download_count >= max_downloads)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
