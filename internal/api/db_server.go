package api

import (
	"fmt"
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
}

func NewDBServerHandler(dbServerService *service.DBServerService, dbMgmtService *service.DatabaseMgmtService) *DBServerHandler {
	return &DBServerHandler{
		dbServerService: dbServerService,
		dbMgmtService:   dbMgmtService,
	}
}

// DB Server endpoints

func (h *DBServerHandler) List(c *gin.Context) {
	h.dbServerService.RefreshAllStatus()
	servers, err := h.dbServerService.List()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, servers)
}

func (h *DBServerHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}
	h.dbServerService.RefreshStatus(id)
	server, err := h.dbServerService.Get(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if server == nil {
		NotFound(c, "database server not found")
		return
	}
	Success(c, server)
}

// Version endpoints

func (h *DBServerHandler) GetVersionTemplates(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}
	server, err := h.dbServerService.Get(id)
	if err != nil || server == nil {
		NotFound(c, "database server not found")
		return
	}
	templates := model.GetVersionTemplates(server.Name)
	Success(c, templates)
}

func (h *DBServerHandler) ListVersions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}
	versions, err := h.dbServerService.ListVersions(id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, versions)
}

func (h *DBServerHandler) InstallVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid id")
		return
	}
	var req model.CreateDBVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	version, err := h.dbServerService.InstallVersion(id, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, version)
}

func (h *DBServerHandler) UninstallVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}
	if err := h.dbServerService.UninstallVersion(vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "uninstalled"})
}

func (h *DBServerHandler) StartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}
	if err := h.dbServerService.StartVersion(vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *DBServerHandler) StopVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}
	if err := h.dbServerService.StopVersion(vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "stopped"})
}

func (h *DBServerHandler) RestartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}
	if err := h.dbServerService.RestartVersion(vid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *DBServerHandler) UpdateVersionPort(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
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
		BadRequest(c, "port must be between 1 and 65535")
		return
	}

	// Check port availability
	if err := h.dbServerService.UpdateVersionPort(vid, req.Port); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "port updated", "port": req.Port})
}

func (h *DBServerHandler) GetVersionLogs(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.dbServerService.GetVersionServiceLogs(vid, lines)
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
		BadRequest(c, "invalid server id")
		return
	}
	dbs, err := h.dbMgmtService.ListDatabases(sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, dbs)
}

func (h *DBServerHandler) CreateDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	var req model.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	db, err := h.dbMgmtService.CreateDatabase(sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, db)
}

func (h *DBServerHandler) DeleteDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	dbID, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid database id")
		return
	}
	if err := h.dbMgmtService.DeleteDatabase(sid, dbID); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

// DB User endpoints

func (h *DBServerHandler) ListDBUsers(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	users, err := h.dbMgmtService.ListDBUsers(sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, users)
}

func (h *DBServerHandler) CreateDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	var req model.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	user, err := h.dbMgmtService.CreateDBUser(sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, user)
}

func (h *DBServerHandler) DeleteDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}
	if err := h.dbMgmtService.DeleteDBUser(sid, uid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

func (h *DBServerHandler) GrantPrivileges(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}
	var req model.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if err := h.dbMgmtService.GrantPrivileges(sid, uid, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "privileges granted"})
}

// Database introspection endpoints

// ListTables lists tables in a database
func (h *DBServerHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid database id")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
		return
	}

	var tables []map[string]interface{}
	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", db.Name, "-e", "SHOW TABLES;").CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("failed to list tables: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			if i == 0 { continue } // skip header
			line = strings.TrimSpace(line)
			if line != "" {
				tables = append(tables, map[string]interface{}{"name": line})
			}
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c",
			"SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;").CombinedOutput()
		if err != nil {
			InternalError(c, fmt.Sprintf("failed to list tables: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i < 2 || line == "" || line == "(0 rows)" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "(") { continue }
			tables = append(tables, map[string]interface{}{"name": line})
		}
	}

	Success(c, tables)
}

// DescribeTable returns table structure
func (h *DBServerHandler) DescribeTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid database id")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "table name is required")
		return
	}
	if !validateTableName(tableName) {
		BadRequest(c, "invalid table name")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
		return
	}

	var dbType service.DBType
	switch server.Name {
	case "mysql":
		dbType = service.DBTypeMySQL
	case "postgresql":
		dbType = service.DBTypePostgreSQL
	default:
		BadRequest(c, "unsupported database type")
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
		InternalError(c, fmt.Sprintf("failed to describe table: %s", string(out)))
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
		BadRequest(c, "invalid database id")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "table name is required")
		return
	}
	if !validateTableName(tableName) {
		BadRequest(c, "invalid table name")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 { page = 1 }
	if pageSize < 1 { pageSize = 50 }
	if pageSize > 200 { pageSize = 200 }
	offset := (page - 1) * pageSize

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
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
			InternalError(c, fmt.Sprintf("failed to query: %s", string(out)))
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
			InternalError(c, fmt.Sprintf("failed to query: %s", string(out)))
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "|")
			for j := range fields { fields[j] = strings.TrimSpace(fields[j]) }
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

// ExecuteSQL executes a SQL query
func (h *DBServerHandler) ExecuteSQL(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid database id")
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}

	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
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
			Success(c, gin.H{"success": false, "error": string(out)})
			return
		}
		output = string(out)
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", db.Name, "-c", req.SQL).CombinedOutput()
		if err != nil {
			Success(c, gin.H{"success": false, "error": string(out)})
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
		BadRequest(c, "invalid database id")
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
		BadRequest(c, "invalid table name")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
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
		BadRequest(c, "invalid database id")
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
		BadRequest(c, "invalid table name")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
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
		BadRequest(c, "invalid database id")
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
		BadRequest(c, "invalid table name")
		return
	}

	db, err := h.dbMgmtService.GetDatabaseByID(did)
	if err != nil || db == nil {
		NotFound(c, "database not found")
		return
	}
	server, _ := h.dbMgmtService.GetServerByID(db.DBServerID)
	if server == nil {
		NotFound(c, "server not found")
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
		Success(c, gin.H{"found": false, "message": "MySQL config file not found"})
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
		os.WriteFile(backupPath, data, 0644)
	}
	dir := filepath.Dir(filePath)
	os.MkdirAll(dir, 0755)
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
		configPath = "/etc/mysql/my.cnf"
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "config saved", "path": configPath})
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

	Success(c, gin.H{"message": "config saved", "path": configPath})
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
		Success(c, gin.H{"found": false, "message": "PostgreSQL config file not found"})
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
		configPath = "/etc/postgresql/main/postgresql.conf"
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "config saved", "path": configPath})
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

	Success(c, gin.H{"message": "config saved", "path": configPath})
}

func (h *DBServerHandler) GetPGCommonParams(c *gin.Context) {
	params := service.GetPostgreSQLCommonParams()
	Success(c, params)
}

// Redis Config endpoints

func (h *DBServerHandler) GetRedisConfig(c *gin.Context) {
	configPath := service.FindRedisConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "Redis config file not found"})
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
		configPath = "/etc/redis/redis.conf"
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "config saved", "path": configPath})
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

	Success(c, gin.H{"message": "config saved", "path": configPath})
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
	return "", fmt.Errorf("unsupported database type")
}
