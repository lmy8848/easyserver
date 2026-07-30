package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"easyserver/internal/auth"
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
	auditRepo     Repository
	writer        *auditWriter
	retentionDays int
}

// NewService creates a new audit Service.
func NewService(ctx context.Context, wg *sync.WaitGroup, auditRepo Repository, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 90
	}

	s := &Service{
		auditRepo:     auditRepo,
		writer:        newAuditWriter(auditRepo),
		retentionDays: retentionDays,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.cleanupLoop(ctx)
	}()
	return s
}

func (s *Service) Close() {
	s.writer.close()
}

func (s *Service) cleanupLoop(ctx context.Context) {
	s.cleanupOldRecords()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupOldRecords()
		case <-ctx.Done():
			return
		}
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
func (s *Service) LogOperation(ctx context.Context, userID int64, username string, action ActionCategory, resource ResourceCategory, extra map[string]interface{}, ip, userAgent string) {
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
	s.enqueue(auditEntry{userID, username, string(action), string(resource), string(detailJSON), ip, userAgent, now, "operation"})
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
	s.enqueue(auditEntry{0, username, string(ActionAuth), string(ResourceAuth), string(detailJSON), "", "", now, "operation"})
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
	s.enqueue(auditEntry{0, "system", string(ActionOther), string(ResourceSystem), string(detailJSON), "", "", now, "operation"})
}

// LogLoginEvent records a login event (success/failed/blocked) as an audit
// operation log. Satisfies auth.LoginEventLogger implicitly.
func (s *Service) LogLoginEvent(ctx context.Context, event auth.LoginEvent) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	detailData := map[string]interface{}{
		"action":     event.Action,
		"username":   event.Username,
		"ip":         event.IP,
		"user_agent": event.UserAgent,
		"success":    event.Success,
		"timestamp":  now.Format(time.RFC3339),
	}
	if event.Reason != "" {
		detailData["reason"] = event.Reason
	}
	detailJSON, _ := json.Marshal(detailData)
	s.enqueue(auditEntry{0, event.Username, string(ActionAuth), string(ResourceAuth), string(detailJSON), event.IP, event.UserAgent, now, "operation"})
}

// loginRecord is a row returned from the login-events query.
type loginRecord struct {
	Username  string
	Action    string
	IP        string
	UserAgent string
	CreatedAt time.Time
}

// getLoginRecords queries operation-type audit logs with action=认证.
func (s *Service) getLoginRecords(ctx context.Context, since time.Time, limit int) ([]loginRecord, error) {
	_, logs, err := s.auditRepo.Query(ctx, AuditFilter{
		Type:   "operation",
		Action: string(ActionAuth),
		Limit:  limit,
		Offset: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("query login events: %w", err)
	}
	var records []loginRecord
	for _, l := range logs {
		if l.CreatedAt.Before(since) {
			continue
		}
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(l.Detail), &detail); err != nil {
			continue
		}
		action, _ := detail["action"].(string)
		if !strings.HasPrefix(action, "LOGIN") {
			continue
		}
		ip, _ := detail["ip"].(string)
		ua, _ := detail["user_agent"].(string)
		username, _ := detail["username"].(string)
		records = append(records, loginRecord{
			Username:  username,
			Action:    action,
			IP:        ip,
			UserAgent: ua,
			CreatedAt: l.CreatedAt,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

// GetLoginHistory returns recent login events for the security panel.
func (s *Service) GetLoginHistory(ctx context.Context, limit int) ([]loginRecord, error) {
	return s.getLoginRecords(ctx, time.Time{}, limit)
}

// CountFailedLoginsByIP counts LOGIN_FAILED events per IP since the given
// time, returning only IPs whose count meets or exceeds threshold.
func (s *Service) CountFailedLoginsByIP(ctx context.Context, since time.Time, threshold int) map[string]int {
	records, err := s.getLoginRecords(ctx, since, 10000)
	if err != nil {
		log.Printf("audit: count failed logins: %v", err)
		return nil
	}
	counts := map[string]int{}
	for _, r := range records {
		if r.Action == "LOGIN_FAILED" && r.IP != "" {
			counts[r.IP]++
		}
	}
	for ip, c := range counts {
		if c < threshold {
			delete(counts, ip)
		}
	}
	return counts
}
