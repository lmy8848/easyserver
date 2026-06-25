package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type DBServerHandler struct {
	dbServerService *service.DBServerService
	dbMgmtService   *service.DatabaseMgmtService
	dbBackupService *service.DBBackupService
	sqlService      *service.SQLQueryService
}

func NewDBServerHandler(dbServerService *service.DBServerService, dbMgmtService *service.DatabaseMgmtService, dbBackupService *service.DBBackupService, sqlService *service.SQLQueryService) *DBServerHandler {
	return &DBServerHandler{
		dbServerService: dbServerService,
		dbMgmtService:   dbMgmtService,
		dbBackupService: dbBackupService,
		sqlService:      sqlService,
	}
}

// DB Server endpoints

func (h *DBServerHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	h.dbServerService.RefreshAllStatus(ctx)
	servers, err := h.dbServerService.List(ctx)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, servers)
}

func (h *DBServerHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的 ID")
		return
	}
	h.dbServerService.RefreshStatus(ctx, id)
	server, err := h.dbServerService.Get(ctx, id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if server == nil {
		NotFound(c, "数据库服务器不存在")
		return
	}
	Success(c, server)
}

// Version endpoints

func (h *DBServerHandler) GetVersionTemplates(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的 ID")
		return
	}
	server, err := h.dbServerService.Get(c.Request.Context(), id)
	if err != nil || server == nil {
		NotFound(c, "数据库服务器不存在")
		return
	}
	templates := model.GetVersionTemplates(server.Name)
	Success(c, templates)
}

func (h *DBServerHandler) ListVersions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的 ID")
		return
	}
	versions, err := h.dbServerService.ListVersions(c.Request.Context(), id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, versions)
}

func (h *DBServerHandler) InstallVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的 ID")
		return
	}
	var req model.CreateDBVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	version, err := h.dbServerService.InstallVersion(c.Request.Context(), id, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, version)
}

func (h *DBServerHandler) UninstallVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}
	if err := h.dbServerService.UninstallVersion(c.Request.Context(), vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "已卸载"})
}

func (h *DBServerHandler) StartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}
	if err := h.dbServerService.StartVersion(c.Request.Context(), vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *DBServerHandler) StopVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}
	if err := h.dbServerService.StopVersion(c.Request.Context(), vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "stopped"})
}

func (h *DBServerHandler) RestartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}
	if err := h.dbServerService.RestartVersion(c.Request.Context(), vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *DBServerHandler) UpdateVersionPort(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}

	var req struct {
		Port int `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.Port < 1 || req.Port > 65535 {
		BadRequest(c, "端口必须在 1 到 65535 之间")
		return
	}

	// Check port availability
	if err := h.dbServerService.UpdateVersionPort(c.Request.Context(), vid, req.Port); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "端口已更新", "port": req.Port})
}

func (h *DBServerHandler) GetVersionLogs(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的版本ID")
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.dbServerService.GetVersionServiceLogs(c.Request.Context(), vid, lines)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"logs": logs})
}

// Database endpoints

func (h *DBServerHandler) ListDatabases(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	dbs, err := h.dbMgmtService.ListDatabases(c.Request.Context(), sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, dbs)
}

func (h *DBServerHandler) CreateDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	var req model.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	db, err := h.dbMgmtService.CreateDatabase(c.Request.Context(), sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, db)
}

func (h *DBServerHandler) DeleteDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	dbID, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	if err := h.dbMgmtService.DeleteDatabase(c.Request.Context(), sid, dbID); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

// DB User endpoints

func (h *DBServerHandler) ListDBUsers(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	users, err := h.dbMgmtService.ListDBUsers(c.Request.Context(), sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, users)
}

func (h *DBServerHandler) CreateDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	var req model.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	user, err := h.dbMgmtService.CreateDBUser(c.Request.Context(), sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, user)
}

func (h *DBServerHandler) DeleteDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的用户ID")
		return
	}
	if err := h.dbMgmtService.DeleteDBUser(c.Request.Context(), sid, uid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

func (h *DBServerHandler) GrantPrivileges(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的用户ID")
		return
	}
	var req model.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if err := h.dbMgmtService.GrantPrivileges(c.Request.Context(), sid, uid, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "权限已授予"})
}

// Database introspection endpoints

// ListTables lists tables in a database
func (h *DBServerHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	tables, err := h.sqlService.ListTables(c.Request.Context(), did)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, tables)
}

// DescribeTable returns table structure
func (h *DBServerHandler) DescribeTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}
	if !service.ValidateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}

	result, err := h.sqlService.DescribeTable(c.Request.Context(), did, tableName)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, result)
}

// QueryTable returns table data with pagination
func (h *DBServerHandler) QueryTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}
	if !service.ValidateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.sqlService.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"headers":   result.Headers,
		"rows":      result.Rows,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ExecuteSQL executes a SQL query
func (h *DBServerHandler) ExecuteSQL(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.ExecuteSQL(c.Request.Context(), did, req.SQL)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	if !result.Success {
		Success(c, gin.H{"success": false, "error": result.Error})
		return
	}

	Success(c, gin.H{"success": true, "output": result.Output})
}

// InsertRecord inserts a record into a table
func (h *DBServerHandler) InsertRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table string                 `json:"table" binding:"required"`
		Data  map[string]interface{} `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.InsertRecord(c.Request.Context(), did, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

// UpdateRecord updates a record in a table
func (h *DBServerHandler) UpdateRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table      string                 `json:"table" binding:"required"`
		Data       map[string]interface{} `json:"data" binding:"required"`
		PrimaryKey string                 `json:"primary_key" binding:"required"`
		PrimaryVal interface{}            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

// DeleteRecord deletes a record from a table
func (h *DBServerHandler) DeleteRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table      string      `json:"table" binding:"required"`
		PrimaryKey string      `json:"primary_key" binding:"required"`
		PrimaryVal interface{} `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.DeleteRecord(c.Request.Context(), did, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

// MySQL Config endpoints

func (h *DBServerHandler) GetMySQLConfig(c *gin.Context) {
	configPath := service.FindMySQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 MySQL 配置文件"})
		return
	}

	config, err := service.ParseMySQLConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// Build response with common params metadata
	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetCommonParams(section.Name),
		}
	}

	Success(c, response)
}

// saveRawConfig writes raw text content directly to a config file with backup
func saveRawConfig(filePath, content string) error {
	backupPath := filePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(filePath); err == nil {
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(filePath, []byte(content), 0644)
}

// isRawConfigRequest checks if the request is a raw text save (from the raw text modal)
func isRawConfigRequest(sections []struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}) (string, bool) {
	if len(sections) == 1 && sections[0].Name == "custom" {
		if raw, ok := sections[0].Params["raw"]; ok {
			return raw, true
		}
	}
	return "", false
}

func (h *DBServerHandler) SaveMySQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindMySQLConfig()
	if configPath == "" {
		BadRequest(c, "未找到 MySQL 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SaveMySQLConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *DBServerHandler) GetMySQLCommonParams(c *gin.Context) {
	section := c.DefaultQuery("section", "mysqld")
	params := service.GetCommonParams(section)
	Success(c, params)
}

// PostgreSQL Config endpoints

func (h *DBServerHandler) GetPostgreSQLConfig(c *gin.Context) {
	configPath := service.FindPostgreSQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 PostgreSQL 配置文件"})
		return
	}

	config, err := service.ParsePostgreSQLConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetPostgreSQLCommonParams(),
		}
	}

	Success(c, response)
}

func (h *DBServerHandler) SavePostgreSQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindPostgreSQLConfig()
	if configPath == "" {
		BadRequest(c, "未找到 PostgreSQL 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SavePostgreSQLConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *DBServerHandler) GetPGCommonParams(c *gin.Context) {
	params := service.GetPostgreSQLCommonParams()
	Success(c, params)
}

// Redis Config endpoints

func (h *DBServerHandler) GetRedisConfig(c *gin.Context) {
	configPath := service.FindRedisConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 Redis 配置文件"})
		return
	}

	config, err := service.ParseRedisConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetRedisCommonParams(),
		}
	}

	Success(c, response)
}

func (h *DBServerHandler) SaveRedisConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindRedisConfig()
	if configPath == "" {
		BadRequest(c, "未找到 Redis 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SaveRedisConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *DBServerHandler) GetRedisCommonParams(c *gin.Context) {
	params := service.GetRedisCommonParams()
	Success(c, params)
}


// Backup endpoints

// CreateBackup creates a backup of a database
func (h *DBServerHandler) CreateBackup(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	// Get database info
	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		NotFound(c, "数据库不存在")
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), db.DBServerID)
	if err != nil {
		NotFound(c, "数据库服务器不存在")
		return
	}

	backup, err := h.dbBackupService.CreateBackup(c.Request.Context(), db.DBServerID, db.DBVersionID, did, db.Name, server.Name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, backup)
}

// ListBackups returns all backups for a database
func (h *DBServerHandler) ListBackups(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	backups, err := h.dbBackupService.ListBackups(c.Request.Context(), did)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	if backups == nil {
		backups = []model.DBBackup{}
	}

	Success(c, backups)
}

// DownloadBackup downloads a backup file
func (h *DBServerHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	backup, err := h.dbBackupService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		NotFound(c, "备份不存在")
		return
	}

	if backup.Status != "completed" {
		BadRequest(c, "备份未完成")
		return
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		NotFound(c, "备份文件不存在")
		return
	}

	c.File(backup.FilePath)
}

// RestoreBackup restores a database from backup
func (h *DBServerHandler) RestoreBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		BadRequest(c, "请确认恢复，设置 {\"confirm\": true}")
		return
	}

	backup, err := h.dbBackupService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		NotFound(c, "备份不存在")
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), backup.DBServerID)
	if err != nil {
		NotFound(c, "数据库服务器不存在")
		return
	}

	if err := h.dbBackupService.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "数据库恢复成功"})
}

// DeleteBackup deletes a backup
func (h *DBServerHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	if err := h.dbBackupService.DeleteBackup(c.Request.Context(), bid); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "备份已删除"})
}

// CreateTable creates a new table in a database
func (h *DBServerHandler) CreateTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
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
		BadRequest(c, err.Error())
		return
	}

	var columns []service.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, service.TableColumn{
			Name:      col.Name,
			Type:      col.Type,
			Nullable:  col.Nullable,
			IsPrimary: col.IsPrimary,
			AutoIncr:  col.AutoIncr,
		})
	}

	if err := h.sqlService.CreateTable(c.Request.Context(), did, req.Name, columns); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			BadRequest(c, err.Error())
		} else {
			InternalError(c, err.Error())
		}
		return
	}

	Success(c, gin.H{"message": "表已创建", "name": req.Name})
}

// DropTable drops a table from a database
func (h *DBServerHandler) DropTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}

	if err := h.sqlService.DropTable(c.Request.Context(), did, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			BadRequest(c, err.Error())
		} else {
			InternalError(c, err.Error())
		}
		return
	}

	Success(c, gin.H{"message": "表已删除", "name": tableName})
}
