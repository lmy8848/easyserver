package service

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

type AuditService struct {
	db *sql.DB
}

func NewAuditService(db *sql.DB) *AuditService {
	return &AuditService{db: db}
}

// LogOperation logs a server-level operation
func (s *AuditService) LogOperation(userID int64, username, action, resource, detail, ip, userAgent string) {
	go func() {
		detailJSON, _ := json.Marshal(map[string]interface{}{
			"detail":    detail,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, action, resource, string(detailJSON), ip, userAgent, time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log operation: %v", err)
		}
	}()
}

// LogTerminalCommand logs a command executed in the terminal
func (s *AuditService) LogTerminalCommand(userID int64, username, sessionID, command, ip string) {
	go func() {
		detail := map[string]interface{}{
			"session_id": sessionID,
			"command":    command,
			"timestamp":  time.Now().Format(time.RFC3339),
		}
		detailJSON, _ := json.Marshal(detail)
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, "TERMINAL", "/terminal/"+sessionID, string(detailJSON), ip, "WebTerminal", time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log terminal command: %v", err)
		}
	}()
}

// LogFileOperation logs a file operation
func (s *AuditService) LogFileOperation(userID int64, username, action, filePath, ip, userAgent string) {
	go func() {
		detail := map[string]interface{}{
			"file_path":  filePath,
			"action":     action,
			"timestamp":  time.Now().Format(time.RFC3339),
		}
		detailJSON, _ := json.Marshal(detail)
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, "FILE_"+action, filePath, string(detailJSON), ip, userAgent, time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log file operation: %v", err)
		}
	}()
}

// LogSecurityEvent logs a security event
func (s *AuditService) LogSecurityEvent(username, action, detail, ip, userAgent string) {
	go func() {
		detailJSON, _ := json.Marshal(map[string]interface{}{
			"detail":    detail,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			0, username, "SECURITY_"+action, "/auth", string(detailJSON), ip, userAgent, time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log security event: %v", err)
		}
	}()
}

// LogSystemEvent logs a system event
func (s *AuditService) LogSystemEvent(action, detail string) {
	go func() {
		detailJSON, _ := json.Marshal(map[string]interface{}{
			"detail":    detail,
			"timestamp": time.Now().Format(time.RFC3339),
		})
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			0, "system", "SYSTEM_"+action, "/system", string(detailJSON), "127.0.0.1", "EasyServer", time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log system event: %v", err)
		}
	}()
}

// LogServiceOperation logs a service operation
func (s *AuditService) LogServiceOperation(userID int64, username, action, serviceName, ip, userAgent string) {
	go func() {
		detail := map[string]interface{}{
			"service":   serviceName,
			"action":    action,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		detailJSON, _ := json.Marshal(detail)
		_, err := s.db.Exec(
			`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip, user_agent, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, username, "SERVICE_"+action, "/services/"+serviceName, string(detailJSON), ip, userAgent, time.Now(),
		)
		if err != nil {
			log.Printf("audit: failed to log service operation: %v", err)
		}
	}()
}

// GetAuditLogs returns audit logs with filtering
func (s *AuditService) GetAuditLogs(page, pageSize int, username, action, resource, ip, startDate, endDate string) (int64, []map[string]interface{}, error) {
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
	s.db.QueryRow(countQuery, args...).Scan(&total)

	// Get items
	offset := (page - 1) * pageSize
	query := `SELECT id, user_id, username, action, resource, detail, ip, user_agent, created_at
	          FROM audit_logs WHERE ` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := s.db.Query(query, args...)
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
