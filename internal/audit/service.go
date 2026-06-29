package audit

import (
	"context"
	"database/sql"
	"encoding/json"
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
	logType   string
}

type auditWriter struct {
	repo     Repository
	ch       chan auditEntry
	done     chan struct{}
	finished chan struct{}
}

func newAuditWriter(repo Repository) *auditWriter {
	w := &auditWriter{
		repo:     repo,
		ch:       make(chan auditEntry, 1000),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
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
	entries := make([]AuditLog, len(batch))
	for i, e := range batch {
		entries[i] = AuditLog{
			UserID:    e.userID,
			Username:  e.username,
			Action:    e.action,
			Resource:  e.resource,
			Detail:    e.detail,
			IP:        e.ip,
			UserAgent: e.userAgent,
			Type:      e.logType,
			CreatedAt: e.createdAt,
		}
	}
	if err := w.repo.AppendBatch(context.Background(), entries); err != nil {
		log.Printf("audit: failed to flush batch: %v", err)
	}
}

func (w *auditWriter) close() {
	close(w.done)
	<-w.finished
}

// Service provides audit logging.
type Service struct {
	db            *sql.DB
	auditRepo     Repository
	writer        *auditWriter
	retentionDays int
}

// NewService creates a new audit Service. db is reserved for future use.
func NewService(db *sql.DB, auditRepo Repository, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	s := &Service{
		db:            db,
		auditRepo:     auditRepo,
		writer:        newAuditWriter(auditRepo),
		retentionDays: retentionDays,
	}
	go s.cleanupLoop()
	return s
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

func (s *Service) enqueue(entry auditEntry) {
	select {
	case s.writer.ch <- entry:
	default:
		log.Printf("audit: channel full, dropping entry")
	}
}

// LogOperation logs a server-level operation.
func (s *Service) LogOperation(ctx context.Context, userID int64, username, action, resource string, extra map[string]interface{}, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailData := map[string]interface{}{
		"timestamp": now.Format(time.RFC3339),
	}
	for k, v := range extra {
		detailData[k] = v
	}
	detailJSON, _ := json.Marshal(detailData)
	s.enqueue(auditEntry{userID, username, action, resource, string(detailJSON), ip, userAgent, now, "operation"})
}

// LogRequest logs an HTTP request, written by the global audit middleware.
// detail is expected to be a complete JSON string (flat layer with status/method/...);
// it is stored verbatim so Stats/alerts can extract fields at the top level.
func (s *Service) LogRequest(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	s.enqueue(auditEntry{userID, username, action, resource, detail, ip, userAgent, now, "request"})
}

// LogSecurityEvent logs a security event. The action column is the coarse verb
// ("认证"); the human-readable summary is carried in detail.summary.
// Operation logs do not record IP/user-agent (request-log concern).
func (s *Service) LogSecurityEvent(ctx context.Context, username, summary string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"summary":   summary,
		"timestamp": now.Format(time.RFC3339),
	})
	s.enqueue(auditEntry{0, username, "认证", "/auth", string(detailJSON), "", "", now, "operation"})
}

// LogSystemEvent logs a system event. The action column is the coarse verb
// ("其他"); the human-readable summary is carried in detail.summary.
func (s *Service) LogSystemEvent(ctx context.Context, summary string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]interface{}{
		"summary":   summary,
		"timestamp": now.Format(time.RFC3339),
	})
	s.enqueue(auditEntry{0, "system", "其他", "/system", string(detailJSON), "", "", now, "operation"})
}
