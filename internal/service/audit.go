package service

import (
	"database/sql"

	"easyserver/internal/audit"
)

// AuditService is now defined in internal/audit; kept as alias for backward compatibility.
type AuditService = audit.Service

// NewAuditService creates a new AuditService.
// This is a forwarding stub; the implementation lives in internal/audit.
func NewAuditService(db *sql.DB, auditRepo audit.Repository, retentionDays int) *AuditService {
	return audit.NewService(db, auditRepo, retentionDays)
}
