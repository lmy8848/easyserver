package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"easyserver/internal/config"
	"easyserver/internal/middleware"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg            *config.Config
	db             *sql.DB
	authService    *service.AuthService
	monitorService *service.MonitorService
	auditService   *service.AuditService
	sessionService *service.SessionService
}

func NewRouter(cfg *config.Config, db *sql.DB, monitorService *service.MonitorService) *Router {
	authService := service.NewAuthService(db, cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutDuration)
	auditService := service.NewAuditService(db)
	sessionService := service.NewSessionService(db)

	return &Router{
		cfg:            cfg,
		db:             db,
		authService:    authService,
		monitorService: monitorService,
		auditService:   auditService,
		sessionService: sessionService,
	}
}

func (r *Router) Setup() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()

	// Set max memory for multipart forms (32 MB)
	e.MaxMultipartMemory = 32 << 20

	// Create IP whitelist
	ipWhitelist := middleware.NewIPWhitelist(&r.cfg.Auth)

	// Token validator for blacklist check
	tokenValidator := func(userID int64, tokenString string, issuedAt time.Time) (bool, error) {
		return r.authService.IsUserTokenInvalidated(userID, issuedAt)
	}

	// Session validator for single session check
	sessionValidator := func(token string) (bool, error) {
		return r.sessionService.IsSessionValid(token)
	}

	// Global middleware
	e.Use(gin.Logger())
	e.Use(gin.Recovery())
	e.Use(middleware.SecurityMiddleware())
	e.Use(middleware.CORSMiddleware())
	e.Use(middleware.RateLimitMiddleware(r.cfg.Auth.RateLimit, r.cfg.Auth.RateInterval))
	e.Use(middleware.IPWhitelistMiddleware(ipWhitelist))

	// Health check
	e.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes
	api := e.Group("/api")
	{
		// Auth (public)
		auth := api.Group("/auth")
		{
			authHandler := NewAuthHandler(r.authService, r.cfg.Auth.JWTSecret, r.auditService, r.sessionService)
			auth.POST("/login", authHandler.Login)
			auth.POST("/logout", authHandler.Logout)
		}

		// Auth (protected)
		authProtected := api.Group("/auth")
		authProtected.Use(middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator))
		{
			authHandler := NewAuthHandler(r.authService, r.cfg.Auth.JWTSecret, r.auditService, r.sessionService)
			authProtected.GET("/me", authHandler.GetProfile)
			authProtected.POST("/change-password", authHandler.ChangePassword)
		}

		// Protected routes (JWT + Audit + Session Heartbeat)
		protected := api.Group("")
		protected.Use(middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator))
		protected.Use(middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			var wl string
			err := r.db.QueryRow("SELECT COALESCE(ip_whitelist, '') FROM users WHERE id = ?", userID).Scan(&wl)
			return wl, err
		}))
		protected.Use(middleware.SessionHeartbeatMiddleware(r.sessionService))
		protected.Use(middleware.AuditMiddleware(r.db))
		{
			// Monitor (viewer+)
			monitorHandler := NewMonitorHandler(r.monitorService, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
			protected.GET("/monitor/stats", monitorHandler.HandleStats)
			protected.GET("/monitor/history", monitorHandler.HandleHistory)

			// Services read (viewer+)
			serviceHandler := NewServiceHandler(r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
			protected.GET("/services", serviceHandler.List)
			protected.GET("/services/:name", serviceHandler.Get)
			protected.GET("/services/:name/logs", serviceHandler.GetLogs)

			// Services write (operator+)
			svcOp := protected.Group("/services/:name")
			svcOp.Use(middleware.RequireRole("admin", "operator"))
			{
				svcOp.POST("/start", serviceHandler.Start)
				svcOp.POST("/stop", serviceHandler.Stop)
				svcOp.POST("/restart", serviceHandler.Restart)
				svcOp.POST("/enable", serviceHandler.Enable)
				svcOp.POST("/disable", serviceHandler.Disable)
			}

			// Terminal (operator+)
			term := protected.Group("")
			term.Use(middleware.RequireRole("admin", "operator"))
			{
				term.GET("/terminal/:id", func(c *gin.Context) {
					Success(c, nil)
				})
			}

			// Files read (viewer+)
			fileHandler, err := NewFileManagerHandler(r.cfg.FileManager.BasePath, r.auditService)
			if err != nil {
				log.Fatalf("FATAL: failed to init file handler: %v", err)
			}
			protected.GET("/files", fileHandler.List)
			protected.GET("/files/base-path", fileHandler.GetBasePath)
			protected.GET("/files/download", fileHandler.Download)
			protected.GET("/files/content", fileHandler.GetContent)
			protected.GET("/files/search", fileHandler.Search)
			protected.GET("/files/search-content", fileHandler.SearchContent)
			protected.GET("/files/details", fileHandler.GetDetails)
			protected.GET("/files/mime-type", fileHandler.GetMimeType)

			// Files write (operator+)
			fileOp := protected.Group("")
			fileOp.Use(middleware.RequireRole("admin", "operator"))
			{
				fileOp.POST("/files/mkdir", fileHandler.Mkdir)
				fileOp.POST("/files/upload", fileHandler.Upload)
				fileOp.PUT("/files/rename", fileHandler.Rename)
				fileOp.DELETE("/files", fileHandler.Delete)
				fileOp.POST("/files/move", fileHandler.Move)
				fileOp.POST("/files/copy", fileHandler.Copy)
				fileOp.PUT("/files/content", fileHandler.SaveContent)
				fileOp.POST("/files/compress", fileHandler.Compress)
				fileOp.POST("/files/extract", fileHandler.Extract)
				fileOp.PUT("/files/chmod", fileHandler.Chmod)
				fileOp.PUT("/files/chown", fileHandler.Chown)
			}

			// Users (admin only)
			userHandler := NewUserHandler(r.db, r.authService, r.sessionService)
			userMgmt := protected.Group("/users")
			userMgmt.Use(middleware.RequireRole("admin"))
			{
				userMgmt.GET("", userHandler.List)
				userMgmt.POST("", userHandler.Create)
				userMgmt.PUT("/:id", userHandler.Update)
				userMgmt.DELETE("/:id", userHandler.Delete)
				userMgmt.POST("/:id/unlock", userHandler.Unlock)
				userMgmt.POST("/:id/reset-password", userHandler.ResetPassword)
				userMgmt.GET("/:id/activities", userHandler.GetActivities)
				userMgmt.PUT("/:id/expiry", userHandler.SetExpiry)
				userMgmt.PUT("/:id/ip-whitelist", userHandler.SetIPWhitelist)
			}

			// User sessions and activities (admin only)
			protected.GET("/users/sessions", middleware.RequireRole("admin"), userHandler.GetSessions)
			protected.GET("/users/activities", middleware.RequireRole("admin"), userHandler.GetAllActivities)

			// Audit logs (admin only)
			auditHandler := NewAuditHandler(r.db)
			auditGroup := protected.Group("/audit-logs")
			auditGroup.Use(middleware.RequireRole("admin"))
			{
				auditGroup.GET("", auditHandler.List)
				auditGroup.GET("/actions", auditHandler.GetActions)
				auditGroup.GET("/stats", auditHandler.Stats)
				auditGroup.GET("/clean-policy", auditHandler.GetCleanPolicy)
				auditGroup.GET("/export", auditHandler.Export)
				auditGroup.DELETE("/clean", auditHandler.Clean)
			}

			// Settings (admin only)
			settingsHandler := NewSettingsHandler(r.cfg)
			settingsGroup := protected.Group("/settings")
			settingsGroup.Use(middleware.RequireRole("admin"))
			{
				settingsGroup.GET("", settingsHandler.GetSettings)
				settingsGroup.GET("/system", settingsHandler.GetSystemInfo)
			}

			// System (admin only)
			systemHandler := NewSystemHandler()
			systemGroup := protected.Group("/system")
			systemGroup.Use(middleware.RequireRole("admin"))
			{
				systemGroup.GET("/ssh-logins", systemHandler.GetSSHLogins)
				systemGroup.GET("/ssh-config", systemHandler.GetSystemSSHConfig)
				systemGroup.GET("/check-port", systemHandler.CheckPort)
				systemGroup.GET("/check-ports", systemHandler.CheckPorts)
			}

			// Cloud read (viewer+)
			cloudHandler, err := NewCloudHandler(&r.cfg.TencentCloud)
			if err != nil {
				log.Printf("Warning: failed to init cloud handler: %v", err)
				// Register stub handlers that return friendly error
				cloudHandler = &CloudHandler{}
			}
			protected.GET("/cloud/instances", cloudHandler.GetInstances)
			protected.GET("/cloud/instances/:id", cloudHandler.GetInstance)
			protected.GET("/cloud/monitor/:id", cloudHandler.GetMonitorData)
			protected.GET("/cloud/firewall/:id", cloudHandler.GetFirewallRules)
			protected.GET("/cloud/snapshots", cloudHandler.GetSnapshots)
			protected.GET("/cloud/traffic", cloudHandler.GetTraffic)

			// Cloud write (operator+)
			cloudOp := protected.Group("")
			cloudOp.Use(middleware.RequireRole("admin", "operator"))
			{
				cloudOp.POST("/cloud/instances/:id/start", cloudHandler.StartInstance)
				cloudOp.POST("/cloud/instances/:id/stop", cloudHandler.StopInstance)
				cloudOp.POST("/cloud/instances/:id/restart", cloudHandler.RestartInstance)
				cloudOp.POST("/cloud/firewall/:id", cloudHandler.AddFirewallRule)
				cloudOp.DELETE("/cloud/firewall/:id/:ruleId", cloudHandler.DeleteFirewallRule)
				cloudOp.POST("/cloud/snapshots", cloudHandler.CreateSnapshot)
				cloudOp.POST("/cloud/snapshots/:id/apply", cloudHandler.ApplySnapshot)
			}

			// Deploy (admin only)
			deployService, err := service.NewDeployService(r.db, r.cfg.Deploy.EncryptionKey)
			if err != nil {
				log.Fatalf("FATAL: failed to init deploy service: %v", err)
			}
			deployService.InitTables()
			deployHandler := NewDeployHandler(deployService)
			deployMgmt := protected.Group("/deploy")
			deployMgmt.Use(middleware.RequireRole("admin"))
			{
				deployMgmt.GET("/servers", deployHandler.ListServers)
				deployMgmt.POST("/servers", deployHandler.CreateServer)
				deployMgmt.GET("/servers/:id", deployHandler.GetServer)
				deployMgmt.PUT("/servers/:id", deployHandler.UpdateServer)
				deployMgmt.DELETE("/servers/:id", deployHandler.DeleteServer)
				deployMgmt.POST("/servers/:id/test", deployHandler.TestConnection)
				deployMgmt.GET("/tasks", deployHandler.ListTasks)
				deployMgmt.POST("/tasks", deployHandler.CreateTask)
				deployMgmt.GET("/tasks/:id", deployHandler.GetTask)
				deployMgmt.DELETE("/tasks/:id", deployHandler.DeleteTask)
				deployMgmt.POST("/tasks/:id/exec", deployHandler.ExecuteTask)
				deployMgmt.GET("/versions", deployHandler.ListVersions)
				deployMgmt.POST("/versions/:id/rollback", deployHandler.RollbackVersion)
			}

			// Runtime environment management (admin only)
			runtimeService := service.NewRuntimeService(r.db)
			runtimeService.InitTables()
			runtimeHandler := NewRuntimeHandler(runtimeService)
			runtimeMgmt := protected.Group("/runtime")
			runtimeMgmt.Use(middleware.RequireRole("admin"))
			{
				runtimeMgmt.GET("", runtimeHandler.List)
				runtimeMgmt.GET("/:name", runtimeHandler.ListByName)
				runtimeMgmt.POST("/install", runtimeHandler.Install)
				runtimeMgmt.POST("/uninstall", runtimeHandler.Uninstall)
				runtimeMgmt.POST("/set-default", runtimeHandler.SetDefault)
				runtimeMgmt.GET("/detect", runtimeHandler.Detect)
				runtimeMgmt.POST("/import-detected", runtimeHandler.ImportDetected)
				runtimeMgmt.GET("/progress/:id", runtimeHandler.GetProgress)
				runtimeMgmt.GET("/check-deps/:name", runtimeHandler.CheckDependencies)
				runtimeMgmt.GET("/logs/:id", runtimeHandler.GetLogs)
				runtimeMgmt.GET("/cleanup/:id", runtimeHandler.GetCleanupInfo)
			}

			// Runtime version management (admin only)
			runtimeVersionService := service.NewRuntimeVersionService(r.db)
			runtimeVersionService.InitTables()
			runtimeVersionHandler := NewRuntimeVersionHandler(runtimeVersionService)
			runtimeVersionMgmt := protected.Group("/runtime-versions")
			runtimeVersionMgmt.Use(middleware.RequireRole("admin"))
			{
				runtimeVersionMgmt.GET("/:name", runtimeVersionHandler.List)
				runtimeVersionMgmt.POST("/:name/fetch", runtimeVersionHandler.Fetch)
				runtimeVersionMgmt.GET("/:name/resolve/:alias", runtimeVersionHandler.ResolveAlias)
				runtimeVersionMgmt.GET("/:name/suggestions", runtimeVersionHandler.GetAliasSuggestions)
			}

			// Package management (admin only)
			packageService := service.NewPackageManagerService(r.db)
			packageService.InitTables()
			packageHandler := NewPackageManagerHandler(packageService, runtimeService)
			packageMgmt := protected.Group("/packages")
			packageMgmt.Use(middleware.RequireRole("admin"))
			{
				packageMgmt.GET("", packageHandler.ListPackages)
				packageMgmt.GET("/scan/:id", packageHandler.ScanPackages)
				packageMgmt.GET("/search", packageHandler.SearchPackages)
				packageMgmt.GET("/versions/:name", packageHandler.GetPackageVersions)
				packageMgmt.POST("/install", packageHandler.InstallPackage)
				packageMgmt.POST("/uninstall", packageHandler.UninstallPackage)
				packageMgmt.POST("/update", packageHandler.UpdatePackage)
			}

			// Environment configuration (admin only)
			envConfigService := service.NewEnvConfigService(r.db)
			envConfigService.InitTables()
			envConfigHandler := NewEnvConfigHandler(envConfigService)
			envConfigMgmt := protected.Group("/env-config")
			envConfigMgmt.Use(middleware.RequireRole("admin"))
			{
				envConfigMgmt.GET("", envConfigHandler.ListEnvConfigs)
				envConfigMgmt.GET("/:id", envConfigHandler.GetEnvConfig)
				envConfigMgmt.POST("", envConfigHandler.CreateEnvConfig)
				envConfigMgmt.PUT("/:id", envConfigHandler.UpdateEnvConfig)
				envConfigMgmt.DELETE("/:id", envConfigHandler.DeleteEnvConfig)
				envConfigMgmt.GET("/path", envConfigHandler.ListPathEntries)
				envConfigMgmt.POST("/path", envConfigHandler.CreatePathEntry)
				envConfigMgmt.DELETE("/path/:id", envConfigHandler.DeletePathEntry)
				envConfigMgmt.GET("/script", envConfigHandler.GenerateEnvScript)
			}

			// Global configuration (admin only)
			globalConfigMgmt := protected.Group("/global-config")
			globalConfigMgmt.Use(middleware.RequireRole("admin"))
			{
				globalConfigMgmt.GET("", envConfigHandler.ListGlobalConfigs)
				globalConfigMgmt.GET("/:id", envConfigHandler.GetGlobalConfig)
				globalConfigMgmt.POST("", envConfigHandler.CreateGlobalConfig)
				globalConfigMgmt.PUT("/:id", envConfigHandler.UpdateGlobalConfig)
				globalConfigMgmt.DELETE("/:id", envConfigHandler.DeleteGlobalConfig)
			}

			// Web Server + Website management (admin only)
			webServerService := service.NewWebServerService(r.db)
			webServerService.InitTables()
			websiteService := service.NewWebsiteService(r.db)
			websiteService.InitTables()
			webServerHandler := NewWebServerHandler(webServerService, websiteService)
			webServerMgmt := protected.Group("/web-servers")
			webServerMgmt.Use(middleware.RequireRole("admin"))
			{
				// Utilities (must be before /:id to avoid conflict)
				webServerMgmt.GET("/project-types", webServerHandler.GetProjectTypes)
				webServerMgmt.GET("/browse", webServerHandler.BrowseDirs)
				webServerMgmt.GET("/validate-path", webServerHandler.ValidatePath)
				// Web server CRUD
				webServerMgmt.GET("", webServerHandler.List)
				webServerMgmt.GET("/:id", webServerHandler.Get)
				webServerMgmt.POST("", webServerHandler.Create)
				webServerMgmt.DELETE("/:id", webServerHandler.Delete)
				webServerMgmt.POST("/:id/install", webServerHandler.Install)
				webServerMgmt.POST("/:id/uninstall", webServerHandler.Uninstall)
				webServerMgmt.POST("/:id/start", webServerHandler.Start)
				webServerMgmt.POST("/:id/stop", webServerHandler.Stop)
				webServerMgmt.POST("/:id/restart", webServerHandler.Restart)
				webServerMgmt.GET("/:id/status", webServerHandler.Status)
				webServerMgmt.POST("/:id/reload", webServerHandler.Reload)
				webServerMgmt.GET("/:id/test-config", webServerHandler.TestConfig)
				webServerMgmt.GET("/:id/config", webServerHandler.GetConfig)
				webServerMgmt.PUT("/:id/config", webServerHandler.SaveConfig)
				webServerMgmt.GET("/:id/logs", webServerHandler.GetServiceLogs)
				webServerMgmt.POST("/:id/auto-start", webServerHandler.SetAutoStart)
				webServerMgmt.GET("/:id/process", webServerHandler.GetProcessInfo)
				// Websites nested under web server (:id = server, :wid = website)
				webServerMgmt.GET("/:id/websites", webServerHandler.ListWebsites)
				webServerMgmt.GET("/:id/websites/:wid", webServerHandler.GetWebsite)
				webServerMgmt.POST("/:id/websites", webServerHandler.CreateWebsite)
				webServerMgmt.PUT("/:id/websites/:wid", webServerHandler.UpdateWebsite)
				webServerMgmt.DELETE("/:id/websites/:wid", webServerHandler.DeleteWebsite)
				webServerMgmt.POST("/:id/websites/:wid/enable", webServerHandler.EnableWebsite)
				webServerMgmt.POST("/:id/websites/:wid/disable", webServerHandler.DisableWebsite)
				webServerMgmt.GET("/:id/websites/:wid/logs", webServerHandler.GetWebsiteLogs)
				webServerMgmt.POST("/:id/websites/:wid/ssl", webServerHandler.ApplyWebsiteSSL)
			}

			// Database management (admin only)
			dbServerService := service.NewDBServerService(r.db)
			dbServerService.InitTables()
			dbMgmtService := service.NewDatabaseMgmtService(r.db)
			dbServerHandler := NewDBServerHandler(dbServerService, dbMgmtService)
			dbServerMgmt := protected.Group("/db-servers")
			dbServerMgmt.Use(middleware.RequireRole("admin"))
			{
				dbServerMgmt.GET("", dbServerHandler.List)
				dbServerMgmt.GET("/:id", dbServerHandler.Get)
				// Version management
				dbServerMgmt.GET("/:id/version-templates", dbServerHandler.GetVersionTemplates)
				dbServerMgmt.GET("/:id/versions", dbServerHandler.ListVersions)
				dbServerMgmt.POST("/:id/versions", dbServerHandler.InstallVersion)
				dbServerMgmt.DELETE("/versions/:vid", dbServerHandler.UninstallVersion)
				dbServerMgmt.POST("/versions/:vid/start", dbServerHandler.StartVersion)
				dbServerMgmt.POST("/versions/:vid/stop", dbServerHandler.StopVersion)
				dbServerMgmt.POST("/versions/:vid/restart", dbServerHandler.RestartVersion)
				dbServerMgmt.PUT("/versions/:vid/port", dbServerHandler.UpdateVersionPort)
				dbServerMgmt.GET("/versions/:vid/logs", dbServerHandler.GetVersionLogs)
				// Databases nested
				dbServerMgmt.GET("/:id/databases", dbServerHandler.ListDatabases)
				dbServerMgmt.POST("/:id/databases", dbServerHandler.CreateDatabase)
				dbServerMgmt.DELETE("/:id/databases/:did", dbServerHandler.DeleteDatabase)
				// DB Users nested
				dbServerMgmt.GET("/:id/users", dbServerHandler.ListDBUsers)
				dbServerMgmt.POST("/:id/users", dbServerHandler.CreateDBUser)
				dbServerMgmt.DELETE("/:id/users/:uid", dbServerHandler.DeleteDBUser)
				dbServerMgmt.POST("/:id/users/:uid/grant", dbServerHandler.GrantPrivileges)
				// Database introspection
				dbServerMgmt.GET("/databases/:did/tables", dbServerHandler.ListTables)
				dbServerMgmt.GET("/databases/:did/describe", dbServerHandler.DescribeTable)
				dbServerMgmt.GET("/databases/:did/query", dbServerHandler.QueryTable)
				dbServerMgmt.POST("/databases/:did/execute", dbServerHandler.ExecuteSQL)
				dbServerMgmt.POST("/databases/:did/insert", dbServerHandler.InsertRecord)
				dbServerMgmt.POST("/databases/:did/update", dbServerHandler.UpdateRecord)
				dbServerMgmt.POST("/databases/:did/delete", dbServerHandler.DeleteRecord)
				// MySQL config management
				dbServerMgmt.GET("/mysql/config", dbServerHandler.GetMySQLConfig)
				dbServerMgmt.POST("/mysql/config", dbServerHandler.SaveMySQLConfig)
				dbServerMgmt.GET("/mysql/common-params", dbServerHandler.GetMySQLCommonParams)
				// PostgreSQL config management
				dbServerMgmt.GET("/postgresql/config", dbServerHandler.GetPostgreSQLConfig)
				dbServerMgmt.POST("/postgresql/config", dbServerHandler.SavePostgreSQLConfig)
				dbServerMgmt.GET("/postgresql/common-params", dbServerHandler.GetPGCommonParams)
				// Redis config management
				dbServerMgmt.GET("/redis/config", dbServerHandler.GetRedisConfig)
				dbServerMgmt.POST("/redis/config", dbServerHandler.SaveRedisConfig)
				dbServerMgmt.GET("/redis/common-params", dbServerHandler.GetRedisCommonParams)
			}
		}
	}

	// WebSocket routes (unified authentication)
	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator))
	{
		// Monitor WebSocket (viewer+)
		monitorHandler := NewMonitorHandler(r.monitorService, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
		wsGroup.GET("/monitor", middleware.RequireWSRole("admin", "operator", "viewer"), monitorHandler.HandleWebSocket)

		// Terminal WebSocket (operator+)
		terminalHandler := NewTerminalHandler(r.cfg.Auth.JWTSecret, r.auditService, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
		wsGroup.GET("/terminal/:id", middleware.RequireWSRole("admin", "operator"), terminalHandler.HandleWebSocket)

		// Service logs WebSocket (viewer+)
		serviceLogHandler := NewServiceHandler(r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
		wsGroup.GET("/services/:name/logs", middleware.RequireWSRole("admin", "operator", "viewer"), serviceLogHandler.HandleLogsWebSocket)
	}

	// Serve embedded frontend (production mode)
	if r.cfg.Server.ServeFrontend {
		ServeWeb(e)
	}

	return e
}