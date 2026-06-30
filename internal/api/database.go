package api

import (
	"easyserver/internal/api/middleware"
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"

	"github.com/gin-gonic/gin"
)

// registerDatabaseRoutes registers database management routes
func registerDatabaseRoutes(protected *gin.RouterGroup, dbServerService *dbserver.Service, dbMgmtService *database_mgmt.Service) {
	handler := NewDBServerHandler(dbServerService)
	versionHandler := NewVersionHandler(dbServerService)
	dbHandler := NewDatabaseHandler(dbMgmtService)
	userHandler := NewUserHandler(dbMgmtService)
	backupHandler := NewBackupHandler(dbServerService, dbMgmtService)
	configHandler := NewConfigHandler()

	protected.GET("/db-servers", handler.List)
	protected.GET("/db-servers/:id", handler.Get)

	// Version management
	protected.GET("/db-servers/:id/version-templates", versionHandler.GetVersionTemplates)
	protected.GET("/db-servers/:id/versions", versionHandler.ListVersions)
	protected.POST("/db-servers/:id/versions", middleware.SetAction("DBSERVERS_INSTALL"), versionHandler.InstallVersion)
	protected.DELETE("/db-servers/versions/:vid", middleware.SetAction("DBSERVERS_UNINSTALL"), versionHandler.UninstallVersion)
	protected.POST("/db-servers/versions/:vid/start", middleware.SetAction("DATABASE_START_VERSION"), versionHandler.StartVersion)
	protected.POST("/db-servers/versions/:vid/stop", middleware.SetAction("DATABASE_STOP_VERSION"), versionHandler.StopVersion)
	protected.POST("/db-servers/versions/:vid/restart", middleware.SetAction("DATABASE_RESTART_VERSION"), versionHandler.RestartVersion)
	protected.PUT("/db-servers/versions/:vid/port", middleware.SetAction("DATABASE_UPDATE_PORT"), versionHandler.UpdateVersionPort)
	protected.GET("/db-servers/versions/:vid/logs", versionHandler.GetVersionLogs)

	// Databases nested
	protected.GET("/db-servers/:id/databases", dbHandler.ListDatabases)
	protected.POST("/db-servers/:id/databases", middleware.SetAction("DATABASES_CREATE"), dbHandler.CreateDatabase)
	protected.DELETE("/db-servers/:id/databases/:did", middleware.SetAction("DATABASES_DELETE"), dbHandler.DeleteDatabase)

	// DB Users nested
	protected.GET("/db-servers/:id/users", userHandler.ListDBUsers)
	protected.POST("/db-servers/:id/users", middleware.SetAction("DATABASE_CREATE_USER"), userHandler.CreateDBUser)
	protected.DELETE("/db-servers/:id/users/:uid", middleware.SetAction("DATABASE_DELETE_USER"), userHandler.DeleteDBUser)
	protected.POST("/db-servers/:id/users/:uid/grant", middleware.SetAction("DATABASE_GRANT_PRIVILEGES"), userHandler.GrantPrivileges)

	// Database introspection
	protected.GET("/db-servers/databases/:did/tables", dbHandler.ListTables)
	protected.GET("/db-servers/databases/:did/describe", dbHandler.DescribeTable)
	protected.GET("/db-servers/databases/:did/query", dbHandler.QueryTable)
	protected.POST("/db-servers/databases/:did/execute", middleware.SetAction("DATABASE_EXECUTE_SQL"), dbHandler.ExecuteSQL)
	protected.POST("/db-servers/databases/:did/insert", middleware.SetAction("DATABASE_INSERT_RECORD"), dbHandler.InsertRecord)
	protected.POST("/db-servers/databases/:did/update", middleware.SetAction("DATABASE_UPDATE_RECORD"), dbHandler.UpdateRecord)
	protected.POST("/db-servers/databases/:did/delete", middleware.SetAction("DATABASE_DELETE_RECORD"), dbHandler.DeleteRecord)

	// Table management
	protected.POST("/db-servers/databases/:did/tables", middleware.SetAction("DATABASE_CREATE_TABLE"), dbHandler.CreateTable)
	protected.DELETE("/db-servers/databases/:did/tables", middleware.SetAction("DATABASE_DROP_TABLE"), dbHandler.DropTable)

	// Database backup
	protected.POST("/db-servers/databases/:did/backup", middleware.SetAction("DATABASE_CREATE_BACKUP"), backupHandler.CreateBackup)
	protected.GET("/db-servers/databases/:did/backups", backupHandler.ListBackups)
	protected.GET("/db-servers/backups/:bid/download", backupHandler.DownloadBackup)
	protected.POST("/db-servers/backups/:bid/restore", middleware.SetAction("DATABASE_RESTORE_BACKUP"), backupHandler.RestoreBackup)
	protected.DELETE("/db-servers/backups/:bid", middleware.SetAction("DATABASE_DELETE_BACKUP"), backupHandler.DeleteBackup)

	// MySQL config management
	protected.GET("/db-servers/mysql/config", configHandler.GetMySQLConfig)
	protected.POST("/db-servers/mysql/config", middleware.SetAction("DATABASE_SAVE_MYSQL_CONFIG"), configHandler.SaveMySQLConfig)
	protected.GET("/db-servers/mysql/common-params", configHandler.GetMySQLCommonParams)

	// PostgreSQL config management
	protected.GET("/db-servers/postgresql/config", configHandler.GetPostgreSQLConfig)
	protected.POST("/db-servers/postgresql/config", middleware.SetAction("DATABASE_SAVE_POSTGRES_CONFIG"), configHandler.SavePostgreSQLConfig)
	protected.GET("/db-servers/postgresql/common-params", configHandler.GetPGCommonParams)

	// Redis config management
	protected.GET("/db-servers/redis/config", configHandler.GetRedisConfig)
	protected.POST("/db-servers/redis/config", middleware.SetAction("DATABASE_SAVE_REDIS_CONFIG"), configHandler.SaveRedisConfig)
	protected.GET("/db-servers/redis/common-params", configHandler.GetRedisCommonParams)
}
