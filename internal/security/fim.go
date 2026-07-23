package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"time"

	"easyserver/internal/infra/apperror"
)

// FIMBaseline is one monitored file's baseline hash.
type FIMBaseline struct {
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	Mtime     string `json:"mtime"`
	UpdatedAt string `json:"updated_at"`
}

// FIMChange is a detected file change.
type FIMChange struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"` // modified / added / deleted
	OldHash    string `json:"old_hash"`
	NewHash    string `json:"new_hash"`
	DetectedAt string `json:"detected_at"`
}

// defaultFIMPaths are the critical files monitored by default.
var defaultFIMPaths = []string{
	"/etc/ssh/sshd_config",
	"/etc/nginx/nginx.conf",
	"/opt/easyserver/config.yaml",
	"/root/.ssh/authorized_keys",
}

// ScanBaseline hashes the default critical files and stores/updates the baseline.
func (s *Service) ScanBaseline(ctx context.Context) error {
	if s.db == nil {
		return apperror.ErrInternal.WithMessage("db 不可用")
	}
	for _, p := range defaultFIMPaths {
		hash, size, mtime, err := hashFile(p)
		if err != nil {
			continue // file does not exist, skip
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO fim_baseline (path, hash, size, mtime, updated_at) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(path) DO UPDATE SET hash=excluded.hash, size=excluded.size, mtime=excluded.mtime, updated_at=excluded.updated_at`,
			p, hash, size, mtime, time.Now().Format(time.RFC3339))
		if err != nil {
			return apperror.WrapError(err)
		}
	}
	return nil
}

// CheckChanges compares current file hashes against the baseline and records
// any modifications/deletions. Returns the detected changes.
func (s *Service) CheckChanges(ctx context.Context) ([]FIMChange, error) {
	if s.db == nil {
		return nil, apperror.ErrInternal.WithMessage("db 不可用")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT path, hash FROM fim_baseline")
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	defer rows.Close()
	var changes []FIMChange
	for rows.Next() {
		var path, oldHash string
		if err := rows.Scan(&path, &oldHash); err != nil {
			continue
		}
		newHash, _, _, err := hashFile(path)
		if err != nil {
			// file deleted
			ch := FIMChange{Path: path, ChangeType: "deleted", OldHash: oldHash, DetectedAt: time.Now().Format(time.RFC3339)}
			changes = append(changes, ch)
			s.recordChange(ctx, ch)
			continue
		}
		if newHash != oldHash {
			ch := FIMChange{Path: path, ChangeType: "modified", OldHash: oldHash, NewHash: newHash, DetectedAt: time.Now().Format(time.RFC3339)}
			changes = append(changes, ch)
			s.recordChange(ctx, ch)
			// update baseline to new hash
			_, _ = s.db.ExecContext(ctx, "UPDATE fim_baseline SET hash=?, updated_at=? WHERE path=?", newHash, time.Now().Format(time.RFC3339), path)
		}
	}
	return changes, nil
}

func (s *Service) recordChange(ctx context.Context, ch FIMChange) {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO fim_changes (path, change_type, old_hash, new_hash, detected_at) VALUES (?, ?, ?, ?, ?)",
		ch.Path, ch.ChangeType, ch.OldHash, ch.NewHash, ch.DetectedAt)
	if err != nil {
		log.Printf("fim: record change: %v", err)
	}
	log.Printf("fim: %s %s (old=%s new=%s)", ch.ChangeType, ch.Path, ch.OldHash, ch.NewHash)
}

// ListBaseline returns the current baseline.
func (s *Service) ListBaseline(ctx context.Context) ([]FIMBaseline, error) {
	if s.db == nil {
		return nil, apperror.ErrInternal.WithMessage("db 不可用")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT path, hash, size, mtime, updated_at FROM fim_baseline ORDER BY path")
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	defer rows.Close()
	var bl []FIMBaseline
	for rows.Next() {
		var b FIMBaseline
		if err := rows.Scan(&b.Path, &b.Hash, &b.Size, &b.Mtime, &b.UpdatedAt); err != nil {
			continue
		}
		bl = append(bl, b)
	}
	return bl, nil
}

// ListChanges returns recent detected changes.
func (s *Service) ListChanges(ctx context.Context, limit int) ([]FIMChange, error) {
	if s.db == nil {
		return nil, apperror.ErrInternal.WithMessage("db 不可用")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, "SELECT path, change_type, old_hash, new_hash, detected_at FROM fim_changes ORDER BY detected_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, apperror.WrapError(err)
	}
	defer rows.Close()
	var ch []FIMChange
	for rows.Next() {
		var c FIMChange
		if err := rows.Scan(&c.Path, &c.ChangeType, &c.OldHash, &c.NewHash, &c.DetectedAt); err != nil {
			continue
		}
		ch = append(ch, c)
	}
	return ch, nil
}

// ResetBaseline clears the baseline and re-scans the default paths.
func (s *Service) ResetBaseline(ctx context.Context) error {
	if s.db == nil {
		return apperror.ErrInternal.WithMessage("db 不可用")
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM fim_baseline"); err != nil {
		return apperror.WrapError(err)
	}
	return s.ScanBaseline(ctx)
}

// hashFile returns sha256 hex, size, and mtime of a file.
func hashFile(path string) (hash string, size int64, mtime string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, "", err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", 0, "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), fi.Size(), fi.ModTime().Format(time.RFC3339), nil
}
