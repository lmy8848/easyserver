package api

import (
	"easyserver/internal/dbserver"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerDatabaseRoutes registers database management routes
func registerDatabaseRoutes(protected *gin.RouterGroup, dbServerService *dbserver.Service, dbMgmtService *service.DatabaseMgmtService, dbBackupService *service.DBBackupService, sqlService *service.SQLQueryService) {
	handler := NewDBServerHandler(dbServerService)
	versionHandler := NewVersionHandler(dbServerService)
	dbHandler := NewDatabaseHandler(dbMgmtService, sqlService)
	userHandler := NewUserHandler(dbMgmtService)
	backupHandler := NewBackupHandler(dbServerService, dbMgmtService, dbBackupService)
	configHandler := NewConfigHandler()

	protected.GET("/db-servers", handler.List)
	protected.GET("/db-servers/:id", handler.Get)

	// Version management
	protected.GET("/db-servers/:id/version-templates", versionHandler.GetVersionTemplates)
	protected.GET("/db-servers/:id/versions", versionHandler.ListVersions)
	protected.POST("/db-servers/:id/versions", versionHandler.InstallVersion)
	protected.DELETE("/db-servers/versions/:vid", versionHandler.UninstallVersion)
	protected.POST("/db-servers/versions/:vid/start", versionHandler.StartVersion)
	protected.POST("/db-servers/versions/:vid/stop", versionHandler.StopVersion)
	protected.POST("/db-servers/versions/:vid/restart", versionHandler.RestartVersion)
	protected.PUT("/db-servers/versions/:vid/port", versionHandler.UpdateVersionPort)
	protected.GET("/db-servers/versions/:vid/logs", versionHandler.GetVersionLogs)

	// Databases nested
	protected.GET("/db-servers/:id/databases", dbHandler.ListDatabases)
	protected.POST("/db-servers/:id/databases", dbHandler.CreateDatabase)
	protected.DELETE("/db-servers/:id/databases/:did", dbHandler.DeleteDatabase)

	// DB Users nested
	protected.GET("/db-servers/:id/users", userHandler.ListDBUsers)
	protected.POST("/db-servers/:id/users", userHandler.CreateDBUser)
	protected.DELETE("/db-servers/:id/users/:uid", userHandler.DeleteDBUser)
	protected.POST("/db-servers/:id/users/:uid/grant", userHandler.GrantPrivileges)

	// Database introspection
	protected.GET("/db-servers/databases/:did/tables", dbHandler.ListTables)
	protected.GET("/db-servers/databases/:did/describe", dbHandler.DescribeTable)
	protected.GET("/db-servers/databases/:did/query", dbHandler.QueryTable)
	protected.POST("/db-servers/databases/:did/execute", dbHandler.ExecuteSQL)
	protected.POST("/db-servers/databases/:did/insert", dbHandler.InsertRecord)
	protected.POST("/db-servers/databases/:did/update", dbHandler.UpdateRecord)
	protected.POST("/db-servers/databases/:did/delete", dbHandler.DeleteRecord)

	// Table management
	protected.POST("/db-servers/databases/:did/tables", dbHandler.CreateTable)
	protected.DELETE("/db-servers/databases/:did/tables", dbHandler.DropTable)

	// Database backup
	protected.POST("/db-servers/databases/:did/backup", backupHandler.CreateBackup)
	protected.GET("/db-servers/databases/:did/backups", backupHandler.ListBackups)
	protected.GET("/db-servers/backups/:bid/download", backupHandler.DownloadBackup)
	protected.POST("/db-servers/backups/:bid/restore", backupHandler.RestoreBackup)
	protected.DELETE("/db-servers/backups/:bid", backupHandler.DeleteBackup)

	// MySQL config management
	protected.GET("/db-servers/mysql/config", configHandler.GetMySQLConfig)
	protected.POST("/db-servers/mysql/config", configHandler.SaveMySQLConfig)
	protected.GET("/db-servers/mysql/common-params", configHandler.GetMySQLCommonParams)

	// PostgreSQL config management
	protected.GET("/db-servers/postgresql/config", configHandler.GetPostgreSQLConfig)
	protected.POST("/db-servers/postgresql/config", configHandler.SavePostgreSQLConfig)
	protected.GET("/db-servers/postgresql/common-params", configHandler.GetPGCommonParams)

	// Redis config management
	protected.GET("/db-servers/redis/config", configHandler.GetRedisConfig)
	protected.POST("/db-servers/redis/config", configHandler.SaveRedisConfig)
	protected.GET("/db-servers/redis/common-params", configHandler.GetRedisCommonParams)
}
