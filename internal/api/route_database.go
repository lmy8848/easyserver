package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerDatabaseRoutes registers database management routes
func registerDatabaseRoutes(protected *gin.RouterGroup, dbServerService *service.DBServerService, dbMgmtService *service.DatabaseMgmtService, dbBackupService *service.DBBackupService) {
	handler := NewDBServerHandler(dbServerService, dbMgmtService, dbBackupService)

	protected.GET("/db-servers", handler.List)
	protected.GET("/db-servers/:id", handler.Get)

	// Version management
	protected.GET("/db-servers/:id/version-templates", handler.GetVersionTemplates)
	protected.GET("/db-servers/:id/versions", handler.ListVersions)
	protected.POST("/db-servers/:id/versions", handler.InstallVersion)
	protected.DELETE("/db-servers/versions/:vid", handler.UninstallVersion)
	protected.POST("/db-servers/versions/:vid/start", handler.StartVersion)
	protected.POST("/db-servers/versions/:vid/stop", handler.StopVersion)
	protected.POST("/db-servers/versions/:vid/restart", handler.RestartVersion)
	protected.PUT("/db-servers/versions/:vid/port", handler.UpdateVersionPort)
	protected.GET("/db-servers/versions/:vid/logs", handler.GetVersionLogs)

	// Databases nested
	protected.GET("/db-servers/:id/databases", handler.ListDatabases)
	protected.POST("/db-servers/:id/databases", handler.CreateDatabase)
	protected.DELETE("/db-servers/:id/databases/:did", handler.DeleteDatabase)

	// DB Users nested
	protected.GET("/db-servers/:id/users", handler.ListDBUsers)
	protected.POST("/db-servers/:id/users", handler.CreateDBUser)
	protected.DELETE("/db-servers/:id/users/:uid", handler.DeleteDBUser)
	protected.POST("/db-servers/:id/users/:uid/grant", handler.GrantPrivileges)

	// Database introspection
	protected.GET("/db-servers/databases/:did/tables", handler.ListTables)
	protected.GET("/db-servers/databases/:did/describe", handler.DescribeTable)
	protected.GET("/db-servers/databases/:did/query", handler.QueryTable)
	protected.POST("/db-servers/databases/:did/execute", handler.ExecuteSQL)
	protected.POST("/db-servers/databases/:did/insert", handler.InsertRecord)
	protected.POST("/db-servers/databases/:did/update", handler.UpdateRecord)
	protected.POST("/db-servers/databases/:did/delete", handler.DeleteRecord)

	// Table management
	protected.POST("/db-servers/databases/:did/tables", handler.CreateTable)
	protected.DELETE("/db-servers/databases/:did/tables", handler.DropTable)

	// Database backup
	protected.POST("/db-servers/databases/:did/backup", handler.CreateBackup)
	protected.GET("/db-servers/databases/:did/backups", handler.ListBackups)
	protected.GET("/db-servers/backups/:bid/download", handler.DownloadBackup)
	protected.POST("/db-servers/backups/:bid/restore", handler.RestoreBackup)
	protected.DELETE("/db-servers/backups/:bid", handler.DeleteBackup)

	// MySQL config management
	protected.GET("/db-servers/mysql/config", handler.GetMySQLConfig)
	protected.POST("/db-servers/mysql/config", handler.SaveMySQLConfig)
	protected.GET("/db-servers/mysql/common-params", handler.GetMySQLCommonParams)

	// PostgreSQL config management
	protected.GET("/db-servers/postgresql/config", handler.GetPostgreSQLConfig)
	protected.POST("/db-servers/postgresql/config", handler.SavePostgreSQLConfig)
	protected.GET("/db-servers/postgresql/common-params", handler.GetPGCommonParams)

	// Redis config management
	protected.GET("/db-servers/redis/config", handler.GetRedisConfig)
	protected.POST("/db-servers/redis/config", handler.SaveRedisConfig)
	protected.GET("/db-servers/redis/common-params", handler.GetRedisCommonParams)
}
