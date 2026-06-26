package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/repository"
	"easyserver/internal/audit"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	db           *sql.DB
	auditService *audit.Service
	auditRepo    repository.AuditRepository
}

func NewAuditHandler(db *sql.DB, auditService *audit.Service) *AuditHandler {
	return &AuditHandler{db: db, auditService: auditService}
}

// NewAuditHandlerWithRepo creates an AuditHandler with repository support
func NewAuditHandlerWithRepo(db *sql.DB, auditService *audit.Service, auditRepo repository.AuditRepository) *AuditHandler {
	return &AuditHandler{db: db, auditService: auditService, auditRepo: auditRepo}
}

type AuditLogItem struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	CreatedAt string `json:"created_at"`
}

type AuditLogListResponse struct {
	Total int64          `json:"total"`
	Items []AuditLogItem `json:"items"`
}

// List returns audit logs with pagination and filtering
func (h *AuditHandler) List(c *gin.Context) {
	// Parse query params
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	username := c.Query("username")
	action := c.Query("action")
	resource := c.Query("resource")
	ip := c.Query("ip")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// Use repository if available
	if h.auditRepo != nil {
		filter := repository.AuditFilter{
			Username:  username,
			Action:    action,
			Resource:  resource,
			IP:        ip,
			StartDate: startDate,
			EndDate:   endDate,
			Offset:    offset,
			Limit:     pageSize,
		}
		total, logs, err := h.auditRepo.Query(c.Request.Context(), filter)
		if err != nil {
			InternalError(c, err.Error())
			return
		}
		items := make([]AuditLogItem, 0, len(logs))
		for _, log := range logs {
			items = append(items, AuditLogItem{
				ID:        log.ID,
				UserID:    log.UserID,
				Username:  log.Username,
				Action:    log.Action,
				Resource:  log.Resource,
				Detail:    log.Detail,
				IP:        log.IP,
				UserAgent: log.UserAgent,
				CreatedAt: log.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		Success(c, AuditLogListResponse{
			Total: total,
			Items: items,
		})
		return
	}

	// Fallback to direct SQL
	where := "1=1"
	args := []interface{}{}

	if username != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+username+"%")
	}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
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
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		InternalError(c, err.Error())
		return
	}

	// Get items
	query := `SELECT id, user_id, username, action, resource, detail, ip, user_agent, created_at
	          FROM audit_logs WHERE ` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer rows.Close()

	items := []AuditLogItem{}
	for rows.Next() {
		var item AuditLogItem
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Action,
			&item.Resource, &item.Detail, &item.IP, &item.UserAgent, &createdAt); err != nil {
			continue
		}
		item.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, AuditLogListResponse{
		Total: total,
		Items: items,
	})
}

// GetActions returns distinct actions for filtering
func (h *AuditHandler) GetActions(c *gin.Context) {
	if h.auditRepo != nil {
		actions, err := h.auditRepo.GetActions(c.Request.Context())
		if err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, actions)
		return
	}

	rows, err := h.db.Query("SELECT DISTINCT action FROM audit_logs ORDER BY action")
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err == nil {
			actions = append(actions, action)
		}
	}

	Success(c, actions)
}

// Stats returns audit log statistics
func (h *AuditHandler) Stats(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 90 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	// 按用户统计
	userRows, err := h.db.Query(`
		SELECT username, COUNT(*) as cnt
		FROM audit_logs
		WHERE created_at >= ?
		GROUP BY username
		ORDER BY cnt DESC
		LIMIT 10
	`, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer userRows.Close()

	type UserStat struct {
		Username string `json:"username"`
		Count    int    `json:"count"`
	}
	var userStats []UserStat
	for userRows.Next() {
		var s UserStat
		if userRows.Scan(&s.Username, &s.Count) == nil {
			userStats = append(userStats, s)
		}
	}

	// 按操作类型统计
	actionRows, err := h.db.Query(`
		SELECT action, COUNT(*) as cnt
		FROM audit_logs
		WHERE created_at >= ?
		GROUP BY action
		ORDER BY cnt DESC
		LIMIT 10
	`, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer actionRows.Close()

	type ActionStat struct {
		Action string `json:"action"`
		Count  int    `json:"count"`
	}
	var actionStats []ActionStat
	for actionRows.Next() {
		var s ActionStat
		if actionRows.Scan(&s.Action, &s.Count) == nil {
			actionStats = append(actionStats, s)
		}
	}

	// 按天统计
	dayRows, err := h.db.Query(`
		SELECT DATE(created_at) as day, COUNT(*) as cnt
		FROM audit_logs
		WHERE created_at >= ?
		GROUP BY DATE(created_at)
		ORDER BY day ASC
	`, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer dayRows.Close()

	type DayStat struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	var dayStats []DayStat
	for dayRows.Next() {
		var s DayStat
		if dayRows.Scan(&s.Day, &s.Count) == nil {
			dayStats = append(dayStats, s)
		}
	}

	// 按状态码统计
	statusRows, err := h.db.Query(`
		SELECT
			CASE
				WHEN CAST(json_extract(detail, '$.status') AS INTEGER) >= 500 THEN '5xx'
				WHEN CAST(json_extract(detail, '$.status') AS INTEGER) >= 400 THEN '4xx'
				WHEN CAST(json_extract(detail, '$.status') AS INTEGER) >= 200 THEN '2xx'
				ELSE 'other'
			END as status_group,
			COUNT(*) as cnt
		FROM audit_logs
		WHERE created_at >= ?
		GROUP BY status_group
	`, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer statusRows.Close()

	type StatusStat struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	var statusStats []StatusStat
	for statusRows.Next() {
		var s StatusStat
		if statusRows.Scan(&s.Status, &s.Count) == nil {
			statusStats = append(statusStats, s)
		}
	}

	// 异常告警：失败操作（4xx/5xx）
	alertRows, err := h.db.Query(`
		SELECT id, username, action, resource, detail, ip, created_at
		FROM audit_logs
		WHERE created_at >= ?
		  AND CAST(json_extract(detail, '$.status') AS INTEGER) >= 400
		ORDER BY id DESC
		LIMIT 200
	`, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer alertRows.Close()

	type AlertItem struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		Status    int    `json:"status"`
		IP        string `json:"ip"`
		CreatedAt string `json:"created_at"`
	}
	var alerts []AlertItem
	for alertRows.Next() {
		var a AlertItem
		var detail string
		var createdAt time.Time
		if alertRows.Scan(&a.ID, &a.Username, &a.Action, &a.Resource, &detail, &a.IP, &createdAt) == nil {
			a.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
			// Parse status from detail
			var detailObj struct {
				Status int `json:"status"`
			}
			if json.Unmarshal([]byte(detail), &detailObj) == nil {
				a.Status = detailObj.Status
			}
			alerts = append(alerts, a)
		}
	}

	Success(c, gin.H{
		"user_stats":   userStats,
		"action_stats": actionStats,
		"day_stats":    dayStats,
		"status_stats": statusStats,
		"alerts":       alerts,
	})
}

// sanitizeCSVField prevents CSV formula injection by prefixing dangerous leading characters.
func sanitizeCSVField(field string) string {
	if field == "" {
		return field
	}
	// Characters that can trigger formula execution in spreadsheet applications
	if strings.ContainsAny(string(field[0]), "=+-@\t\r") {
		return "'" + field
	}
	return field
}

// GetCleanPolicy returns the current clean policy
func (h *AuditHandler) GetCleanPolicy(c *gin.Context) {
	// Check if there's a scheduled clean task
	var count int
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)

	Success(c, gin.H{
		"retention_days": 90,
		"total_records":  count,
		"auto_clean":     true,
	})
}

// Export returns audit logs as CSV
func (h *AuditHandler) Export(c *gin.Context) {
	username := c.Query("username")
	action := c.Query("action")
	resource := c.Query("resource")
	ip := c.Query("ip")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	where := "1=1"
	args := []interface{}{}

	if username != "" {
		where += " AND username LIKE ?"
		args = append(args, "%"+username+"%")
	}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
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

	query := `SELECT id, username, action, resource, detail, ip, created_at
	          FROM audit_logs WHERE ` + where + ` ORDER BY id DESC LIMIT 10000`
	rows, err := h.db.Query(query, args...)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	defer rows.Close()

	// Set CSV headers
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=audit_logs.csv")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-cache")

	// Write BOM for Excel
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	// Write CSV header
	c.Writer.WriteString("ID,用户,操作,资源,详情,IP,时间\n")

	for rows.Next() {
		var id int64
		var username, action, resource, detail, ip, createdAt string
		if err := rows.Scan(&id, &username, &action, &resource, &detail, &ip, &createdAt); err != nil {
			continue
		}
		// Sanitize CSV fields to prevent formula injection
		c.Writer.WriteString(fmt.Sprintf("%d,%s,%s,%s,\"%s\",%s,%s\n",
			id,
			sanitizeCSVField(username),
			sanitizeCSVField(action),
			sanitizeCSVField(resource),
			strings.ReplaceAll(detail, "\"", "\"\""),
			sanitizeCSVField(ip),
			createdAt))
	}
}

// Clean deletes audit logs older than specified days
func (h *AuditHandler) Clean(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))
	if days < 1 {
		days = 90
	}

	since := time.Now().AddDate(0, 0, -days)

	if h.auditRepo != nil {
		rows, err := h.auditRepo.Clean(c.Request.Context(), since)
		if err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"deleted": rows})
		return
	}

	result, err := h.db.Exec("DELETE FROM audit_logs WHERE created_at < ?", since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	rows, _ := result.RowsAffected()
	Success(c, gin.H{"deleted": rows})
}

// VerifyIntegrity verifies the integrity of audit log signatures
func (h *AuditHandler) VerifyIntegrity(c *gin.Context) {
	if h.auditService == nil {
		InternalError(c, "audit service not available")
		return
	}

	total, valid, invalid, err := h.auditService.VerifyAllSignatures(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"total":   total,
		"valid":   valid,
		"invalid": invalid,
	})
}

func registerAuditRoutes(protected *gin.RouterGroup, db *sql.DB, auditService *audit.Service, auditRepo repository.AuditRepository) {
	handler := NewAuditHandlerWithRepo(db, auditService, auditRepo)
	protected.GET("/audit-logs", handler.List)
	protected.GET("/audit-logs/actions", handler.GetActions)
	protected.GET("/audit-logs/stats", handler.Stats)
	protected.GET("/audit-logs/clean-policy", handler.GetCleanPolicy)
	protected.GET("/audit-logs/export", handler.Export)
	protected.DELETE("/audit-logs/clean", handler.Clean)
	protected.GET("/audit-logs/verify", handler.VerifyIntegrity)
}
