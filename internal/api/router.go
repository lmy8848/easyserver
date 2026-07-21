package api

import (
	"context"
	"database/sql"
	"log"
	"net/http"
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
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
	"easyserver/internal/deploy"
	deployhttp "easyserver/internal/deploy/http"
	"easyserver/internal/envconfig"
	envconfighttp "easyserver/internal/envconfig/http"
	"easyserver/internal/filemanager"
	filemanagerhttp "easyserver/internal/filemanager/http"
	"easyserver/internal/fileshare"
	filesharehttp "easyserver/internal/fileshare/http"
	"easyserver/internal/firewall"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/executor"
	"easyserver/internal/infra/launcher"
	"easyserver/internal/monitor"
	monitorhttp "easyserver/internal/monitor/http"
	"easyserver/internal/notification"
	notificationhttp "easyserver/internal/notification/http"
	"easyserver/internal/notify"
	"easyserver/internal/process"
	processhttp "easyserver/internal/process/http"
	"easyserver/internal/qrlogin"
	"easyserver/internal/runtimeenv"
	"easyserver/internal/ssh"
	sshhttp "easyserver/internal/ssh/http"
	"easyserver/internal/systemd"
	systemdhttp "easyserver/internal/systemd/http"
	"easyserver/internal/systemprocess"
	"easyserver/internal/terminal"
	"easyserver/internal/web"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg                  *config.Config
	configPath           string
	db                   *sql.DB
	executor             executor.CommandExecutor
	auditRepo            audit.Repository
	authService          *auth.AuthService
	monitorService       *monitor.MonitorService
	auditService         *audit.Service
	sessionService       *auth.SessionService
	qrLoginService       *qrlogin.Service
	alertService         *alert.Service
	processManager       *process.Service
	systemProcessService *systemprocess.Service
	notificationService  *notification.Service
	serviceManager       *systemd.ServiceManager

	// Container service
	containerService *container.Service

	// Cron service
	cronService *cron.Service

	// Database services
	dbServerService     *dbserver.Service
	databaseMgmtService *database_mgmt.Service

	// Deploy service
	deployService *deploy.Service

	// Environment config service
	envConfigService *envconfig.Service

	// Firewall service
	firewallService *firewall.Service

	// Runtime services
	runtimeService        *runtimeenv.Service
	packageManagerService *runtimeenv.PackageService

	// SSH service
	sshConfigService *ssh.Service

	// Web server services
	webServerService *web.Service
	websiteService   *web.WebsiteService

	// Terminal manager
	terminalManager *terminal.Manager

	// File manager
	fileManager *filemanager.Manager

	// File share repo
	fileShareRepo fileshare.Repository

	// Cloud service (nil if disabled)
	cloudService *cloud.Service

	// Launcher for hot restart (FD passing)
	launcher *launcher.Launcher
}

// RouterDeps holds the shared service instances created once in main.go.
// Passing them in (instead of re-creating inside NewRouter) guarantees a
// single instance per service so that in-memory caches, background goroutines,
// and lifecycle hooks (Close, flush) all operate on the same state.
type RouterDeps struct {
	DB                   *sql.DB
	Executor             executor.CommandExecutor
	AuthService          *auth.AuthService
	MonitorService       *monitor.MonitorService
	AuditService         *audit.Service
	SessionService       *auth.SessionService
	AuditRepo            audit.Repository
	ProcessManager       *process.Service
	SystemProcessService *systemprocess.Service
	NotificationService  *notification.Service
	ServiceManager       *systemd.ServiceManager

	// Container service
	ContainerService *container.Service

	// Cron service
	CronService *cron.Service

	// Database services
	DBServerService     *dbserver.Service
	DatabaseMgmtService *database_mgmt.Service

	// Deploy service
	DeployService *deploy.Service

	// Environment config service
	EnvConfigService *envconfig.Service

	// Firewall service
	FirewallService *firewall.Service

	// Runtime services
	RuntimeService        *runtimeenv.Service
	PackageManagerService *runtimeenv.PackageService

	// SSH service
	SSHConfigService *ssh.Service

	// Web server services
	WebServerService *web.Service
	WebsiteService   *web.WebsiteService

	// Notify + Alert (wired in main.go)
	NotifyService *notify.Service
	AlertService  *alert.Service

	// Terminal manager
	TerminalManager *terminal.Manager

	// File manager
	FileManager *filemanager.Manager

	// File share repository
	FileShareRepo fileshare.Repository

	// QR login service (scan-to-login)
	QRLoginService *qrlogin.Service

	// Cloud service (nil if disabled)
	CloudService *cloud.Service

	// Launcher for hot restart (FD passing)
	Launcher *launcher.Launcher
}

func NewRouter(cfg *config.Config, configPath string, deps RouterDeps) *Router {
	return &Router{
		cfg:                  cfg,
		configPath:           configPath,
		db:                   deps.DB,
		executor:             deps.Executor,
		auditRepo:            deps.AuditRepo,
		authService:          deps.AuthService,
		monitorService:       deps.MonitorService,
		auditService:         deps.AuditService,
		sessionService:       deps.SessionService,
		alertService:         deps.AlertService,
		processManager:       deps.ProcessManager,
		systemProcessService: deps.SystemProcessService,
		notificationService:  deps.NotificationService,
		serviceManager:       deps.ServiceManager,

		// Container service
		containerService: deps.ContainerService,

		// Cron service
		cronService: deps.CronService,

		// Database services
		dbServerService:     deps.DBServerService,
		databaseMgmtService: deps.DatabaseMgmtService,

		// Deploy service
		deployService: deps.DeployService,

		// Environment config service
		envConfigService: deps.EnvConfigService,

		// Firewall service
		firewallService: deps.FirewallService,

		// Runtime services
		runtimeService:        deps.RuntimeService,
		packageManagerService: deps.PackageManagerService,

		// SSH service
		sshConfigService: deps.SSHConfigService,

		// Web server services
		webServerService: deps.WebServerService,
		websiteService:   deps.WebsiteService,

		// Terminal manager
		terminalManager: deps.TerminalManager,

		// File manager
		fileManager: deps.FileManager,

		// File share repo
		fileShareRepo:  deps.FileShareRepo,
		qrLoginService: deps.QRLoginService,

		// Cloud service
		cloudService: deps.CloudService,

		// Launcher
		launcher: deps.Launcher,
	}
}

func (r *Router) Setup() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.MaxMultipartMemory = 64 << 20 // 64 MB multipart memory (rest goes to temp disk)

	// Trust only configured reverse proxies for X-Forwarded-For. Gin's default
	// trusts 0.0.0.0/0, which lets any client spoof ClientIP and bypass IP
	// whitelist, rate limiting and audit logging. Default to localhost (same-
	// host nginx). Front by a CDN by setting server.trusted_proxies to its ranges.
	trustedProxies := r.cfg.Server.TrustedProxies
	if trustedProxies == nil {
		trustedProxies = []string{"127.0.0.1"}
	}
	if err := e.SetTrustedProxies(trustedProxies); err != nil {
		log.Printf("router: invalid trusted_proxies %v, falling back to RemoteAddr only: %v", trustedProxies, err)
		_ = e.SetTrustedProxies(nil)
	}

	// Create IP whitelist
	ipWhitelist := middleware.NewIPWhitelist(&r.cfg.Auth)

	// Validators
	tokenValidator := func(userID int64, tokenString string, issuedAt time.Time) (bool, error) {
		return r.authService.IsUserTokenInvalidated(context.Background(), userID, issuedAt)
	}
	sessionValidator := func(token string) (bool, error) {
		return r.sessionService.IsSessionValid(context.Background(), token)
	}

	// Initialize CSP nonce: injects nonce into <script> tags of embedded index.html
	cspNonce := InitCSPNonce()

	// Global middleware (no rate limiter — tiered limiters are applied per group below)
	e.Use(gin.Logger(), gin.Recovery(),
		ErrorHandler(),
		DomainRedirectMiddleware(r.cfg.Server.Domain, r.cfg.Server.RedirectMode, r.cfg.Server.WwwHandling),
		middleware.SecurityMiddleware(cspNonce),
		middleware.CORSMiddleware(r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode),
		middleware.IPWhitelistMiddleware(ipWhitelist),
	)

	// Health check (no rate limit)
	e.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes — Tier 2: general API limiter
	// MaxUploadSize from config (default 512MB); 0 = use default
	maxUploadSize := r.cfg.Server.MaxUploadSize
	if maxUploadSize <= 0 {
		maxUploadSize = 512 << 20 // 512 MB default
	}
	api := e.Group("/api")
	api.Use(
		middleware.MaxBodySizeMiddleware(maxUploadSize),
		middleware.RateLimitMiddleware("api", r.cfg.Auth.RateLimit, r.cfg.Auth.RateInterval),
	)

	// Auth routes (public + protected)
	authhttp.RegisterRoutes(api, r.authService, r.auditService, r.sessionService, r.qrLoginService, r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator, r.cfg.Auth.SessionTimeout, r.cfg.Auth.LoginRateLimit, r.cfg.Auth.LoginRateInterval, r.cfg)

	// Protected routes (JWT + SingleAdmin + Audit + Session Heartbeat)
	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return r.authService.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(r.sessionService, r.cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(r.auditService),
		middleware.CSRFMiddleware(),
	)

	// File upload sub-group: extended timeouts for large uploads, no MaxBodySize (use Gin's multipart memory limit instead)
	fileRoutes := api.Group("/files",
		middleware.ReadTimeout(10*time.Minute),
		middleware.WriteTimeout(10*time.Minute),
	)
	fileRoutes.Use(
		middleware.RateLimitMiddleware("api", r.cfg.Auth.RateLimit, r.cfg.Auth.RateInterval),
		middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return r.authService.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(r.sessionService, r.cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(r.auditService),
		middleware.CSRFMiddleware(),
	)

	// WebSocket routes
	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator))

	// Register domain routes
	monitorhttp.RegisterRoutes(protected, wsGroup, r.monitorService, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	systemdhttp.RegisterRoutes(protected, wsGroup, r.serviceManager, r.executor, r.cfg.Auth.JWTSecret, r.auditService, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerTerminalRoutes(protected, wsGroup, r.terminalManager, r.cfg.Auth.JWTSecret, r.auditService, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	filemanagerhttp.RegisterRoutes(protected, fileRoutes, r.fileManager, maxUploadSize)
	audithttp.RegisterRoutes(protected, r.db, r.auditService, r.auditRepo)
	registerSettingsRoutes(protected, r.cfg, r.configPath, r.alertService, r.executor, r.launcher)
	registerSystemRoutes(protected, r.executor)
	protected.GET("/system/ports", (&monitorhttp.PortMonitorHandler{}).GetListeningPorts)
	cloudhttp.RegisterRoutes(protected, r.cloudService, &r.cfg.TencentCloud, r.cfg.Server.Port)
	deployhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), r.deployService)
	registerRuntimeRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), r.runtimeService, r.packageManagerService)
	envconfighttp.RegisterRoutes(protected, r.envConfigService)
	registerWebServerRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), r.webServerService, r.websiteService, r.processManager)
	registerDatabaseRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), r.dbServerService, r.databaseMgmtService)
	registerCronRoutes(protected, r.cronService, r.executor)
	registerFirewallRoutes(protected, r.firewallService, r.cfg.Server.Port)
	sshhttp.RegisterRoutes(protected, r.sshConfigService)
	containerhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), r.containerService, r.auditService)
	registerTemplateRoutes(protected)
	processhttp.RegisterRoutes(protected, r.processManager)
	registerSystemProcessRoutes(protected, r.systemProcessService)
	notificationhttp.RegisterRoutes(protected, r.notificationService)
	filesharehttp.RegisterRoutes(protected, r.fileShareRepo, r.fileManager, r.cfg)

	// Public file share routes (no auth): /share/:token/info + /share/:token/download.
	// /share/:token itself is NOT registered so it falls through to the SPA fallback.
	filesharehttp.RegisterPublicShareRoute(e, r.fileShareRepo, r.fileManager, r.cfg.Auth.RateLimit, r.cfg.Auth.RateInterval, r.cfg)

	// Tier 1: static assets limiter (applied to all frontend routes including SPA fallback)
	if r.cfg.Server.ServeFrontend {
		e.Use(middleware.RateLimitMiddleware("assets", r.cfg.Server.AssetsRateLimit, r.cfg.Server.AssetsRateInterval))
		ServeWeb(e)
	}

	return e
}
