package api

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"easyserver/internal/alert"
	"easyserver/internal/audit"
	audithttp "easyserver/internal/audit/http"
	"easyserver/internal/auth"
	authhttp "easyserver/internal/auth/http"
	"easyserver/internal/cloud"
	cloudhttp "easyserver/internal/cloud/http"
	"easyserver/internal/container"
	containerhttp "easyserver/internal/container/http"
	"easyserver/internal/cron"
	cronhttp "easyserver/internal/cron/http"
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
	dbserverhttp "easyserver/internal/dbserver/http"
	"easyserver/internal/deploy"
	deployhttp "easyserver/internal/deploy/http"
	"easyserver/internal/envconfig"
	envconfighttp "easyserver/internal/envconfig/http"
	"easyserver/internal/filemanager"
	filemanagerhttp "easyserver/internal/filemanager/http"
	"easyserver/internal/firewall"
	firewallhttp "easyserver/internal/firewall/http"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
	"easyserver/internal/infra/executor"
	"easyserver/internal/infra/mise"
	"easyserver/internal/monitor"
	monitorhttp "easyserver/internal/monitor/http"
	"easyserver/internal/notification"
	notificationhttp "easyserver/internal/notification/http"
	"easyserver/internal/notify"
	"easyserver/internal/qrlogin"
	"easyserver/internal/runtimeenv"
	runtimeenvhttp "easyserver/internal/runtimeenv/http"
	"easyserver/internal/security"
	securityhttp "easyserver/internal/security/http"
	settingshttp "easyserver/internal/settings/http"
	"easyserver/internal/ssh"
	sshhttp "easyserver/internal/ssh/http"
	"easyserver/internal/systemd"
	systemdhttp "easyserver/internal/systemd/http"
	"easyserver/internal/terminal"
	terminalhttp "easyserver/internal/terminal/http"
	"easyserver/internal/web"
	webhttp "easyserver/internal/web/http"
	websecurity "easyserver/internal/web/security"
	websecurityhttp "easyserver/internal/web/security/http"

	"github.com/gin-gonic/gin"
)

// Setup constructs the entire application service graph, registers all HTTP
// routes, and returns the handler plus a shutdown function. It is the single
// composition root for the HTTP layer.
func Setup(cfg *config.Config, configPath string, sig *infra.Signal) (http.Handler, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// ── Infrastructure ──

	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	cmdExec := executor.NewOSExecutor()
	miseProvider := mise.NewProvider()

	// ── Shared services (depended upon by others) ──

	notifyService := notify.NewService(cfg.Notify.WebhookURL, cfg.Notify.Enabled)

	notificationRepo := notification.NewSQLiteRepository(db)
	notificationService := notification.NewService(notificationRepo)

	alertService := alert.NewService(notifyService, notificationService)
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

	auditRepo := audit.NewSQLiteRepository(db)
	auditSvc := audit.NewService(ctx, &wg, auditRepo, cfg.Audit.RetentionDays)

	// ── Auth ──

	userRepo := auth.NewSQLiteUserRepository(db)
	tokenRepo := auth.NewSQLiteTokenRepository(db)
	totpRepo := auth.NewTOTPRepository(db)

	authSvc := auth.NewAuthService(ctx, &wg, cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutDuration, userRepo, tokenRepo, auditSvc, totpRepo, notifyService)
	if err := authSvc.InitDefaultAdmin(ctx); err != nil {
		log.Fatalf("init default admin: %v", err)
	}

	sessionSvc := auth.NewSessionService(ctx, &wg, cfg.Auth.IdleTimeout, cfg.Auth.SessionCleanupInterval)

	qrLoginService := qrlogin.NewService(qrlogin.NewSQLiteRepository(db), cfg.Auth.JWTSecret, cfg.Auth.SessionTimeout, sessionSvc)

	// ── Monitor ──

	monitorRepo := monitor.NewSQLiteRepository(db)
	monitorSvc := monitor.NewMonitorService(ctx, &wg, monitorRepo, cfg.Monitor.CollectInterval, cfg.Monitor.HistoryRetention, alertService, auditSvc)

	// ── Terminal ──

	terminalManager := terminal.NewManager(ctx, &wg, cmdExec, cfg.Auth.IdleTimeout)

	// ── Domain services (no background goroutines) ──

	cronRepo := cron.NewSQLiteRepository(db)
	cronService := cron.NewService(cronRepo, cmdExec, miseProvider)

	serviceManager := systemd.NewServiceManager(cmdExec, cronRepo, miseProvider)

	containerService := container.NewService(cmdExec)

	dbServerRepo := dbserver.NewSQLiteRepository(db)
	dbServerService := dbserver.NewService(cmdExec, dbServerRepo)
	dbServerService.SeedPredefinedServers(ctx)

	databaseMgmtRepo := database_mgmt.NewSQLiteRepository(db)
	databaseMgmtService := database_mgmt.NewService(databaseMgmtRepo, cmdExec)

	deployRepo := deploy.NewSQLiteRepository(db)
	deploySvc, err := deploy.NewService(deployRepo, cfg.Deploy.EncryptionKey)
	if err != nil {
		log.Fatalf("init deploy service: %v", err)
	}

	envConfigRepo := envconfig.NewSQLiteRepository(db)
	envConfigService := envconfig.NewService(envConfigRepo)

	firewallRepo := firewall.NewSQLiteRepository(db)
	firewallService := firewall.NewService(firewallRepo, cmdExec, cfg.Server.Port)

	runtimeRepo := runtimeenv.NewSQLiteRepository(db)
	runtimeService := runtimeenv.NewService(runtimeRepo, cmdExec, envConfigService, miseProvider)
	if err := runtimeService.Init(ctx); err != nil {
		log.Printf("ERROR: Failed to init runtime service: %v", err)
	}
	packageManagerService := runtimeenv.NewPackageService(cmdExec, miseProvider)

	sshConfigService := ssh.NewService(cmdExec)

	webServerRepo := web.NewSQLiteServerRepository(db)
	websiteRepo := web.NewSQLiteWebsiteRepository(db)
	webServerSvc := web.NewService(webServerRepo, cmdExec)
	webServerSvc.SeedPredefinedWebServers(ctx)

	securityRepo := websecurity.NewSQLiteSecurityRepository(db)
	securitySvc := websecurity.NewSecurityService(securityRepo, firewallService, cmdExec)
	websiteSvc := web.NewWebsiteService(websiteRepo, webServerRepo, cmdExec, securityRepo)

	fileManager, err := filemanager.NewManager(cfg.FileManager.BasePath)
	if err != nil {
		log.Fatalf("init file manager: %v", err)
	}

	fileShareRepo := filemanager.NewSQLiteShareRepository(db)
	if deleted, err := fileShareRepo.DeleteExpired(ctx); err != nil {
		log.Printf("ERROR: failed to cleanup expired file shares on startup: %v", err)
	} else if deleted > 0 {
		log.Printf("Cleaned up %d expired file shares on startup", deleted)
	}

	var cloudService *cloud.Service
	if cfg.TencentCloud.Enabled {
		cloudService, err = cloud.NewService(
			cfg.TencentCloud.SecretID,
			cfg.TencentCloud.SecretKey,
			cfg.TencentCloud.Region,
			cfg.TencentCloud.InstanceID,
		)
		if err != nil {
			log.Printf("WARNING: failed to init cloud service: %v", err)
		}
	}

	// ── HTTP Engine + Routes ──

	e := newEngine(cfg)
	g := newGroups(e, cfg, authSvc, sessionSvc, auditSvc)

	// Domain route registration
	authhttp.RegisterRoutes(g.API, authSvc, auditSvc, sessionSvc, qrLoginService, cfg.Auth.JWTSecret, g.sessionValidator, g.tokenValidator, cfg.Auth.SessionTimeout, cfg.Auth.LoginRateLimit, cfg.Auth.LoginRateInterval, cfg)
	monitorhttp.RegisterRoutes(g.Protected, g.WS, monitorSvc, cmdExec, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	systemdhttp.RegisterRoutes(g.Protected, g.WS, serviceManager, cmdExec, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	terminalhttp.RegisterRoutes(g.Protected, g.WS, terminalManager, cfg.Auth.JWTSecret, auditSvc, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	filemanagerhttp.RegisterRoutes(g.Protected, g.File, fileManager, g.maxUploadSize)
	audithttp.RegisterRoutes(g.Protected, db, auditRepo)
	settingshttp.RegisterRoutes(g.Protected, cfg, configPath, alertService, monitorSvc, cmdExec, sig)
	cloudhttp.RegisterRoutes(g.Protected, cloudService, &cfg.TencentCloud, cfg.Server.Port)
	deployhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), deploySvc)
	runtimeenvhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), runtimeService, packageManagerService)
	envconfighttp.RegisterRoutes(g.Protected, envConfigService)
	webhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), webServerSvc, websiteSvc)
	dbserverhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), dbServerService, databaseMgmtService)
	cronhttp.RegisterRoutes(g.Protected, cronService, cmdExec)
	firewallhttp.RegisterRoutes(g.Protected, firewallService, cfg.Server.Port)
	sshhttp.RegisterRoutes(g.Protected, sshConfigService)
	containerhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), containerService)
	notificationhttp.RegisterRoutes(g.Protected, notificationService)
	securityhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), security.NewService(cmdExec, db))
	if securitySvc != nil {
		secHandler := websecurityhttp.NewSecurityHandler(securitySvc)
		secHandler.RegisterRoutes(g.Protected.Group("/websites"))
	}
	filemanagerhttp.RegisterShareRoutes(g.Protected, fileShareRepo, fileManager, cfg)
	filemanagerhttp.RegisterPublicShareRoute(g.API, fileShareRepo, fileManager, cfg.Auth.RateLimit, cfg.Auth.RateInterval, cfg)

	// Static frontend
	if cfg.Server.ServeFrontend {
		e.Use(middleware.RateLimitMiddleware("assets", cfg.Server.AssetsRateLimit, cfg.Server.AssetsRateInterval))
		ServeWeb(e)
	}

	// ── Startup audit event ──
	auditSvc.LogSystemEvent(ctx, "面板启动")

	// ── Shutdown function ──
	shutdown := func() {
		auditSvc.LogSystemEvent(context.Background(), "面板停止")
		auditSvc.Close()
		cancel()
		wg.Wait()
		middleware.StopRateLimiter()
		middleware.StopSessionHeartbeatLimiter()
		db.Close()
	}

	return e, shutdown
}

// routeGroups holds the pre-configured gin router groups.
type routeGroups struct {
	API              *gin.RouterGroup
	Protected        *gin.RouterGroup
	WS               *gin.RouterGroup
	File             *gin.RouterGroup
	maxUploadSize    int64
	tokenValidator   func(userID int64, tokenString string, issuedAt time.Time) (bool, error)
	sessionValidator func(token string) (bool, error)
}

// newEngine creates the gin engine with global middleware.
func newEngine(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.MaxMultipartMemory = 64 << 20

	trustedProxies := cfg.Server.TrustedProxies
	if trustedProxies == nil {
		trustedProxies = []string{"127.0.0.1"}
	}
	if err := e.SetTrustedProxies(trustedProxies); err != nil {
		log.Printf("router: invalid trusted_proxies %v, falling back to RemoteAddr only: %v", trustedProxies, err)
		_ = e.SetTrustedProxies(nil)
	}

	cspNonce := InitCSPNonce()

	e.Use(gin.Logger(), gin.Recovery(),
		middleware.ErrorHandler(),
		middleware.DomainRedirectMiddleware(cfg.Server.Domain, cfg.Server.RedirectMode, cfg.Server.WwwHandling),
		middleware.SecurityMiddleware(cspNonce),
		middleware.CORSMiddleware(cfg.Server.AllowedOrigins, cfg.Server.DevMode),
		middleware.IPWhitelistMiddleware(middleware.NewIPWhitelist(cfg.Auth.IPWhitelist)),
	)

	e.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return e
}

// newGroups creates the authenticated route groups.
func newGroups(e *gin.Engine, cfg *config.Config, authSvc *auth.AuthService, sessionSvc *auth.SessionService, auditSvc *audit.Service) *routeGroups {
	maxUploadSize := cfg.Server.MaxUploadSize
	if maxUploadSize <= 0 {
		maxUploadSize = 512 << 20
	}

	tokenValidator := func(userID int64, tokenString string, issuedAt time.Time) (bool, error) {
		return authSvc.IsUserTokenInvalidated(context.Background(), userID, issuedAt)
	}
	sessionValidator := func(token string) (bool, error) {
		return sessionSvc.IsSessionValid(context.Background(), token)
	}

	api := e.Group("/api")
	api.Use(
		middleware.MaxBodySizeMiddleware(maxUploadSize),
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval),
	)

	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return authSvc.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(sessionSvc.UpdateActivity, cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(auditSvc),
		middleware.CSRFMiddleware(),
	)

	// File upload sub-group
	fileRoutes := api.Group("/files",
		middleware.ReadTimeout(10*time.Minute),
		middleware.WriteTimeout(10*time.Minute),
	)
	fileRoutes.Use(
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval),
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return authSvc.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(sessionSvc.UpdateActivity, cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(auditSvc),
		middleware.CSRFMiddleware(),
	)

	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator))

	return &routeGroups{
		API:              api,
		Protected:        protected,
		WS:               wsGroup,
		File:             fileRoutes,
		maxUploadSize:    maxUploadSize,
		tokenValidator:   tokenValidator,
		sessionValidator: sessionValidator,
	}
}
