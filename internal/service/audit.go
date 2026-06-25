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
	repo       repository.AuditRepository
	ch         chan auditEntry
	done       chan struct{}
	finished   chan struct{}
	signingKey []byte
}

func newAuditWriter(repo repository.AuditRepository, signingKey []byte) *AuditWriter {
	w := &AuditWriter{
		repo:       repo,
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
	entries := make([]repository.SignedAuditEntry, len(batch))
	for i, e := range batch {
		entries[i] = repository.SignedAuditEntry{
			UserID:    e.userID,
			Username:  e.username,
			Action:    e.action,
			Resource:  e.resource,
			Detail:    e.detail,
			IP:        e.ip,
			UserAgent: e.userAgent,
			CreatedAt: e.createdAt,
			Signature: w.signEntry(e),
		}
	}
	if err := w.repo.AppendSignedBatch(context.Background(), entries); err != nil {
		log.Printf("audit: failed to flush batch: %v", err)
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

func NewAuditService(db *sql.DB, auditRepo repository.AuditRepository, retentionDays int) *AuditService {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	// Load or generate the signing key for HMAC-SHA256 (persisted across restarts)
	key := loadOrCreateSigningKey(db)

	s := &AuditService{
		db:            db,
		auditRepo:     auditRepo,
		writer:        newAuditWriter(auditRepo, key),
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
	rows, err := s.auditRepo.Clean(context.Background(), since)
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

// VerifySignature verifies the integrity of an audit log entry
func (s *AuditService) VerifySignature(ctx context.Context, id int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entry, err := s.auditRepo.GetSignedEntry(ctx, id)
	if err != nil {
		return false, err
	}

	// Regenerate signature
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		entry.UserID, entry.Username, entry.Action, entry.Resource, entry.Detail, entry.IP, entry.UserAgent, entry.CreatedAt.Format(time.RFC3339Nano))
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(data))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(entry.Signature), []byte(expected)), nil
}

// VerifyAllSignatures verifies integrity of all audit log entries
func (s *AuditService) VerifyAllSignatures(ctx context.Context) (total, valid, invalid int, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ids, err := s.auditRepo.ListIDsForVerification(ctx, 1000)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, id := range ids {
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
