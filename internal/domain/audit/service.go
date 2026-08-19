package audit

import (
	"context"
	"encoding/json"
	"log"
	"maps"
	"sync"
	"time"

	"easyserver/internal/domain/auth"
	"easyserver/internal/infra/config"
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

func newAuditWriter(ctx context.Context, repo Repository) *auditWriter {
	w := &auditWriter{
		repo:     repo,
		ch:       make(chan auditEntry, 1000),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	// writer 是进程级后台消费者：脱离调用方 ctx 的取消，仅继承值。
	go w.run(context.WithoutCancel(ctx))
	return w
}

func (w *auditWriter) run(ctx context.Context) {
	batch := make([]auditEntry, 0, 100)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case entry := <-w.ch:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				w.flush(ctx, batch)
				batch = batch[:0]
				timer.Reset(2 * time.Second)
			}
		case <-timer.C:
			if len(batch) > 0 {
				w.flush(ctx, batch)
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
						w.flush(ctx, batch)
					}
					close(w.finished)
					return
				}
			}
		}
	}
}

func (w *auditWriter) flush(ctx context.Context, batch []auditEntry) {
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
	if err := w.repo.AppendBatch(ctx, entries); err != nil {
		log.Printf("audit: failed to flush batch: %v", err)
	}
}

func (w *auditWriter) close() {
	close(w.done)
	<-w.finished
}

// Service provides audit logging.
type Service struct {
	auditRepo Repository
	writer    *auditWriter
	store     *config.Store
}

// NewService creates a new audit Service. 保留天数运行时实时读 store
// （settings 修改后下次清理即生效）。
func NewService(ctx context.Context, wg *sync.WaitGroup, auditRepo Repository, store *config.Store) *Service {
	s := &Service{
		auditRepo: auditRepo,
		writer:    newAuditWriter(ctx, auditRepo),
		store:     store,
	}
	wg.Go(func() {
		s.cleanupLoop(ctx)
	})
	return s
}

// retentionDays 返回当前生效的保留天数（兜底 90 天）。
func (s *Service) retentionDays() int {
	d := s.store.Get().Audit.RetentionDays
	if d <= 0 {
		return 90
	}
	return d
}

func (s *Service) Close() {
	s.writer.close()
}

func (s *Service) cleanupLoop(ctx context.Context) {
	s.cleanupOldRecords(ctx)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupOldRecords(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) cleanupOldRecords(ctx context.Context) {
	days := s.retentionDays()
	since := time.Now().AddDate(0, 0, -days)
	rows, err := s.auditRepo.Clean(ctx, since)
	if err != nil {
		log.Printf("audit: cleanup error: %v", err)
		return
	}
	if rows > 0 {
		log.Printf("audit: cleaned up %d old records (older than %d days)", rows, days)
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
func (s *Service) LogOperation(ctx context.Context, userID int64, username, action, resource string, extra map[string]any, ip, userAgent string) {
	now := time.Now()
	detailData := map[string]any{
		"timestamp": now.Format(time.RFC3339),
	}
	maps.Copy(detailData, extra)
	detailJSON, _ := json.Marshal(detailData)
	s.enqueue(auditEntry{userID, username, action, resource, string(detailJSON), ip, userAgent, now, "operation"})
}

// LogRequest logs an HTTP request, written by the global audit middleware.
// detail is expected to be a complete JSON string (flat layer with status/method/...);
// it is stored verbatim so Stats/alerts can extract fields at the top level.
func (s *Service) LogRequest(ctx context.Context, userID int64, username, action, resource, detail, ip, userAgent string) {
	now := time.Now()
	s.enqueue(auditEntry{userID, username, action, resource, detail, ip, userAgent, now, "request"})
}

// LogSecurityEvent logs a security event. The action column is the coarse verb
// ("认证"); the human-readable summary is carried in detail.summary.
// Operation logs do not record IP/user-agent (request-log concern).
func (s *Service) LogSecurityEvent(ctx context.Context, username, summary string) {
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]any{
		"summary":   summary,
		"timestamp": now.Format(time.RFC3339),
	})
	s.enqueue(auditEntry{0, username, string(ActionAuth), string(ResourceAuth), string(detailJSON), "", "", now, "operation"})
}

// LogSystemEvent logs a system event. The action column is the coarse verb
// ("其他"); the human-readable summary is carried in detail.summary.
func (s *Service) LogSystemEvent(ctx context.Context, summary string) {
	now := time.Now()
	detailJSON, _ := json.Marshal(map[string]any{
		"summary":   summary,
		"timestamp": now.Format(time.RFC3339),
	})
	s.enqueue(auditEntry{0, "system", string(ActionOther), string(ResourceSystem), string(detailJSON), "", "", now, "operation"})
}

// LogLoginEvent records a login event (success/failed/blocked) as an audit
// operation log. Satisfies auth.LoginEventLogger implicitly.
func (s *Service) LogLoginEvent(ctx context.Context, event auth.LoginEvent) {
	now := time.Now()
	detailData := map[string]any{
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
