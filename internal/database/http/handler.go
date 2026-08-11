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
	"easyserver/internal/infra/apperror"

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
	protected.GET("/db/instances", instanceHandler.ListInstances)
	protected.POST("/db/instances", instanceHandler.CreateInstance)
	protected.GET("/db/docker-tags", instanceHandler.ListDockerTags)
	// Installs run without a database row until they finish; the install id is
	// the container id. The log endpoint streams one install's log via SSE.
	protected.GET("/db/installs/:iid/log", instanceHandler.InstallLogStream)
	protected.POST("/db/installs/:iid/cancel", instanceHandler.CancelInstall)
	protected.DELETE("/db/instances/:iid", instanceHandler.UninstallInstance)
	protected.POST("/db/instances/:iid/reset-password", instanceHandler.ResetAdminPassword)
	protected.POST("/db/instances/:iid/start", instanceHandler.StartInstance)
	protected.POST("/db/instances/:iid/stop", instanceHandler.StopInstance)
	protected.POST("/db/instances/:iid/restart", instanceHandler.RestartInstance)
	protected.GET("/db/instances/:iid/logs", instanceHandler.GetInstanceLogs)
	protected.GET("/db/instances/:iid/config", instanceHandler.GetInstanceConfig)
	protected.PUT("/db/instances/:iid/config", instanceHandler.SaveInstanceConfig)

	// Logical databases, scoped by instance. Databases are live database state —
	// the db name is the identifier, there is no persisted db id.
	protected.GET("/db/instances/:iid/databases", dbHandler.ListDatabases)
	protected.POST("/db/instances/:iid/databases", dbHandler.CreateDatabase)
	protected.DELETE("/db/instances/:iid/databases/:dbname", dbHandler.DeleteDatabase)

	// DB Users, scoped by instance (username, plus host for MySQL).
	protected.GET("/db/instances/:iid/users", userHandler.ListDBUsers)
	protected.POST("/db/instances/:iid/users", userHandler.CreateDBUser)
	protected.DELETE("/db/instances/:iid/users/:username", userHandler.DeleteDBUser)
	protected.POST("/db/instances/:iid/users/:username/grant", userHandler.GrantPrivileges)

	// Database introspection (instance-scoped, addressed by database name)
	protected.GET("/db/instances/:iid/databases/:dbname/tables", dbHandler.ListTables)
	protected.GET("/db/instances/:iid/databases/:dbname/describe", dbHandler.DescribeTable)
	protected.GET("/db/instances/:iid/databases/:dbname/query", dbHandler.QueryTable)
	protected.POST("/db/instances/:iid/databases/:dbname/execute", dbHandler.ExecuteSQL)
	protected.POST("/db/instances/:iid/databases/:dbname/insert", dbHandler.InsertRecord)
	protected.POST("/db/instances/:iid/databases/:dbname/update", dbHandler.UpdateRecord)
	protected.POST("/db/instances/:iid/databases/:dbname/delete", dbHandler.DeleteRecord)

	// Table management
	protected.POST("/db/instances/:iid/databases/:dbname/tables", dbHandler.CreateTable)
	protected.DELETE("/db/instances/:iid/databases/:dbname/tables", dbHandler.DropTable)

	// Database backup
	protected.POST("/db/instances/:iid/databases/:dbname/backup", backupHandler.CreateBackup)
	protected.GET("/db/instances/:iid/databases/:dbname/backups", backupHandler.ListBackups)
	protected.GET("/db/backups/:bid/download", backupHandler.DownloadBackup)
	protected.POST("/db/backups/:bid/restore", backupHandler.RestoreBackup)
	protected.GET("/db/backups/:bid/restore-status", backupHandler.RestoreStatus)
	protected.DELETE("/db/backups/:bid", backupHandler.DeleteBackup)

	// Redis key browser (instance-scoped, addressed by logical DB index)
	protected.GET("/db/redis/instances/:iid/dbs", redisHandler.ListDBs)
	protected.GET("/db/redis/instances/:iid/keys", redisHandler.ScanKeys)
	protected.GET("/db/redis/instances/:iid/value", redisHandler.GetValue)
	protected.POST("/db/redis/instances/:iid/value", redisHandler.SetValue)
	protected.POST("/db/redis/instances/:iid/del", redisHandler.DelKeys)
	protected.POST("/db/redis/instances/:iid/expire", redisHandler.Expire)
	protected.POST("/db/redis/instances/:iid/persist", redisHandler.Persist)
	protected.POST("/db/redis/instances/:iid/flushdb", redisHandler.FlushDB)
}

// InstanceHandler handles instance lifecycle endpoints, scoped by database type.
type InstanceHandler struct {
	svc *database.Service
}

func NewInstanceHandler(svc *database.Service) *InstanceHandler {
	return &InstanceHandler{svc: svc}
}

func parseDBType(c *gin.Context) (database.DBType, bool) {
	dbType := database.DBType(c.Query("dbtype"))
	if !database.IsValidDBType(dbType) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库类型"))
		return "", false
	}
	return dbType, true
}

func parseIID(c *gin.Context) (int64, bool) {
	iid, err := strconv.ParseInt(c.Param("iid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的实例ID"))
		return 0, false
	}
	return iid, true
}

func (h *InstanceHandler) ListInstances(c *gin.Context) {
	dbType, ok := parseDBType(c)
	if !ok {
		return
	}
	// Refresh live container status before returning — instance status is never
	// persisted as authoritative; the runtime is the source of truth.
	h.svc.RefreshStatus(c.Request.Context(), dbType)
	instances, err := h.svc.ListInstances(c.Request.Context(), dbType)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, instances)
}

func (h *InstanceHandler) ListDockerTags(c *gin.Context) {
	dbType, ok := parseDBType(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	tags, total, err := h.svc.ListDockerTags(c.Request.Context(), dbType, page, pageSize)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"items": tags, "total": total, "page": page, "page_size": pageSize})
}

func (h *InstanceHandler) InstallLogStream(c *gin.Context) {
	installID := c.Param("iid")
	if installID == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的安装ID"))
		return
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
		return
	}
	log := tk.Log()

	cursor := 0
	for {
		select {
		case <-c.Request.Context().Done():
			return
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
			if lines, next := log.Tail(cursor); len(lines) > 0 {
				for _, line := range lines {
					send(map[string]string{"type": "line", "text": line})
				}
				cursor = next
			}
			errMsg := ""
			if tk.Err() != nil {
				errMsg = tk.Err().Error()
			}
			send(map[string]string{"type": "done", "error": errMsg})
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// CancelInstall aborts an in-flight install (image pull or provisioning).
func (h *InstanceHandler) CancelInstall(c *gin.Context) {
	iid := c.Param("iid")
	if iid == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的安装ID"))
		return
	}
	if err := h.svc.CancelInstall(iid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

func (h *InstanceHandler) CreateInstance(c *gin.Context) {
	var req database.CreateDBInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if !database.IsValidDBType(req.DBType) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库类型"))
		return
	}
	middleware.AuditSummary(c, "创建数据库实例 "+req.Version)
	instance, err := h.svc.CreateInstance(c.Request.Context(), req.DBType, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, instance)
}

func (h *InstanceHandler) UninstallInstance(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	// Retain the data volume by default (the container's data persists and the
	// instance can be re-installed onto it); pass ?purge=1 to also delete it.
	purge := c.Query("purge") == "1"
	middleware.AuditSummary(c, "卸载数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.UninstallInstance(c.Request.Context(), iid, purge); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已卸载"})
}

func (h *InstanceHandler) ResetAdminPassword(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	password, err := h.svc.ResetAdminPassword(c.Request.Context(), iid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"admin_password": password})
}

func (h *InstanceHandler) StartInstance(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	middleware.AuditSummary(c, "启动数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.StartInstance(c.Request.Context(), iid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "running"})
}

func (h *InstanceHandler) StopInstance(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	middleware.AuditSummary(c, "停止数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.StopInstance(c.Request.Context(), iid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "stopped"})
}

func (h *InstanceHandler) RestartInstance(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	middleware.AuditSummary(c, "重启数据库实例 #"+strconv.FormatInt(iid, 10))
	if err := h.svc.RestartInstance(c.Request.Context(), iid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "running"})
}

func (h *InstanceHandler) GetInstanceLogs(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.svc.GetInstanceServiceLogs(c.Request.Context(), iid, lines)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"logs": logs})
}

func (h *InstanceHandler) GetInstanceConfig(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	view, err := h.svc.GetInstanceConfig(c.Request.Context(), iid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, view)
}

func (h *InstanceHandler) SaveInstanceConfig(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req struct {
		Params map[string]string `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if err := h.svc.SaveInstanceConfig(c.Request.Context(), iid, req.Params); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "配置已保存，重启后生效"})
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

func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbs, err := h.svc.ListDatabases(c.Request.Context(), iid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, dbs)
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req database.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库 "+req.Name)
	db, err := h.svc.CreateDatabase(c.Request.Context(), iid, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, db)
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}
	middleware.AuditSummary(c, "删除数据库 "+dbName)
	if err := h.svc.DeleteDatabase(c.Request.Context(), iid, dbName); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

// --- Database introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	tables, err := h.svc.ListTables(c.Request.Context(), iid, dbName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, tables)
}

func (h *DatabaseHandler) DescribeTable(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}

	result, err := h.svc.DescribeTable(c.Request.Context(), iid, dbName, tableName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) QueryTable(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.svc.QueryTable(c.Request.Context(), iid, dbName, tableName, page, pageSize)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"headers":   result.Headers,
		"rows":      result.Rows,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

func (h *DatabaseHandler) ExecuteSQL(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "执行SQL (数据库: "+dbName+")")

	result, err := h.svc.ExecuteSQL(c.Request.Context(), iid, dbName, req.SQL)
	if err != nil {
		c.Error(apperror.ErrNotFound.Wrap(err))
		return
	}

	if !result.Success {
		httpx.Success(c, gin.H{"success": false, "error": result.Error})
		return
	}

	httpx.Success(c, gin.H{"success": true, "output": result.Output})
}

func (h *DatabaseHandler) InsertRecord(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	var req struct {
		Table string                 `json:"table" binding:"required"`
		Data  map[string]interface{} `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "插入记录到表 "+req.Table)

	result, err := h.svc.InsertRecord(c.Request.Context(), iid, dbName, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) UpdateRecord(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	var req struct {
		Table      string                 `json:"table" binding:"required"`
		Data       map[string]interface{} `json:"data" binding:"required"`
		PrimaryKey string                 `json:"primary_key" binding:"required"`
		PrimaryVal interface{}            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "更新表 "+req.Table+" 记录")

	result, err := h.svc.UpdateRecord(c.Request.Context(), iid, dbName, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) DeleteRecord(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	var req struct {
		Table      string      `json:"table" binding:"required"`
		PrimaryKey string      `json:"primary_key" binding:"required"`
		PrimaryVal interface{} `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "删除表 "+req.Table+" 记录")

	result, err := h.svc.DeleteRecord(c.Request.Context(), iid, dbName, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

// --- Table management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	var req struct {
		Name    string `json:"name" binding:"required"`
		Columns []struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Nullable  bool   `json:"nullable"`
			IsPrimary bool   `json:"is_primary"`
			AutoIncr  bool   `json:"auto_incr"`
		} `json:"columns" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建表 "+req.Name)

	var columns []database.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, database.TableColumn{
			Name:      col.Name,
			Type:      col.Type,
			Nullable:  col.Nullable,
			IsPrimary: col.IsPrimary,
			AutoIncr:  col.AutoIncr,
		})
	}

	if err := h.svc.CreateTable(c.Request.Context(), iid, dbName, req.Name, columns); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			c.Error(apperror.ErrBadRequest.Wrap(err))
		} else {
			c.Error(apperror.WrapError(err))
		}
		return
	}

	httpx.Success(c, gin.H{"message": "表已创建", "name": req.Name})
}

func (h *DatabaseHandler) DropTable(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	middleware.AuditSummary(c, "删除表 "+tableName)

	if err := h.svc.DropTable(c.Request.Context(), iid, dbName, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			c.Error(apperror.ErrBadRequest.Wrap(err))
		} else {
			c.Error(apperror.WrapError(err))
		}
		return
	}

	httpx.Success(c, gin.H{"message": "表已删除", "name": tableName})
}

// UserHandler handles DB user management endpoints.
type UserHandler struct {
	svc *database.Service
}

func NewUserHandler(svc *database.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) ListDBUsers(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	users, err := h.svc.ListDBUsers(c.Request.Context(), iid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, users)
}

func (h *UserHandler) CreateDBUser(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	var req database.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库用户 "+req.Username)
	user, err := h.svc.CreateDBUser(c.Request.Context(), iid, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, user)
}

func (h *UserHandler) DeleteDBUser(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	username := c.Param("username")
	if username == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的用户名"))
		return
	}
	host := c.DefaultQuery("host", "%")
	middleware.AuditSummary(c, "删除数据库用户 "+username)
	if err := h.svc.DeleteDBUser(c.Request.Context(), iid, username, host); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

func (h *UserHandler) GrantPrivileges(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	username := c.Param("username")
	if username == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的用户名"))
		return
	}
	host := c.DefaultQuery("host", "%")
	var req database.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "授权数据库用户 "+username)
	if err := h.svc.GrantPrivileges(c.Request.Context(), iid, username, host, &req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "权限已授予"})
}

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	svc *database.Service
}

func NewBackupHandler(svc *database.Service) *BackupHandler {
	return &BackupHandler{svc: svc}
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}
	middleware.AuditSummary(c, "创建数据库备份 "+dbName)

	instance, err := h.svc.GetInstance(c.Request.Context(), iid)
	if err != nil || instance == nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库实例不存在"))
		return
	}

	backup, err := h.svc.CreateBackup(c.Request.Context(), iid, dbName, instance.DBType)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, backup)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	iid, ok := parseIID(c)
	if !ok {
		return
	}
	dbName := c.Param("dbname")
	if dbName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库名"))
		return
	}

	backups, err := h.svc.ListBackups(c.Request.Context(), iid, dbName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	if backups == nil {
		backups = []database.DBBackup{}
	}

	httpx.Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}

	if backup.Status != "success" {
		c.Error(apperror.ErrBadRequest.WithMessage("备份未完成"))
		return
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		c.Error(apperror.ErrNotFound.WithMessage("备份文件不存在"))
		return
	}

	c.File(backup.FilePath)
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		c.Error(apperror.ErrBadRequest.WithMessage("请确认恢复，设置 {\"confirm\": true}"))
		return
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "恢复数据库备份 "+backup.DatabaseName)

	if err := h.svc.RestoreBackup(c.Request.Context(), bid, backup.DBType); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "恢复已开始，请轮询恢复状态"})
}

func (h *BackupHandler) RestoreStatus(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
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
	status, ok := h.svc.GetRestoreStatus(c.Request.Context(), bid)
	if !ok {
		send(map[string]any{"type": "done", "error": "恢复状态已丢失（服务可能已重启），请手动确认数据"})
		return
	}

	// 任务运行中保持连接；10s 心跳防反向代理超时断连，500ms 轮询检查终态。
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			send(map[string]any{"type": "running"})
			continue
		case <-time.After(500 * time.Millisecond):
		}
		status, ok = h.svc.GetRestoreStatus(c.Request.Context(), bid)
		if !ok {
			send(map[string]any{"type": "done", "error": "恢复状态已丢失（服务可能已重启），请手动确认数据"})
			return
		}
		if status.Status != "running" {
			send(map[string]any{"type": "done", "status": status.Status, "error": status.Error})
			return
		}
	}
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库备份 "+backup.DatabaseName)

	if err := h.svc.DeleteBackup(c.Request.Context(), bid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "备份已删除"})

}
