package http

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/database"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers database management routes.
func RegisterRoutes(protected *gin.RouterGroup, svc *database.Service) {
	instanceHandler := NewInstanceHandler(svc)
	dbHandler := NewDatabaseHandler(svc)
	userHandler := NewUserHandler(svc)
	backupHandler := NewBackupHandler(svc)
	redisHandler := NewRedisHandler(svc)

	// Instance lifecycle, scoped by database type (?dbtype= query; POST reads it
	// from the body).
	protected.GET("/db/instances", httpx.H(instanceHandler.ListInstances))
	protected.POST("/db/instances", httpx.H(instanceHandler.CreateInstance))
	protected.GET("/db/docker-tags", httpx.H(instanceHandler.ListDockerTags))
	// Installs run without a database row until they finish; the install id is
	// the container id. The log endpoint streams one install's log via SSE.
	protected.GET("/db/installs/:iid/log", httpx.H(instanceHandler.InstallLogStream))
	protected.POST("/db/installs/:iid/cancel", httpx.H(instanceHandler.CancelInstall))
	protected.DELETE("/db/instances/:iid", httpx.H(instanceHandler.UninstallInstance))
	protected.POST("/db/instances/:iid/reset-password", httpx.H(instanceHandler.ResetAdminPassword))
	protected.POST("/db/instances/:iid/start", httpx.H(instanceHandler.StartInstance))
	protected.POST("/db/instances/:iid/stop", httpx.H(instanceHandler.StopInstance))
	protected.POST("/db/instances/:iid/restart", httpx.H(instanceHandler.RestartInstance))
	protected.GET("/db/instances/:iid/logs", httpx.H(instanceHandler.GetInstanceLogs))
	protected.GET("/db/instances/:iid/config", httpx.H(instanceHandler.GetInstanceConfig))
	protected.PUT("/db/instances/:iid/config", httpx.H(instanceHandler.SaveInstanceConfig))

	// Logical databases, scoped by instance. Databases are live database state —
	// the db name is the identifier, there is no persisted db id.
	protected.GET("/db/instances/:iid/databases", httpx.H(dbHandler.ListDatabases))
	protected.POST("/db/instances/:iid/databases", httpx.H(dbHandler.CreateDatabase))
	protected.DELETE("/db/instances/:iid/databases/:dbname", httpx.H(dbHandler.DeleteDatabase))

	// DB Users, scoped by instance (username, plus host for MySQL).
	protected.GET("/db/instances/:iid/users", httpx.H(userHandler.ListDBUsers))
	protected.POST("/db/instances/:iid/users", httpx.H(userHandler.CreateDBUser))
	protected.DELETE("/db/instances/:iid/users/:username", httpx.H(userHandler.DeleteDBUser))
	protected.POST("/db/instances/:iid/users/:username/grant", httpx.H(userHandler.GrantPrivileges))
	protected.POST("/db/instances/:iid/users/:username/password", httpx.H(userHandler.ResetPassword))

	// Database introspection (instance-scoped, addressed by database name)
	protected.GET("/db/instances/:iid/databases/:dbname/tables", httpx.H(dbHandler.ListTables))
	protected.GET("/db/instances/:iid/databases/:dbname/describe", httpx.H(dbHandler.DescribeTable))
	protected.GET("/db/instances/:iid/databases/:dbname/query", httpx.H(dbHandler.QueryTable))
	protected.POST("/db/instances/:iid/databases/:dbname/execute", httpx.H(dbHandler.ExecuteSQL))
	protected.POST("/db/instances/:iid/databases/:dbname/insert", httpx.H(dbHandler.InsertRecord))
	protected.POST("/db/instances/:iid/databases/:dbname/update", httpx.H(dbHandler.UpdateRecord))
	protected.POST("/db/instances/:iid/databases/:dbname/delete", httpx.H(dbHandler.DeleteRecord))

	// Table management
	protected.POST("/db/instances/:iid/databases/:dbname/tables", httpx.H(dbHandler.CreateTable))
	protected.DELETE("/db/instances/:iid/databases/:dbname/tables", httpx.H(dbHandler.DropTable))

	// Database backup
	protected.POST("/db/instances/:iid/databases/:dbname/backup", httpx.H(backupHandler.CreateBackup))
	protected.GET("/db/instances/:iid/databases/:dbname/backups", httpx.H(backupHandler.ListBackups))
	protected.GET("/db/backups/:bid/download", httpx.H(backupHandler.DownloadBackup))
	protected.POST("/db/backups/:bid/restore", httpx.H(backupHandler.RestoreBackup))
	protected.GET("/db/backups/:bid/restore-status", httpx.H(backupHandler.RestoreStatus))
	protected.GET("/db/backups/:bid/status", httpx.H(backupHandler.BackupStatusStream))
	protected.DELETE("/db/backups/:bid", httpx.H(backupHandler.DeleteBackup))

	// Redis key browser (instance-scoped, addressed by logical DB index)
	protected.GET("/db/redis/instances/:iid/databases-count", httpx.H(redisHandler.DBCount))
	protected.GET("/db/redis/instances/:iid/keys", httpx.H(redisHandler.ScanKeys))
	protected.GET("/db/redis/instances/:iid/value", httpx.H(redisHandler.GetValue))
	protected.POST("/db/redis/instances/:iid/value", httpx.H(redisHandler.SetValue))
	protected.POST("/db/redis/instances/:iid/del", httpx.H(redisHandler.DelKeys))
	protected.POST("/db/redis/instances/:iid/expire", httpx.H(redisHandler.Expire))
	protected.POST("/db/redis/instances/:iid/persist", httpx.H(redisHandler.Persist))
	protected.POST("/db/redis/instances/:iid/flushdb", httpx.H(redisHandler.FlushDB))
}

// InstanceHandler handles instance lifecycle endpoints, scoped by database type.
type InstanceHandler struct {
	svc *database.Service
}

func NewInstanceHandler(svc *database.Service) *InstanceHandler {
	return &InstanceHandler{svc: svc}
}

func parseDBType(c *gin.Context) (database.DBType, error) {
	dbType := database.DBType(c.Query("dbtype"))
	if !database.IsValidDBType(dbType) {
		return "", errx.BadRequest("无效的数据库类型")
	}
	return dbType, nil
}

func parseIID(c *gin.Context) (int64, error) {
	iid, err := strconv.ParseInt(c.Param("iid"), 10, 64)
	if err != nil {
		return 0, errx.BadRequest("无效的实例ID")
	}
	return iid, nil
}

func (h *InstanceHandler) ListInstances(c *gin.Context) (any, error) {
	dbType, err := parseDBType(c)
	if err != nil {
		return nil, err
	}
	// Refresh live container status before returning — instance status is never
	// persisted as authoritative; the runtime is the source of truth.
	h.svc.RefreshStatus(c.Request.Context(), dbType)
	instances, err := h.svc.ListInstances(c.Request.Context(), dbType)
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// ListDockerTags returns available Docker image tags for a database type.
func (h *InstanceHandler) ListDockerTags(c *gin.Context) (any, error) {
	dbType, err := parseDBType(c)
	if err != nil {
		return nil, err
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	tags, total, err := h.svc.ListDockerTags(c.Request.Context(), dbType, page, pageSize)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"tags":      tags,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, nil
}

// InstallLogStream streams installation progress for a pending/failed install via SSE.
func (h *InstanceHandler) InstallLogStream(c *gin.Context) (any, error) {
	installID := c.Param("iid")
	if installID == "" {
		return nil, errx.BadRequest("无效的安装ID")
	}
	// SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Lines and completion are both sent as default `data:` events with a JSON
	// envelope {type, ...} — the front-end's EventSource onmessage parses them.
	send := func(payload map[string]string) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		c.Writer.Flush()
	}

	// Task may still be alive (installing) or already finished (done/errored) —
	// finished failed/canceled tasks are kept so their log stays replayable; a
	// succeeded task is cleaned up on completion (its log already flushed to the
	// client). If it's gone entirely, the server restarted mid-install and the
	// in-memory log is lost.
	tk, ok := h.svc.InstallTask(installID)
	if !ok {
		send(map[string]string{"type": "done", "error": "安装日志已丢失（服务可能已重启或安装已完成），无法查看"})
		return nil, nil
	}
	log := tk.Log()

	cursor := 0
	for {
		select {
		case <-c.Request.Context().Done():
			return nil, nil
		default:
		}
		lines, next := log.Tail(cursor)
		for _, line := range lines {
			send(map[string]string{"type": "line", "text": line})
		}
		cursor = next

		select {
		case <-tk.Done():
			// Flush anything that landed between the tail above and completion.
			if lines, _ := log.Tail(cursor); len(lines) > 0 {
				for _, line := range lines {
					send(map[string]string{"type": "line", "text": line})
				}
			}
			errMsg := ""
			if tk.Err() != nil {
				errMsg = tk.Err().Error()
			}
			send(map[string]string{"type": "done", "error": errMsg})
			return nil, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// CancelInstall aborts an in-flight install (image pull or provisioning).
func (h *InstanceHandler) CancelInstall(c *gin.Context) (any, error) {
	iid := c.Param("iid")
	if iid == "" {
		return nil, errx.BadRequest("无效的安装ID")
	}
	if err := h.svc.CancelInstall(iid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *InstanceHandler) CreateInstance(c *gin.Context) (any, error) {
	var req database.CreateDBInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	if !database.IsValidDBType(req.DBType) {
		return nil, errx.BadRequest("无效的数据库类型")
	}
	middleware.AuditSummary(c, "创建数据库实例 "+req.Version)
	instance, err := h.svc.CreateInstance(c.Request.Context(), req.DBType, &req)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func (h *InstanceHandler) UninstallInstance(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	// Retain the data volume by default (the container's data persists and the
	// instance can be re-installed onto it); pass ?purge=1 to also delete it.
	purge := c.Query("purge") == "1"
	middleware.AuditSummary(c, "卸载数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.UninstallInstance(c.Request.Context(), iid, purge); err != nil {
		return nil, err
	}
	return gin.H{"message": "已卸载"}, nil
}

func (h *InstanceHandler) ResetAdminPassword(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	password, err := h.svc.ResetAdminPassword(c.Request.Context(), iid)
	if err != nil {
		return nil, err
	}
	return gin.H{"admin_password": password}, nil
}

func (h *InstanceHandler) StartInstance(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "启动数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.StartInstance(c.Request.Context(), iid); err != nil {
		return nil, err
	}
	return gin.H{"status": "running"}, nil
}

func (h *InstanceHandler) StopInstance(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "停止数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.StopInstance(c.Request.Context(), iid); err != nil {
		return nil, err
	}
	return gin.H{"status": "stopped"}, nil
}

func (h *InstanceHandler) RestartInstance(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "重启数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.RestartInstance(c.Request.Context(), iid); err != nil {
		return nil, err
	}
	return gin.H{"status": "running"}, nil
}

func (h *InstanceHandler) GetInstanceLogs(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.svc.GetInstanceServiceLogs(c.Request.Context(), iid, lines)
	if err != nil {
		return nil, err
	}
	return gin.H{"logs": logs}, nil
}

func (h *InstanceHandler) GetInstanceConfig(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	view, err := h.svc.GetInstanceConfig(c.Request.Context(), iid)
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (h *InstanceHandler) SaveInstanceConfig(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	var req struct {
		Params map[string]string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	if err := h.svc.SaveInstanceConfig(c.Request.Context(), iid, req.Params); err != nil {
		return nil, err
	}
	return gin.H{"message": "配置已保存，重启后生效"}, nil
}

// DatabaseHandler handles database CRUD, introspection, and table management endpoints.
type DatabaseHandler struct {
	svc *database.Service
}

func NewDatabaseHandler(svc *database.Service) *DatabaseHandler {
	return &DatabaseHandler{
		svc: svc,
	}
}

// --- Database CRUD ---

func (h *DatabaseHandler) ListDatabases(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbs, err := h.svc.ListDatabases(c.Request.Context(), iid)
	if err != nil {
		return nil, err
	}
	return dbs, nil
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	var req database.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "创建数据库 "+req.Name)
	db, err := h.svc.CreateDatabase(c.Request.Context(), iid, &req)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}
	middleware.AuditSummary(c, "删除数据库 "+dbName)
	if err := h.svc.DeleteDatabase(c.Request.Context(), iid, dbName); err != nil {
		return nil, err
	}
	return nil, nil
}

// --- Database introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	tables, err := h.svc.ListTables(c.Request.Context(), iid, dbName)
	if err != nil {
		return nil, err
	}

	return tables, nil
}

func (h *DatabaseHandler) DescribeTable(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}
	tableName := c.Query("table")
	if tableName == "" {
		return nil, errx.BadRequest("表名不能为空")
	}

	result, err := h.svc.DescribeTable(c.Request.Context(), iid, dbName, tableName)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (h *DatabaseHandler) QueryTable(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}
	tableName := c.Query("table")
	if tableName == "" {
		return nil, errx.BadRequest("表名不能为空")
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.svc.QueryTable(c.Request.Context(), iid, dbName, tableName, page, pageSize)
	if err != nil {
		return nil, err
	}

	return gin.H{
		"headers":   result.Headers,
		"rows":      result.Rows,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	}, nil
}

func (h *DatabaseHandler) ExecuteSQL(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "执行SQL (数据库: "+dbName+")")

	result, err := h.svc.ExecuteSQL(c.Request.Context(), iid, dbName, req.SQL)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (h *DatabaseHandler) InsertRecord(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	var req struct {
		Table string         `json:"table" binding:"required"`
		Data  map[string]any `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "插入记录到表 "+req.Table)

	result, err := h.svc.InsertRecord(c.Request.Context(), iid, dbName, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	return result, nil
}

func (h *DatabaseHandler) UpdateRecord(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	var req struct {
		Table      string         `json:"table" binding:"required"`
		Data       map[string]any `json:"data" binding:"required"`
		PrimaryKey string         `json:"primary_key" binding:"required"`
		PrimaryVal any            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "更新表 "+req.Table+" 记录")

	result, err := h.svc.UpdateRecord(c.Request.Context(), iid, dbName, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	return result, nil
}

func (h *DatabaseHandler) DeleteRecord(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	var req struct {
		Table      string `json:"table" binding:"required"`
		PrimaryKey string `json:"primary_key" binding:"required"`
		PrimaryVal any    `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "删除表 "+req.Table+" 记录")

	result, err := h.svc.DeleteRecord(c.Request.Context(), iid, dbName, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	return result, nil
}

// --- Table management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	var req struct {
		Name      string `json:"name" binding:"required"`
		Charset   string `json:"charset"`
		Collation string `json:"collation"`
		Columns   []struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			Length       string `json:"length"`
			Nullable     bool   `json:"nullable"`
			IsPrimary    bool   `json:"is_primary"`
			AutoIncr     bool   `json:"auto_incr"`
			Unique       bool   `json:"unique"`
			DefaultValue string `json:"default_value"`
		} `json:"columns" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "创建表 "+req.Name)

	var columns []database.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, database.TableColumn{
			Name:         col.Name,
			Type:         col.Type,
			Length:       col.Length,
			Nullable:     col.Nullable,
			IsPrimary:    col.IsPrimary,
			AutoIncr:     col.AutoIncr,
			Unique:       col.Unique,
			DefaultValue: col.DefaultValue,
		})
	}

	if err := h.svc.CreateTable(c.Request.Context(), iid, dbName, req.Name, columns, req.Charset, req.Collation); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			return nil, errx.BadRequest("%w", err)
		}
		return nil, err
	}

	return gin.H{"message": "表已创建", "name": req.Name}, nil
}

func (h *DatabaseHandler) DropTable(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	tableName := c.Query("table")
	if tableName == "" {
		return nil, errx.BadRequest("表名不能为空")
	}
	middleware.AuditSummary(c, "删除表 "+tableName)

	if err := h.svc.DropTable(c.Request.Context(), iid, dbName, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			return nil, errx.BadRequest("%w", err)
		}
		return nil, err
	}

	return gin.H{"message": "表已删除", "name": tableName}, nil
}

// UserHandler handles DB user management endpoints.
type UserHandler struct {
	svc *database.Service
}

func NewUserHandler(svc *database.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) ListDBUsers(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	users, err := h.svc.ListDBUsers(c.Request.Context(), iid)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (h *UserHandler) CreateDBUser(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	var req database.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "创建数据库用户 "+req.Username)
	user, err := h.svc.CreateDBUser(c.Request.Context(), iid, &req)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (h *UserHandler) DeleteDBUser(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	username := c.Param("username")
	if username == "" {
		return nil, errx.BadRequest("无效的用户名")
	}
	host := c.DefaultQuery("host", "%")
	middleware.AuditSummary(c, "删除数据库用户 "+username)
	if err := h.svc.DeleteDBUser(c.Request.Context(), iid, username, host); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *UserHandler) GrantPrivileges(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	username := c.Param("username")
	if username == "" {
		return nil, errx.BadRequest("无效的用户名")
	}
	host := c.DefaultQuery("host", "%")
	var req database.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "授权数据库用户 "+username)
	if err := h.svc.GrantPrivileges(c.Request.Context(), iid, username, host, &req); err != nil {
		return nil, err
	}
	return gin.H{"message": "权限已授予"}, nil
}

func (h *UserHandler) ResetPassword(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	username := c.Param("username")
	if username == "" {
		return nil, errx.BadRequest("无效的用户名")
	}
	host := c.DefaultQuery("host", "%")
	var req database.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "重置数据库用户密码 "+username)
	if err := h.svc.ResetPassword(c.Request.Context(), iid, username, host, req.Password); err != nil {
		return nil, err
	}
	return gin.H{"message": "密码重置成功"}, nil
}

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	svc *database.Service
}

func NewBackupHandler(svc *database.Service) *BackupHandler {
	return &BackupHandler{svc: svc}
}

func (h *BackupHandler) CreateBackup(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}
	middleware.AuditSummary(c, "创建数据库备份 "+dbName)

	instance, err := h.svc.GetInstance(c.Request.Context(), iid)
	if err != nil || instance == nil {
		return nil, errx.NotFound("数据库实例不存在")
	}

	backup, err := h.svc.CreateBackup(c.Request.Context(), iid, dbName, instance.DBType)
	if err != nil {
		return nil, err
	}

	return backup, nil
}

func (h *BackupHandler) ListBackups(c *gin.Context) (any, error) {
	iid, err := parseIID(c)
	if err != nil {
		return nil, err
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		return nil, errx.BadRequest("无效的数据库名")
	}

	backups, err := h.svc.ListBackups(c.Request.Context(), iid, dbName)
	if err != nil {
		return nil, err
	}

	if backups == nil {
		backups = []database.DBBackup{}
	}

	return backups, nil
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) (any, error) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的备份ID")
	}

	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		return nil, errx.NotFound("备份不存在")
	}

	if backup.Status != "success" {
		return nil, errx.BadRequest("备份未完成")
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		return nil, errx.NotFound("备份文件不存在")
	}

	c.File(backup.FilePath)
	return nil, nil
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) (any, error) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的备份ID")
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		return nil, errx.BadRequest("请确认恢复，设置 {\"confirm\": true}")
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		return nil, errx.NotFound("备份不存在")
	}
	middleware.AuditSummary(c, "恢复数据库备份 "+backup.DatabaseName)

	if err := h.svc.RestoreBackup(c.Request.Context(), bid, backup.DBType); err != nil {
		return nil, err
	}

	return gin.H{"message": "恢复已开始，请轮询恢复状态"}, nil
}

func (h *BackupHandler) RestoreStatus(c *gin.Context) (any, error) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的备份ID")
	}
	// SSE：恢复是内存异步任务，终态到达即推送并关闭连接，前端无需轮询。
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	send := func(payload map[string]any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		c.Writer.Flush()
	}

	// 恢复任务不存在（服务重启内存态丢失，或从未发起）→ 立即终结，语义与
	// 原 404 一致，但走 SSE done 帧（EventSource 拿不到状态码）。
	_, ok := h.svc.GetRestoreStatus(c.Request.Context(), bid)
	if !ok {
		send(map[string]any{"type": "done", "error": "恢复状态已丢失（服务可能已重启），请手动确认数据"})
		return nil, nil
	}

	// 任务运行中保持连接；10s 心跳防反向代理超时断连，500ms 轮询检查终态。
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return nil, nil
		case <-heartbeat.C:
			send(map[string]any{"type": "running"})
			continue
		case <-time.After(500 * time.Millisecond):
		}
		status, ok := h.svc.GetRestoreStatus(c.Request.Context(), bid)
		if !ok {
			send(map[string]any{"type": "done", "error": "恢复状态已丢失（服务可能已重启），请手动确认数据"})
			return nil, nil
		}
		if status.Status != "running" {
			send(map[string]any{"type": "done", "status": status.Status, "error": status.Error})
			return nil, nil
		}
	}
}

// BackupStatusStream 订阅单个备份的状态流（SSE）：等备份内存任务完成信号，
// 终态到达即推送 done 帧并关闭。不做轮询 —— 任务 Done() 即完成。
func (h *BackupHandler) BackupStatusStream(c *gin.Context) (any, error) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的备份ID")
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	send := func(payload map[string]any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		c.Writer.Flush()
	}

	done, err := h.svc.WaitBackup(bid)
	if err != nil {
		send(map[string]any{"type": "done", "error": "备份状态已丢失"})
		return nil, nil
	}
	// 任务运行中保持连接；10s 心跳防反向代理超时断连。终态由 Done() 通知。
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return nil, nil
		case <-heartbeat.C:
			send(map[string]any{"type": "running"})
		case <-done:
			backup, gerr := h.svc.GetBackup(c.Request.Context(), bid)
			if gerr != nil {
				send(map[string]any{"type": "done", "error": "备份状态已丢失"})
				return nil, nil
			}
			send(map[string]any{"type": "done", "status": backup.Status, "error": backup.ErrorMessage})
			return nil, nil
		}
	}
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) (any, error) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的备份ID")
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		return nil, errx.NotFound("备份不存在")
	}
	middleware.AuditSummary(c, "删除数据库备份 "+backup.DatabaseName)

	if err := h.svc.DeleteBackup(c.Request.Context(), bid); err != nil {
		return nil, err
	}

	return gin.H{"message": "备份已删除"}, nil
}
