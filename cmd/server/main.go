package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/api"
	"easyserver/internal/config"
	"easyserver/internal/database"
	"easyserver/internal/executor"
	"easyserver/internal/middleware"
	"easyserver/internal/model"
	"easyserver/internal/repository/sqlite"
	"easyserver/internal/service"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	devMode := flag.Bool("dev", false, "run in development mode (no embed, API only)")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Check for weak JWT secret
	if len(cfg.Auth.JWTSecret) < 32 {
		if !*devMode {
			log.Println("ERROR: JWT secret must be at least 32 bytes.")
			log.Println("Please set a strong secret in config.yaml or use EASYSERVER_JWT_SECRET environment variable.")
			os.Exit(1)
		}
		log.Println("WARNING: JWT secret is too short. Using in development mode only!")
	}

	// Check for default JWT secret
	defaultSecrets := []string{
		"easyserver-secret-key-change-me",
		"change-me-to-a-random-secret",
		"change-me-to-a-random-secret-at-least-32-bytes-long",
	}
	for _, defaultSecret := range defaultSecrets {
		if cfg.Auth.JWTSecret == defaultSecret {
			if !*devMode {
				log.Println("ERROR: JWT secret is set to a default value.")
				log.Println("Please set a strong secret in config.yaml or use EASYSERVER_JWT_SECRET environment variable.")
				os.Exit(1)
			}
			log.Println("WARNING: Using default JWT secret in development mode. Change it for production!")
			break
		}
	}

	// Validate file manager base path
	if cfg.FileManager.BasePath == "" {
		log.Fatalf("FATAL: filemanager.base_path is required")
	}
	if cfg.FileManager.BasePath == "/" {
		log.Fatalf("FATAL: filemanager.base_path cannot be '/' for security reasons")
	}

	// Check for default deploy encryption key
	defaultDeployKeys := []string{
		"change-me-to-a-random-32-byte-key!!",
		"change-me-to-a-random-32-byte-key",
	}
	for _, defaultKey := range defaultDeployKeys {
		if cfg.Deploy.EncryptionKey == defaultKey {
			if !*devMode {
				log.Println("ERROR: deploy.encryption_key is set to a default value.")
				log.Println("Please set a strong key in config.yaml or use EASYSERVER_ENCRYPTION_KEY environment variable.")
				os.Exit(1)
			}
			log.Println("WARNING: Using default deploy encryption key in development mode. Change it for production!")
			break
		}
	}

	// Initialize database
	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize repositories (single source of truth for data access)
	userRepo := sqlite.NewUserRepository(db)
	sessionRepo := sqlite.NewSessionRepository(db)
	tokenRepo := sqlite.NewTokenBlacklistRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	activityRepo := sqlite.NewActivityRepository(db)
	totpRepo := sqlite.NewTOTPRepository(db)
	monitorRepo := sqlite.NewMonitorRepository(db)

	// Initialize auth service and default admin (single shared instance)
	authService := service.NewAuthService(cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutDuration)
	authService.SetRepositories(userRepo, tokenRepo, activityRepo, totpRepo)
	if err := authService.InitDefaultAdmin(context.Background()); err != nil {
		log.Fatalf("Failed to initialize default admin: %v", err)
	}

	// Initialize monitor service with WaitGroup for proper shutdown
	var monitorWg sync.WaitGroup
	monitorService := service.NewMonitorService(monitorRepo, cfg.Monitor.CollectInterval, cfg.Monitor.HistoryRetention)
	monitorWg.Add(1)
	go func() {
		defer monitorWg.Done()
		monitorService.Start()
	}()

	// Initialize shared command executor
	cmdExec := executor.NewOSExecutor()

	// Initialize audit service and system event monitor (single shared instance)
	auditService := service.NewAuditService(db, auditRepo, cfg.Audit.RetentionDays)
	systemMonitor := service.NewSystemEventMonitor(auditService, cmdExec)
	systemMonitor.Start()

	// Initialize session service and start cleanup (single shared instance)
	sessionService := service.NewSessionService(sessionRepo)
	sessionDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(cfg.Auth.SessionCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sessionService.CleanupExpiredSessions(context.Background()); err != nil {
					log.Printf("session cleanup error: %v", err)
				}
			case <-sessionDone:
				return
			}
		}
	}()

	// Initialize process guardian (single shared instance)
	processRepo := sqlite.NewProcessRepository(db)
	processManager := service.NewProcessManager(processRepo, cmdExec)

	// Initialize system process service (single shared instance)
	serviceWhitelistRepo := sqlite.NewServiceWhitelistRepository(db)
	systemProcessService := service.NewSystemProcessService(cmdExec, serviceWhitelistRepo, auditService)

	// Initialize notification service (single shared instance)
	notificationRepo := sqlite.NewNotificationRepository(db)
	notificationService := service.NewNotificationService(notificationRepo)

	// Initialize TOTP service (single shared instance)
	totpService := service.NewTOTPService(totpRepo)

	// Initialize container services (single shared instance)
	containerService := service.NewContainerService(cmdExec)
	dockerService := service.NewDockerService(cmdExec)
	composeService := service.NewComposeService(cmdExec)
	volumeService := service.NewVolumeService(cmdExec)
	networkService := service.NewNetworkService(cmdExec)

	// Initialize service manager (single shared instance)
	serviceManager := service.NewServiceManager(cmdExec)

	// Initialize cron service (single shared instance)
	cronRepo := sqlite.NewCronRepository(db)
	cronService := service.NewCronService(cronRepo, cmdExec)

	// Initialize database services (single shared instance)
	dbServerRepo := sqlite.NewDBServerRepository(db)
	dbServerService := service.NewDBServerService(cmdExec, dbServerRepo)
	dbServerService.SeedPredefinedServers(context.Background())
	databaseMgmtRepo := sqlite.NewDatabaseMgmtRepository(db)
	databaseMgmtService := service.NewDatabaseMgmtService(databaseMgmtRepo, cmdExec)
	dbBackupRepo := sqlite.NewDBBackupRepository(db)
	dbBackupService := service.NewDBBackupService(dbBackupRepo, cmdExec)
	sqlQueryService := service.NewSQLQueryService(databaseMgmtService, cmdExec)

	// Initialize deploy service (single shared instance)
	deployRepo := sqlite.NewDeployRepository(db)
	deployService, err := service.NewDeployService(deployRepo, cfg.Deploy.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to init deploy service: %v", err)
	}

	// Initialize environment config service (single shared instance)
	envConfigRepo := sqlite.NewEnvConfigRepository(db)
	envConfigService := service.NewEnvConfigService(envConfigRepo)
	envConfigService.InitDefaultGlobalConfigs(context.Background())

	// Initialize firewall service (single shared instance)
	firewallRepo := sqlite.NewFirewallRepository(db)
	firewallService := service.NewFirewallService(firewallRepo, cmdExec)

	// Initialize runtime services (single shared instance)
	runtimeRepo := sqlite.NewRuntimeRepository(db)
	runtimeService := service.NewRuntimeService(db, cmdExec)
	runtimeVersionService := service.NewRuntimeVersionService(runtimeRepo)
	packageRepo := sqlite.NewPackageRepository(db)
	packageManagerService := service.NewPackageManagerService(packageRepo, cmdExec)

	// Initialize SSH service (single shared instance)
	sshConfigService := service.NewSSHConfigService(cmdExec)

	// Initialize web server services (single shared instance)
	webServerRepo := sqlite.NewWebServerRepository(db)
	websiteRepo := sqlite.NewWebsiteRepository(db)
	webServerService := service.NewWebServerService(webServerRepo, cmdExec)
	webServerService.SeedPredefinedWebServers(context.Background())
	websiteService := service.NewWebsiteService(websiteRepo, webServerRepo, cmdExec)

	// Initialize notify + alert services (single shared instance)
	notifyService := service.NewNotifyService(cfg.Notify.WebhookURL, cfg.Notify.Enabled)
	authService.SetNotifyService(notifyService)

	alertService := service.NewAlertService(notifyService)
	var alertRules []model.AlertRule
	for i, rule := range cfg.Alerts.Rules {
		alertRules = append(alertRules, model.AlertRule{
			ID:        int64(i + 1),
			Name:      rule.Name,
			Metric:    rule.Metric,
			Threshold: rule.Threshold,
			Duration:  rule.Duration,
			Enabled:   rule.Enabled,
		})
	}
	alertService.SetRules(alertRules)
	monitorService.SetAlertService(alertService)

	// Initialize terminal manager (single shared instance)
	terminalManager := service.NewTerminalManager(cmdExec)

	// Initialize file manager (single shared instance)
	fileManager, err := service.NewFileManager(cfg.FileManager.BasePath)
	if err != nil {
		log.Fatalf("Failed to init file manager: %v", err)
	}

	// Initialize cloud service (single shared instance, nil if disabled)
	var cloudService *service.CloudService
	if cfg.TencentCloud.Enabled {
		cloudService, err = service.NewCloudService(
			cfg.TencentCloud.SecretID,
			cfg.TencentCloud.SecretKey,
			cfg.TencentCloud.Region,
			cfg.TencentCloud.InstanceID,
		)
		if err != nil {
			log.Printf("WARNING: failed to init cloud service: %v", err)
		}
	}

	// Log server start
	auditService.LogSystemEvent(context.Background(), "SERVER_START", "EasyServer started")

	// Setup router with shared service instances (no duplicate creation)
	router := api.NewRouter(cfg, *configPath, api.RouterDeps{
		DB:                   db,
		Executor:             cmdExec,
		AuthService:          authService,
		MonitorService:       monitorService,
		AuditService:         auditService,
		SessionService:       sessionService,
		TotpService:          totpService,
		AuditRepo:            auditRepo,
		ProcessManager:       processManager,
		SystemProcessService: systemProcessService,
		NotificationService:  notificationService,
		ServiceManager:       serviceManager,

		// Container services
		ContainerService: containerService,
		DockerService:    dockerService,
		ComposeService:   composeService,
		VolumeService:    volumeService,
		NetworkService:   networkService,

		// Cron service
		CronService: cronService,

		// Database services
		DBServerService:     dbServerService,
		DatabaseMgmtService: databaseMgmtService,
		DBBackupService:     dbBackupService,
		SQLQueryService:     sqlQueryService,

		// Deploy service
		DeployService: deployService,

		// Environment config service
		EnvConfigService: envConfigService,

		// Firewall service
		FirewallService: firewallService,

		// Runtime services
		RuntimeService:        runtimeService,
		RuntimeVersionService: runtimeVersionService,
		PackageManagerService: packageManagerService,

		// SSH service
		SSHConfigService: sshConfigService,

		// Web server services
		WebServerService: webServerService,
		WebsiteService:   websiteService,

		// Notify + Alert services
		NotifyService: notifyService,
		AlertService:  alertService,

		// Terminal manager
		TerminalManager: terminalManager,

		// File manager
		FileManager: fileManager,

		// Cloud service
		CloudService: cloudService,
	})
	r := router.Setup()

	// Create HTTP server with graceful shutdown support
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if *devMode {
		log.Printf("Starting EasyServer in DEV mode on %s", addr)
		log.Printf("Frontend should be served separately on port 5173")
	} else {
		log.Printf("Starting EasyServer on %s", addr)
	}

	// Start server in goroutine
	go func() {
		if cfg.Server.TLS.Enabled {
			if err := srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server gracefully
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Stop rate limiter background goroutine
	middleware.StopRateLimiter()

	// Stop session heartbeat limiter background goroutine
	middleware.StopSessionHeartbeatLimiter()

	// Stop session cleanup goroutine
	close(sessionDone)

	// Stop system event monitor
	systemMonitor.Stop()

	// Log server shutdown
	auditService.LogSystemEvent(context.Background(), "SERVER_STOP", "EasyServer stopped")

	// Flush remaining audit log entries
	auditService.Close()

	// Stop monitor service
	monitorService.Stop()
	monitorWg.Wait()

	// Stop process guardian
	processManager.Shutdown()

	log.Println("Server exited properly")
}
