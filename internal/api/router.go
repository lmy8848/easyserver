package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"easyserver/internal/alert"
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
	"easyserver/internal/firewall"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/executor"
	"easyserver/internal/monitor"
	"easyserver/internal/notification"
	"easyserver/internal/notify"
	"easyserver/internal/packagemanager"
	"easyserver/internal/process"
	"easyserver/internal/runtimeenv"
	"easyserver/internal/ssh"
	"easyserver/internal/systemd"
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
	runtimeVersionService *runtimeenv.VersionService
	packageManagerService *packagemanager.Service

	// SSH service
	sshConfigService *ssh.Service

	// Web server services
	webServerService *web.Service
	websiteService   *web.WebsiteService

	// Terminal manager
	terminalManager *terminal.Manager

	// File manager
	fileManager *filemanager.Manager

	// Cloud service (nil if disabled)
	cloudService *cloud.Service
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
	RuntimeVersionService *runtimeenv.VersionService
	PackageManagerService *packagemanager.Service

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

	// Cloud service (nil if disabled)
	CloudService *cloud.Service
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
		runtimeVersionService: deps.RuntimeVersionService,
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

		// Cloud service
		cloudService: deps.CloudService,
	}
}

func (r *Router) Setup() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.MaxMultipartMemory = 32 << 20

	// Create IP whitelist
	ipWhitelist := middleware.NewIPWhitelist(&r.cfg.Auth)

	// Validators
	tokenValidator := func(userID int64, tokenString string, issuedAt time.Time) (bool, error) {
		return r.authService.IsUserTokenInvalidated(context.Background(), userID, issuedAt)
	}
	sessionValidator := func(token string) (bool, error) {
		return r.sessionService.IsSessionValid(context.Background(), token)
	}

	// Global middleware
	e.Use(gin.Logger(), gin.Recovery(),
		ErrorHandler(),
		middleware.SecurityMiddleware(),
		middleware.CORSMiddleware(r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode),
		middleware.RateLimitMiddleware(r.cfg.Auth.RateLimit, r.cfg.Auth.RateInterval),
		middleware.IPWhitelistMiddleware(ipWhitelist),
	)

	// Health check
	e.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes
	api := e.Group("/api")

	// Auth routes (public + protected)
	registerAuthRoutes(api, r.authService, r.auditService, r.sessionService, r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator, r.cfg.Auth.SessionTimeout)

	// Protected routes (JWT + SingleAdmin + Audit + Session Heartbeat)
	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return r.authService.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(r.sessionService, r.cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(r.auditService),
	)

	// WebSocket routes
	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator))

	// Register domain routes
	registerMonitorRoutes(protected, wsGroup, r.monitorService, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerServiceRoutes(protected, wsGroup, r.serviceManager, r.executor, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerTerminalRoutes(protected, wsGroup, r.terminalManager, r.cfg.Auth.JWTSecret, r.auditService, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerFileRoutes(protected, r.fileManager, r.auditService)
	registerAuditRoutes(protected, r.db, r.auditService, r.auditRepo)
	registerSettingsRoutes(protected, r.cfg, r.configPath, r.alertService, r.executor)
	registerSystemRoutes(protected, r.executor)
	registerCloudRoutes(protected, r.cloudService, &r.cfg.TencentCloud, r.cfg.Server.Port)
	registerDeployRoutes(protected, r.deployService)
	registerRuntimeRoutes(protected, r.runtimeService, r.runtimeVersionService, r.packageManagerService)
	registerEnvRoutes(protected, r.envConfigService)
	registerWebServerRoutes(protected, r.webServerService, r.websiteService)
	registerDatabaseRoutes(protected, r.dbServerService, r.databaseMgmtService)
	registerCronRoutes(protected, r.cronService, r.executor)
	registerFirewallRoutes(protected, r.firewallService, r.cfg.Server.Port)
	registerSSHRoutes(protected, r.sshConfigService)
	registerContainerRoutes(protected, r.containerService, r.auditService)
	registerTemplateRoutes(protected)
	registerProcessRoutes(protected, r.processManager)
	registerSystemProcessRoutes(protected, r.systemProcessService)
	registerNotificationRoutes(protected, r.notificationService)

	// Serve embedded frontend (production mode)
	if r.cfg.Server.ServeFrontend {
		ServeWeb(e)
	}

	return e
}
