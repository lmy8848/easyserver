package api

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
}

func NewDBServerHandler(dbServerService *service.DBServerService, dbMgmtService *service.DatabaseMgmtService, dbBackupService *service.DBBackupService) *DBServerHandler {
	return &DBServerHandler{
		dbServerService: dbServerService,
		dbMgmtService:   dbMgmtService,
		dbBackupService: dbBackupService,
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

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	var tables []map[string]interface{}
	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", db.Name, "-e", "SHOW TABLES;").CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("获取表列表失败: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			if i == 0 {
				continue
			} // skip header
			line = strings.TrimSpace(line)
			if line != "" {
				tables = append(tables, map[string]interface{}{"name": line})
			}
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c",
			"SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;").CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("获取表列表失败: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i < 2 || line == "" || line == "(0 rows)" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "(") {
				continue
			}
			tables = append(tables, map[string]interface{}{"name": line})
		}
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
	if !validateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	var dbType service.DBType
	switch server.Name {
	case "mysql":
		dbType = service.DBTypeMySQL
	case "postgresql":
		dbType = service.DBTypePostgreSQL
	default:
		BadRequest(c, "不支持的数据库类型")
		return
	}

	// Build and execute DESCRIBE query
	builder := service.NewSQLBuilder(dbType)
	describeSQL := builder.BuildDescribeTable(tableName)

	var out []byte
	switch server.Name {
	case "mysql":
		out, err = exec.Command("mysql", db.Name, "-e", describeSQL).CombinedOutput()
	case "postgresql":
		out, err = exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c", describeSQL).CombinedOutput()
	}
	if err != nil {
		InternalError(c, fmt.Sprintf("获取表结构失败: %s", string(out)))
		return
	}

	// Parse into structured TableInfo
	tableInfo := service.ParseTableInfo(dbType, tableName, string(out))

	// Convert to response format
	var columns []map[string]interface{}
	for _, col := range tableInfo.Columns {
		c := map[string]interface{}{
			"name":           col.Name,
			"type":           col.Type,
			"is_primary_key": col.IsPrimaryKey,
			"is_auto_incr":   col.IsAutoIncr,
			"has_default":    col.HasDefault,
			"default":        col.DefaultValue,
			"is_nullable":    col.IsNullable,
		}
		columns = append(columns, c)
	}

	Success(c, gin.H{
		"table_name":  tableName,
		"primary_key": tableInfo.PrimaryKey,
		"columns":     columns,
	})
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
	if !validateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	// Get total count
	var total int
	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", db.Name, "-N", "-e", fmt.Sprintf("SELECT COUNT(*) FROM `%s`;", tableName)).CombinedOutput()
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-t", "-c",
			fmt.Sprintf("SELECT COUNT(*) FROM \"%s\";", tableName)).CombinedOutput()
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
		}
	}

	// Get data
	var headers []string
	var rows [][]interface{}
	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", db.Name, "-e",
			fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d;", tableName, pageSize, offset)).CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("查询失败: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "\t")
			if i == 0 {
				headers = fields
			} else {
				var row []interface{}
				for _, f := range fields {
					row = append(row, f)
				}
				rows = append(rows, row)
			}
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c",
			fmt.Sprintf("SELECT * FROM \"%s\" LIMIT %d OFFSET %d;", tableName, pageSize, offset)).CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("查询失败: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "|")
			for j := range fields {
				fields[j] = strings.TrimSpace(fields[j])
			}
			if i == 0 {
				headers = fields
			} else if i >= 2 && !strings.HasPrefix(line, "(") && line != "" {
				var row []interface{}
				for _, f := range fields {
					row = append(row, f)
				}
				rows = append(rows, row)
			}
		}
	}

	Success(c, gin.H{
		"headers":   headers,
		"rows":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// pathPattern matches filesystem paths that should be stripped from error output
var pathPattern = regexp.MustCompile(`(?:/[\w.-]+){2,}`)

// sanitizeSQLError strips sensitive information (file paths, internal details) from
// SQL command output while preserving the useful error message for the user.
func sanitizeSQLError(raw string) string {
	lines := strings.Split(raw, "\n")
	var sanitized []string
	for _, line := range lines {
		// Skip lines that contain filesystem paths
		if pathPattern.MatchString(line) {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sanitized = append(sanitized, line)
	}
	if len(sanitized) == 0 {
		return "query execution failed"
	}
	return strings.Join(sanitized, "\n")
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

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	// Validate SQL using the comprehensive validator
	dbType := getDBType(server.Name)
	validator := service.NewSQLValidator(dbType)
	if r := validator.ValidateSQL(req.SQL); !r.Valid {
		BadRequest(c, r.Message)
		return
	}

	var output string
	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", db.Name, "-e", req.SQL).CombinedOutput()
		if err != nil {
			log.Printf("ExecuteSQL mysql error [db=%s]: %s", db.Name, string(out))
			Success(c, gin.H{"success": false, "error": sanitizeSQLError(string(out))})
			return
		}
		output = string(out)
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c", req.SQL).CombinedOutput()
		if err != nil {
			log.Printf("ExecuteSQL postgresql error [db=%s]: %s", db.Name, string(out))
			Success(c, gin.H{"success": false, "error": sanitizeSQLError(string(out))})
			return
		}
		output = string(out)
	}

	Success(c, gin.H{"success": true, "output": output})
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
	if !validateTableName(req.Table) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	dbType := getDBType(server.Name)
	builder := service.NewSQLBuilder(dbType)
	validator := service.NewSQLValidator(dbType)

	// Validate
	if r := validator.ValidateInsert(req.Table, req.Data, nil); !r.Valid {
		BadRequest(c, r.Message)
		return
	}

	// Build SQL
	sql := builder.BuildInsert(req.Table, req.Data, nil)

	// Dry-run check
	if c.Query("dry_run") == "true" {
		Success(c, gin.H{"success": true, "dry_run": true, "sql": sql})
		return
	}

	// Execute
	output, err := executeSQL(dbType, db.Name, sql)
	if err != nil {
		Success(c, gin.H{"success": false, "error": output})
		return
	}

	Success(c, gin.H{"success": true, "output": output})
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
	if !validateTableName(req.Table) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	dbType := getDBType(server.Name)
	builder := service.NewSQLBuilder(dbType)
	validator := service.NewSQLValidator(dbType)

	if r := validator.ValidateUpdate(req.Table, req.Data, req.PrimaryKey, req.PrimaryVal); !r.Valid {
		BadRequest(c, r.Message)
		return
	}

	sql := builder.BuildUpdate(req.Table, req.Data, req.PrimaryKey, req.PrimaryVal)

	if c.Query("dry_run") == "true" {
		Success(c, gin.H{"success": true, "dry_run": true, "sql": sql})
		return
	}

	output, err := executeSQL(dbType, db.Name, sql)
	if err != nil {
		Success(c, gin.H{"success": false, "error": output})
		return
	}

	Success(c, gin.H{"success": true, "output": output})
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
	if !validateTableName(req.Table) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	dbType := getDBType(server.Name)
	builder := service.NewSQLBuilder(dbType)
	validator := service.NewSQLValidator(dbType)

	if r := validator.ValidateDelete(req.Table, req.PrimaryKey, req.PrimaryVal); !r.Valid {
		BadRequest(c, r.Message)
		return
	}

	sql := builder.BuildDelete(req.Table, req.PrimaryKey, req.PrimaryVal)

	if c.Query("dry_run") == "true" {
		Success(c, gin.H{"success": true, "dry_run": true, "sql": sql})
		return
	}

	output, err := executeSQL(dbType, db.Name, sql)
	if err != nil {
		Success(c, gin.H{"success": false, "error": output})
		return
	}

	Success(c, gin.H{"success": true})
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

// Helper functions

var tableNameRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validateTableName(name string) bool {
	return name != "" && len(name) <= 64 && tableNameRegexp.MatchString(name)
}

func getDBType(name string) service.DBType {
	switch name {
	case "mysql":
		return service.DBTypeMySQL
	case "postgresql":
		return service.DBTypePostgreSQL
	case "redis":
		return service.DBTypeRedis
	}
	return service.DBTypeMySQL
}

func executeSQL(dbType service.DBType, dbName string, sql string) (string, error) {
	switch dbType {
	case service.DBTypeMySQL:
		out, err := exec.Command("mysql", dbName, "-e", sql).CombinedOutput()
		return string(out), err
	case service.DBTypePostgreSQL:
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", dbName, "-c", sql).CombinedOutput()
		return string(out), err
	}
	return "", fmt.Errorf("不支持的数据库类型")
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

	if !validateTableName(req.Name) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	// Validate column types against whitelist
	allowedTypes := map[string]bool{
		"INT": true, "INTEGER": true, "TINYINT": true, "SMALLINT": true, "MEDIUMINT": true, "BIGINT": true,
		"FLOAT": true, "DOUBLE": true, "DECIMAL": true, "NUMERIC": true, "REAL": true,
		"VARCHAR": true, "CHAR": true, "TEXT": true, "TINYTEXT": true, "MEDIUMTEXT": true, "LONGTEXT": true,
		"BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true, "BINARY": true, "VARBINARY": true,
		"DATE": true, "TIME": true, "DATETIME": true, "TIMESTAMP": true, "YEAR": true,
		"BOOLEAN": true, "BOOL": true, "BIT": true,
		"JSON": true, "ENUM": true, "SET": true,
		"SERIAL": true, "BIGSERIAL": true, "SMALLSERIAL": true, // PostgreSQL
		"UUID": true, "JSONB": true, // PostgreSQL
	}
	for _, col := range req.Columns {
		// Extract base type (strip length/precision like VARCHAR(255))
		baseType := strings.ToUpper(strings.Split(col.Type, "(")[0])
		baseType = strings.TrimSpace(baseType)
		if !allowedTypes[baseType] {
			BadRequest(c, fmt.Sprintf("不支持的列类型: %s", col.Type))
			return
		}
		if !validateTableName(col.Name) {
			BadRequest(c, fmt.Sprintf("无效的列名: %s", col.Name))
			return
		}
	}

	// Build CREATE TABLE SQL
	var columns []string
	for _, col := range req.Columns {
		parts := []string{fmt.Sprintf("`%s`", col.Name), col.Type}
		if col.IsPrimary {
			parts = append(parts, "PRIMARY KEY")
		}
		if col.AutoIncr {
			parts = append(parts, "AUTO_INCREMENT")
		}
		if !col.Nullable {
			parts = append(parts, "NOT NULL")
		}
		columns = append(columns, strings.Join(parts, " "))
	}

	var sql string
	switch server.Name {
	case "mysql":
		sql = fmt.Sprintf("CREATE TABLE `%s` (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;",
			req.Name, strings.Join(columns, ", "))
	case "postgresql":
		// PostgreSQL syntax adjustments
		var pgColumns []string
		for _, col := range req.Columns {
			parts := []string{fmt.Sprintf("\"%s\"", col.Name), col.Type}
			if col.IsPrimary {
				parts = append(parts, "PRIMARY KEY")
			}
			if col.AutoIncr {
				// PostgreSQL uses SERIAL for auto increment
				parts = []string{fmt.Sprintf("\"%s\"", col.Name), "SERIAL", "PRIMARY KEY"}
			}
			if !col.Nullable && !col.IsPrimary {
				parts = append(parts, "NOT NULL")
			}
			pgColumns = append(pgColumns, strings.Join(parts, " "))
		}
		sql = fmt.Sprintf("CREATE TABLE \"%s\" (%s);", req.Name, strings.Join(pgColumns, ", "))
	default:
		BadRequest(c, "不支持的数据库类型")
		return
	}

	out, err := executeSQL(getDBType(server.Name), db.Name, sql)
	if err != nil {
		InternalError(c, fmt.Sprintf("创建表失败: %s", out))
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
	if !validateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil || db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if server == nil {
		NotFound(c, "服务器不存在")
		return
	}

	var sql string
	switch server.Name {
	case "mysql":
		sql = fmt.Sprintf("DROP TABLE `%s`;", tableName)
	case "postgresql":
		sql = fmt.Sprintf("DROP TABLE \"%s\";", tableName)
	default:
		BadRequest(c, "不支持的数据库类型")
		return
	}

	out, err := executeSQL(getDBType(server.Name), db.Name, sql)
	if err != nil {
		InternalError(c, fmt.Sprintf("删除表失败: %s", out))
		return
	}

	Success(c, gin.H{"message": "表已删除", "name": tableName})
}
