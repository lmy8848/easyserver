package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"easyserver/internal/repository"
)

type auditEntry struct {
	userID    int64
	username  string
	action    string
	resource  string
	detail    string
	ip        string
	userAgent string
	createdAt time.Time
}

type AuditWriter struct {
	db         *sql.DB
	ch         chan auditEntry
	done       chan struct{}
	finished   chan struct{}
	signingKey []byte
}

func newAuditWriter(db *sql.DB, signingKey []byte) *AuditWriter {
	w := &AuditWriter{
		db:         db,
		ch:         make(chan auditEntry, 1000),
		done:       make(chan struct{}),
		finished:   make(chan struct{}),
		signingKey: signingKey,
	}
	go w.run()
	return w
}

func (w *AuditWriter) run() {
	batch := make([]auditEntry, 0, 100)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case entry := <-w.ch:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				w.flush(batch)
				batch = batch[:0]
				timer.Reset(2 * time.Second)
			}
		case <-timer.C:
			if len(batch) > 0 {
				w.flush(batch)
				batch = batch[:0]
			}
			timer.Reset(2 * time.Second)
		case <-w.done:
			// Drain remaining entries
			for {
				select {
				case entry := <-w.ch:
					batch = append(batch, entry)
				default:
					if len(batch) > 0 {
						w.flush(batch)
					}
					close(w.finished)
					return
				}
			}
		}
	}
}

func (w *AuditWriter) flush(batch []auditEntry) {
	tx, err := w.db.Begin()
	if err != nil {
		log.Printf("audit: failed to begin transaction: %v", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at, signature) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		log.Printf("audit: failed to prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, e := range batch {
		// Generate HMAC-SHA256 signature
		signature := w.signEntry(e)
		if _, err := stmt.Exec(e.userID, e.username, e.action, e.resource, e.detail, e.ip, e.userAgent, e.createdAt, signature); err != nil {
			log.Printf("audit: failed to insert entry: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("audit: failed to commit batch: %v", err)
	}
}

// signEntry generates HMAC-SHA256 signature for an audit entry
func (w *AuditWriter) signEntry(e auditEntry) string {
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		e.userID, e.username, e.action, e.resource, e.detail, e.ip, e.userAgent, e.createdAt.Format(time.RFC3339Nano))
	mac := hmac.New(sha256.New, w.signingKey)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func (w *AuditWriter) close() {
	close(w.done)
	<-w.finished
}

type AuditService struct {
	db            *sql.DB
	auditRepo     repository.AuditRepository
	writer        *AuditWriter
	signingKey    []byte
	retentionDays int
}

func NewAuditService(db *sql.DB, retentionDays int) *AuditService {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	// Load or generate the signing key for HMAC-SHA256 (persisted across restarts)
	key := loadOrCreateSigningKey(db)

	s := &AuditService{
		db:            db,
		writer:        newAuditWriter(db, key),
		signingKey:    key,
		retentionDays: retentionDays,
	}
	// Start automatic cleanup task
	go s.cleanupLoop()
	return s
}

// loadOrCreateSigningKey loads the audit signing key from the database, or
// generates a new random key and persists it if none exists. This ensures
// signature continuity across restarts (old audit logs remain verifiable).
func loadOrCreateSigningKey(db *sql.DB) []byte {
	// Ensure the settings table exists
	db.Exec(`CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	var hexKey string
	err := db.QueryRow("SELECT value FROM system_settings WHERE key = 'audit_signing_key'").Scan(&hexKey)
	if err == nil && hexKey != "" {
		if key, err := hex.DecodeString(hexKey); err == nil && len(key) == 32 {
			return key
		}
	}

	// Generate a new key and persist it
	key := make([]byte, 32)
	if _, err := randomBytes(key); err != nil {
		// Fallback to a fixed key (less secure but functional)
		key = []byte("easyserver-audit-signing-key-default")
	}
	db.Exec("INSERT OR REPLACE INTO system_settings (key, value) VALUES ('audit_signing_key', ?)", hex.EncodeToString(key))
	return key
}

// randomBytes fills the provided slice with random bytes
func randomBytes(b []byte) (int, error) {
	return rand.Read(b)
}

// SetAuditRepository sets the audit repository implementation
func (s *AuditService) SetAuditRepository(repo repository.AuditRepository) {
	s.auditRepo = repo
}

func (s *AuditService) Close() {
	s.writer.close()
}

// cleanupLoop runs automatic cleanup every 24 hours
func (s *AuditService) cleanupLoop() {
	// Initial cleanup on startup
	s.cleanupOldRecords()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanupOldRecords()
	}
}

// cleanupOldRecords deletes audit logs older than retentionDays and runs VACUUM
func (s *AuditService) cleanupOldRecords() {
	since := time.Now().AddDate(0, 0, -s.retentionDays)
	var rows int64
	var err error

	if s.auditRepo != nil {
		rows, err = s.auditRepo.Clean(context.Background(), since)
	} else {
		var result sql.Result
		result, err = s.db.Exec("DELETE FROM audit_logs WHERE created_at < ?", since)
		if err == nil {
			rows, _ = result.RowsAffected()
		}
	}

	if err != nil {
		log.Printf("audit: cleanup error: %v", err)
		return
	}
	if rows > 0 {
		log.Printf("audit: cleaned up %d old records (older than %d days)", rows, s.retentionDays)
		// Run VACUUM to reclaim space (only if significant deletions)
		if rows > 100 {
			if _, err := s.db.Exec("VACUUM"); err != nil {
				log.Printf("audit: vacuum error: %v", err)
			}
		}
	}
}

// LogOperation logs a server-level operation
func (s *AuditService) LogOperation(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"detail":    detail,
		"timestamp": now.Format(time.RFC3339),
	})
	entry := auditEntry{userID, username, action, resource, string(detailJSON), ip, userAgent, now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogTerminalCommand logs a command executed in the terminal
func (s *AuditService) LogTerminalCommand(ctx context.Context, userID int64, username, sessionID, command, ip string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"session_id": sessionID,
		"command":    command,
		"timestamp":  now.Format(time.RFC3339),
	})
	entry := auditEntry{userID, username, "TERMINAL", "/terminal/" + sessionID, string(detailJSON), ip, "WebTerminal", now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogFileOperation logs a file operation
func (s *AuditService) LogFileOperation(ctx context.Context, userID int64, username, action, filePath, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"file_path": filePath,
		"action":    action,
		"timestamp": now.Format(time.RFC3339),
	})
	entry := auditEntry{userID, username, "FILE_" + action, filePath, string(detailJSON), ip, userAgent, now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogSecurityEvent logs a security event
func (s *AuditService) LogSecurityEvent(ctx context.Context, username, action, detail, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"detail":    detail,
		"timestamp": now.Format(time.RFC3339),
	})
	entry := auditEntry{0, username, "SECURITY_" + action, "/auth", string(detailJSON), ip, userAgent, now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogSystemEvent logs a system event
func (s *AuditService) LogSystemEvent(ctx context.Context, action, detail string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"detail":    detail,
		"timestamp": now.Format(time.RFC3339),
	})
	entry := auditEntry{0, "system", "SYSTEM_" + action, "/system", string(detailJSON), "127.0.0.1", "EasyServer", now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogServiceOperation logs a service operation
func (s *AuditService) LogServiceOperation(ctx context.Context, userID int64, username, action, serviceName, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"service":   serviceName,
		"action":    action,
		"timestamp": now.Format(time.RFC3339),
	})
	entry := auditEntry{userID, username, "SERVICE_" + action, "/services/" + serviceName, string(detailJSON), ip, userAgent, now}
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// GetAuditLogs returns audit logs with filtering
func (s *AuditService) GetAuditLogs(ctx context.Context, page, pageSize int, username, action, resource, ip, startDate, endDate string) (int64, []map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if s.auditRepo != nil {
		filter := repository.AuditFilter{
			Username:  username,
			Action:    action,
			Resource:  resource,
			IP:        ip,
			StartDate: startDate,
			EndDate:   endDate,
			Offset:    (page - 1) * pageSize,
			Limit:     pageSize,
		}
		total, logs, err := s.auditRepo.Query(ctx, filter)
		if err != nil {
			return 0, nil, err
		}
		items := make([]map[string]interface{}, 0, len(logs))
		for _, log := range logs {
			items = append(items, map[string]interface{}{
				"id":         log.ID,
				"user_id":    log.UserID,
				"username":   log.Username,
				"action":     log.Action,
				"resource":   log.Resource,
				"detail":     log.Detail,
				"ip":         log.IP,
				"user_agent": log.UserAgent,
				"created_at": log.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		return total, items, nil
	}

	where := "1=1"
	args := []interface{}{}

	if username != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+username+"%")
	}
	if action != "" {
		where += " AND action LIKE ?"
		args = append(args, "%"+action+"%")
	}
	if resource != "" {
		where += " AND resource LIKE ?"
		args = append(args, "%"+resource+"%")
	}
	if ip != "" {
		where += " AND ip LIKE ?"
		args = append(args, "%"+ip+"%")
	}
	if startDate != "" {
		where += " AND created_at >= ?"
		args = append(args, startDate)
	}
	if endDate != "" {
		where += " AND created_at <= ?"
		args = append(args, endDate+" 23:59:59")
	}

	// Get total count
	var total int64
	countQuery := "SELECT COUNT(*) FROM audit_logs WHERE " + where
	s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	// Get items
	offset := (page - 1) * pageSize
	query := `SELECT id, user_id, username, action, resource, detail, ip, user_agent, created_at
	          FROM audit_logs WHERE ` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		var id, userID int64
		var username, action, resource, detail, ip, userAgent string
		var createdAt time.Time
		if err := rows.Scan(&id, &userID, &username, &action, &resource, &detail, &ip, &userAgent, &createdAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":         id,
			"user_id":    userID,
			"username":   username,
			"action":     action,
			"resource":   resource,
			"detail":     detail,
			"ip":         ip,
			"user_agent": userAgent,
			"created_at": createdAt.Format("2006-01-02 15:04:05"),
		})
	}

	return total, items, nil
}

// VerifySignature verifies the integrity of an audit log entry
func (s *AuditService) VerifySignature(ctx context.Context, id int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var userID int64
	var username, action, resource, detail, ip, userAgent, signature string
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, username, action, resource, detail, ip, user_agent, created_at, signature
		 FROM audit_logs WHERE id = ?`, id,
	).Scan(&userID, &username, &action, &resource, &detail, &ip, &userAgent, &createdAt, &signature)
	if err != nil {
		return false, err
	}

	// Regenerate signature
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		userID, username, action, resource, detail, ip, userAgent, createdAt.Format(time.RFC3339Nano))
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(data))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected)), nil
}

// VerifyAllSignatures verifies integrity of all audit log entries
func (s *AuditService) VerifyAllSignatures(ctx context.Context) (total, valid, invalid int, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM audit_logs ORDER BY id DESC LIMIT 1000`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		total++
		ok, err := s.VerifySignature(ctx, id)
		if err != nil {
			continue
		}
		if ok {
			valid++
		} else {
			invalid++
		}
	}
	return total, valid, invalid, nil
}
