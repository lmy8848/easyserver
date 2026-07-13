package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/alert"
	"easyserver/internal/api"
	"easyserver/internal/api/middleware"
	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/cloud"
	"easyserver/internal/container"
	"easyserver/internal/cron"
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
	"easyserver/internal/deploy"
	"easyserver/internal/envconfig"
	"easyserver/internal/filemanager"
	"easyserver/internal/fileshare"
	"easyserver/internal/firewall"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
	"easyserver/internal/infra/executor"
	"easyserver/internal/monitor"
	"easyserver/internal/notification"
	"easyserver/internal/notify"
	"easyserver/internal/process"
	"easyserver/internal/qrlogin"
	"easyserver/internal/runtimeenv"
	"easyserver/internal/ssh"
	"easyserver/internal/systemd"
	"easyserver/internal/systemprocess"
	"easyserver/internal/terminal"
	"easyserver/internal/web"
)

// validateConfig fails fast on insecure config values. In dev mode most errors
// downgrade to warnings; empty secrets and missing required paths still abort.
func validateConfig(cfg *config.Config, devMode bool) {
	// JWT secret — empty always rejected
	if cfg.Auth.JWTSecret == "" {
		log.Fatalln("ERROR: JWT secret is empty. Set a strong secret via EASYSERVER_JWT_SECRET env var (32+ chars) or auth.jwt_secret in config.yaml.")
	}
	rejectOrWarn(devMode, len(cfg.Auth.JWTSecret) < 32,
		"JWT secret must be at least 32 bytes. Use: EASYSERVER_JWT_SECRET=$(openssl rand -base64 32)")

	knownWeakJWT := []string{
		"easyserver-secret-key-change-me",
		"change-me-to-a-random-secret",
		"change-me-to-a-random-secret-at-least-32-bytes-long",
		"secret",
		"password",
		"12345678901234567890123456789012",
	}
	for _, weak := range knownWeakJWT {
		if cfg.Auth.JWTSecret == weak {
			rejectOrWarn(devMode, true,
				"JWT secret is set to a well-known default value.")
			break
		}
	}
	if len(cfg.Auth.JWTSecret) >= 32 && uniformString(cfg.Auth.JWTSecret) {
		rejectOrWarn(devMode, true,
			"JWT secret has no entropy (all same character).")
	}

	// Rate limit guards
	rejectOrWarn(devMode, cfg.Auth.RateLimit < 10 || cfg.Auth.RateLimit > 100000,
		"auth.rate_limit must be between 10 and 100000")
	rejectOrWarn(devMode, cfg.Auth.RateInterval < time.Second || cfg.Auth.RateInterval > time.Hour,
		"auth.rate_interval must be between 1s and 1h")

	rejectOrWarn(devMode, cfg.Auth.LoginRateLimit < 1 || cfg.Auth.LoginRateLimit > 100,
		"auth.login_rate_limit must be between 1 and 100")
	rejectOrWarn(devMode, cfg.Auth.LoginRateInterval < time.Second || cfg.Auth.LoginRateInterval > time.Hour,
		"auth.login_rate_interval must be between 1s and 1h")

	rejectOrWarn(devMode, cfg.Server.AssetsRateLimit < 100 || cfg.Server.AssetsRateLimit > 100000,
		"server.assets_rate_limit must be between 100 and 100000")
	rejectOrWarn(devMode, cfg.Server.AssetsRateInterval < time.Second || cfg.Server.AssetsRateInterval > time.Hour,
		"server.assets_rate_interval must be between 1s and 1h")

	// File manager
	if cfg.FileManager.BasePath == "" {
		log.Fatalln("FATAL: filemanager.base_path is required")
	}
	if cfg.FileManager.BasePath == "/" {
		log.Println("WARNING: FileManager BasePath is '/' (full root access).")
	}

	// Deploy encryption key
	if cfg.Deploy.EncryptionKey == "" {
		rejectOrWarn(devMode, true,
			"deploy.encryption_key is required. Set via EASYSERVER_ENCRYPTION_KEY env var (32+ chars) or deploy.encryption_key in config.yaml.")
	}
	knownWeakDeployKeys := []string{
		"change-me-to-a-random-32-byte-key!!",
		"change-me-to-a-random-32-byte-key",
	}
	for _, weak := range knownWeakDeployKeys {
		if cfg.Deploy.EncryptionKey == weak {
			rejectOrWarn(devMode, true,
				"deploy.encryption_key is set to a well-known default value.")
			break
		}
	}
}

func rejectOrWarn(devMode, fail bool, msg string) {
	if !fail {
		return
	}
	if devMode {
		log.Println("WARNING:", msg, "(allowed only in dev mode)")
		return
	}
	log.Fatalln("ERROR:", msg)
}

func uniformString(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

// appServices bundles every running service for the HTTP server, plus a LIFO
// teardown stack populated during wire().
type appServices struct {
	api.RouterDeps
	cleanups []func()
}

func (s *appServices) onCleanup(fn func()) {
	s.cleanups = append(s.cleanups, fn)
}

// Shutdown runs registered cleanups in LIFO order.
func (s *appServices) Shutdown() {
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
}

// wire builds every service from cfg, starts the required background goroutines,
// and registers their cleanups. Caller must Shutdown() the result.
func wire(cfg *config.Config) (*appServices, error) {
	if err := runtimeenv.BootstrapMise(); err != nil {
		log.Printf("ERROR: Failed to bootstrap mise runtime manager: %v", err)
	}

	s := &appServices{}
	ctx := context.Background()

	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}
	s.DB = db
	s.onCleanup(func() { db.Close() })

	// Repositories — only auditRepo is exposed downstream.
	userRepo := auth.NewSQLiteUserRepository(db)
	sessionRepo := auth.NewSQLiteSessionRepository(db)
	tokenRepo := auth.NewSQLiteTokenRepository(db)
	auditRepo := audit.NewSQLiteRepository(db)
	activityRepo := auth.NewSQLiteActivityRepository(db)
	totpRepo := auth.NewTOTPRepository(db)
	monitorRepo := monitor.NewSQLiteRepository(db)
	s.AuditRepo = auditRepo

	// Auth + default admin
	authSvc := auth.NewAuthService(cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutDuration)
	authSvc.SetRepositories(userRepo, tokenRepo, activityRepo, totpRepo)
	if err := authSvc.InitDefaultAdmin(ctx); err != nil {
		return nil, fmt.Errorf("init default admin: %w", err)
	}
	s.AuthService = authSvc

	// Monitor service + collection goroutine
	var monitorWg sync.WaitGroup
	monitorSvc := monitor.NewMonitorService(monitorRepo, cfg.Monitor.CollectInterval, cfg.Monitor.HistoryRetention)
	monitorWg.Add(1)
	infra.Go(func() {
		defer monitorWg.Done()
		monitorSvc.Start()
	})
	s.MonitorService = monitorSvc
	s.onCleanup(func() {
		monitorSvc.Stop()
		monitorWg.Wait()
	})

	// Shared command executor
	cmdExec := executor.NewOSExecutor()
	s.Executor = cmdExec

	// Audit + system event monitor — audit cleanup writes SERVER_STOP then flushes.
	auditSvc := audit.NewService(db, auditRepo, cfg.Audit.RetentionDays)
	s.AuditService = auditSvc
	s.onCleanup(func() {
		auditSvc.LogSystemEvent(context.Background(), "面板停止")
		auditSvc.Close()
	})

	systemMonitor := monitor.NewEventMonitor(auditSvc, cmdExec)
	systemMonitor.Start()
	s.onCleanup(func() { systemMonitor.Stop() })

	// Session service + cleanup goroutine
	sessionSvc := auth.NewSessionService(sessionRepo)
	s.SessionService = sessionSvc

	// QR login service (scan-to-login): depends on session service for coexisting web sessions.
	s.QRLoginService = qrlogin.NewService(qrlogin.NewSQLiteRepository(db), cfg.Auth.JWTSecret, cfg.Auth.SessionTimeout, sessionSvc)
	sessionDone := make(chan struct{})
	infra.Go(func() {
		ticker := time.NewTicker(cfg.Auth.SessionCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sessionSvc.CleanupExpiredSessions(context.Background()); err != nil {
					log.Printf("session cleanup error: %v", err)
				}
			case <-sessionDone:
				return
			}
		}
	})
	s.onCleanup(func() { close(sessionDone) })

	// Package-level rate limiters
	s.onCleanup(middleware.StopRateLimiter)
	s.onCleanup(middleware.StopSessionHeartbeatLimiter)

	// Process guardian
	processRepo := process.NewSQLiteRepository(db)
	processMgr := process.NewService(processRepo, cmdExec)
	s.ProcessManager = processMgr
	s.onCleanup(func() { processMgr.Shutdown() })

	// System process service
	s.SystemProcessService = systemprocess.NewService()

	// Notification
	notificationRepo := notification.NewSQLiteRepository(db)
	s.NotificationService = notification.NewService(notificationRepo)

	// Container, systemd, cron
	s.ContainerService = container.NewService(cmdExec)
	s.ServiceManager = systemd.NewServiceManager(cmdExec, ".")
	cronRepo := cron.NewSQLiteRepository(db)
	s.CronService = cron.NewService(cronRepo, cmdExec)

	// Database services (with seeding)
	dbServerRepo := dbserver.NewSQLiteRepository(db)
	dbServerService := dbserver.NewService(cmdExec, dbServerRepo)
	dbServerService.SeedPredefinedServers(ctx)
	s.DBServerService = dbServerService

	databaseMgmtRepo := database_mgmt.NewSQLiteRepository(db)
	s.DatabaseMgmtService = database_mgmt.NewService(databaseMgmtRepo, cmdExec)

	// Deploy
	deployRepo := deploy.NewSQLiteRepository(db)
	deploySvc, err := deploy.NewService(deployRepo, cfg.Deploy.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init deploy service: %w", err)
	}
	s.DeployService = deploySvc

	// Env config (with seeding)
	envConfigRepo := envconfig.NewSQLiteRepository(db)
	envConfigService := envconfig.NewService(envConfigRepo)
	s.EnvConfigService = envConfigService

	// Firewall (protect SSH + panel port from being accidentally blocked)
	firewallRepo := firewall.NewSQLiteRepository(db)
	s.FirewallService = firewall.NewService(firewallRepo, cmdExec, cfg.Server.Port)

	// Runtime + package
	runtimeRepo := runtimeenv.NewSQLiteRepository(db)
	runtimeService := runtimeenv.NewService(runtimeRepo, cmdExec, envConfigService)
	if err := runtimeService.Init(ctx); err != nil {
		log.Printf("ERROR: Failed to init runtime service: %v", err)
	}
	s.RuntimeService = runtimeService
	s.PackageManagerService = runtimeenv.NewPackageService(cmdExec)

	// SSH
	s.SSHConfigService = ssh.NewService(cmdExec)

	// Web server + websites (with seed)
	webServerRepo := web.NewSQLiteServerRepository(db)
	websiteRepo := web.NewSQLiteWebsiteRepository(db)
	webServerSvc := web.NewService(webServerRepo, cmdExec)
	webServerSvc.SeedPredefinedWebServers(ctx)
	s.WebServerService = webServerSvc
	s.WebsiteService = web.NewWebsiteService(websiteRepo, webServerRepo, cmdExec)

	// Notify + alert (wired into auth + monitor)
	notifyService := notify.NewService(cfg.Notify.WebhookURL, cfg.Notify.Enabled)
	authSvc.SetNotifyService(notifyService)
	s.NotifyService = notifyService

	alertService := alert.NewService(notifyService, s.NotificationService)
	alertRules := make([]alert.AlertRule, 0, len(cfg.Alerts.Rules))
	for i, rule := range cfg.Alerts.Rules {
		alertRules = append(alertRules, alert.AlertRule{
			ID:        int64(i + 1),
			Name:      rule.Name,
			Metric:    rule.Metric,
			Threshold: rule.Threshold,
			Duration:  rule.Duration,
			Enabled:   rule.Enabled,
		})
	}
	alertService.SetRules(alertRules)
	monitorSvc.SetAlertService(alertService)
	s.AlertService = alertService

	// Terminal manager
	terminalManager := terminal.NewManager(cmdExec)
	terminalManager.StartIdleTimeout(cfg.Auth.IdleTimeout)
	s.TerminalManager = terminalManager
	s.onCleanup(func() { terminalManager.StopIdleTimeout() })

	// File manager
	fileManager, err := filemanager.NewManager(cfg.FileManager.BasePath)
	if err != nil {
		return nil, fmt.Errorf("init file manager: %w", err)
	}
	s.FileManager = fileManager

	// File share repository + startup cleanup
	s.FileShareRepo = fileshare.NewSQLiteShareRepository(db)
	if deleted, err := s.FileShareRepo.DeleteExpired(ctx); err != nil {
		log.Printf("ERROR: failed to cleanup expired file shares on startup: %v", err)
	} else if deleted > 0 {
		log.Printf("Cleaned up %d expired file shares on startup", deleted)
	}

	// Cloud service (optional)
	if cfg.TencentCloud.Enabled {
		cloudService, err := cloud.NewService(
			cfg.TencentCloud.SecretID,
			cfg.TencentCloud.SecretKey,
			cfg.TencentCloud.Region,
			cfg.TencentCloud.InstanceID,
		)
		if err != nil {
			log.Printf("WARNING: failed to init cloud service: %v", err)
		} else {
			s.CloudService = cloudService
		}
	}

	auditSvc.LogSystemEvent(ctx, "面板启动")
	return s, nil
}
