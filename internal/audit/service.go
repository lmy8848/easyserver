package audit

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

type auditWriter struct {
	repo       Repository
	ch         chan auditEntry
	done       chan struct{}
	finished   chan struct{}
	signingKey []byte
}

func newAuditWriter(repo Repository, signingKey []byte) *auditWriter {
	w := &auditWriter{
		repo:       repo,
		ch:         make(chan auditEntry, 1000),
		done:       make(chan struct{}),
		finished:   make(chan struct{}),
		signingKey: signingKey,
	}
	go w.run()
	return w
}

func (w *auditWriter) run() {
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

func (w *auditWriter) flush(batch []auditEntry) {
	entries := make([]SignedAuditEntry, len(batch))
	for i, e := range batch {
		entries[i] = SignedAuditEntry{
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

func (w *auditWriter) signEntry(e auditEntry) string {
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		e.userID, e.username, e.action, e.resource, e.detail, e.ip, e.userAgent, e.createdAt.Format(time.RFC3339Nano))
	mac := hmac.New(sha256.New, w.signingKey)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func (w *auditWriter) close() {
	close(w.done)
	<-w.finished
}

// Service provides audit logging with HMAC-signed entries.
type Service struct {
	db            *sql.DB
	auditRepo     Repository
	writer        *auditWriter
	signingKey    []byte
	retentionDays int
}

// NewService creates a new audit Service. db is used for HMAC signing key persistence.
func NewService(db *sql.DB, auditRepo Repository, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	key := loadOrCreateSigningKey(db)

	s := &Service{
		db:            db,
		auditRepo:     auditRepo,
		writer:        newAuditWriter(auditRepo, key),
		signingKey:    key,
		retentionDays: retentionDays,
	}
	go s.cleanupLoop()
	return s
}

func loadOrCreateSigningKey(db *sql.DB) []byte {
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

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		key = []byte("easyserver-audit-signing-key-default")
	}
	db.Exec("INSERT OR REPLACE INTO system_settings (key, value) VALUES ('audit_signing_key', ?)", hex.EncodeToString(key))
	return key
}

func (s *Service) Close() {
	s.writer.close()
}

func (s *Service) cleanupLoop() {
	s.cleanupOldRecords()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanupOldRecords()
	}
}

func (s *Service) cleanupOldRecords() {
	since := time.Now().AddDate(0, 0, -s.retentionDays)
	rows, err := s.auditRepo.Clean(context.Background(), since)
	if err != nil {
		log.Printf("audit: cleanup error: %v", err)
		return
	}
	if rows > 0 {
		log.Printf("audit: cleaned up %d old records (older than %d days)", rows, s.retentionDays)
		if rows > 100 {
			if _, err := s.db.Exec("VACUUM"); err != nil {
				log.Printf("audit: vacuum error: %v", err)
			}
		}
	}
}

// LogOperation logs a server-level operation.
func (s *Service) LogOperation(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string) {
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

// LogTerminalCommand logs a command executed in the terminal.
func (s *Service) LogTerminalCommand(ctx context.Context, userID int64, username, sessionID, command, ip string) {
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

// LogFileOperation logs a file operation.
func (s *Service) LogFileOperation(ctx context.Context, userID int64, username, action, filePath, ip, userAgent string) {
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

// LogSecurityEvent logs a security event.
func (s *Service) LogSecurityEvent(ctx context.Context, username, action, detail, ip, userAgent string) {
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

// LogSystemEvent logs a system event.
func (s *Service) LogSystemEvent(ctx context.Context, action, detail string) {
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

// LogServiceOperation logs a service operation.
func (s *Service) LogServiceOperation(ctx context.Context, userID int64, username, action, serviceName, ip, userAgent string) {
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

// GetAuditLogs returns audit logs with filtering.
func (s *Service) GetAuditLogs(ctx context.Context, page, pageSize int, username, action, resource, ip, startDate, endDate string) (int64, []map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	filter := AuditFilter{
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
	for _, l := range logs {
		items = append(items, map[string]interface{}{
			"id":         l.ID,
			"user_id":    l.UserID,
			"username":   l.Username,
			"action":     l.Action,
			"resource":   l.Resource,
			"detail":     l.Detail,
			"ip":         l.IP,
			"user_agent": l.UserAgent,
			"created_at": l.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return total, items, nil
}

// VerifySignature verifies the integrity of an audit log entry.
func (s *Service) VerifySignature(ctx context.Context, id int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entry, err := s.auditRepo.GetSignedEntry(ctx, id)
	if err != nil {
		return false, err
	}

	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		entry.UserID, entry.Username, entry.Action, entry.Resource, entry.Detail, entry.IP, entry.UserAgent, entry.CreatedAt.Format(time.RFC3339Nano))
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(data))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(entry.Signature), []byte(expected)), nil
}

// VerifyAllSignatures verifies integrity of all audit log entries.
func (s *Service) VerifyAllSignatures(ctx context.Context) (total, valid, invalid int, err error) {
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
