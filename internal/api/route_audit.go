package api

import (
	"database/sql"

	"easyserver/internal/repository"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerAuditRoutes registers audit log routes
func registerAuditRoutes(protected *gin.RouterGroup, db *sql.DB, auditService *service.AuditService, auditRepo repository.AuditRepository) {
	handler := NewAuditHandlerWithRepo(db, auditService, auditRepo)
	protected.GET("/audit-logs", handler.List)
	protected.GET("/audit-logs/actions", handler.GetActions)
	protected.GET("/audit-logs/stats", handler.Stats)
	protected.GET("/audit-logs/clean-policy", handler.GetCleanPolicy)
	protected.GET("/audit-logs/export", handler.Export)
	protected.DELETE("/audit-logs/clean", handler.Clean)
	protected.GET("/audit-logs/verify", handler.VerifyIntegrity)
}
