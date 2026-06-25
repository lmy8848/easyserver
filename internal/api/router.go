package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"easyserver/internal/config"
	"easyserver/internal/executor"
	"easyserver/internal/middleware"
	"easyserver/internal/repository"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg                  *config.Config
	configPath           string
	db                   *sql.DB
	executor             executor.CommandExecutor
	auditRepo            repository.AuditRepository
	authService          *service.AuthService
	monitorService       *service.MonitorService
	auditService         *service.AuditService
	sessionService       *service.SessionService
	totpService          *service.TOTPService
	alertService         *service.AlertService
	processManager       *service.ProcessManager
	systemProcessService *service.SystemProcessService
	notificationService  *service.NotificationService
	serviceManager       *service.ServiceManager

	// Container services
	containerService *service.ContainerService
	dockerService    *service.DockerService
	composeService   *service.ComposeService
	volumeService    *service.VolumeService
	networkService   *service.NetworkService

	// Cron service
	cronService *service.CronService

	// Database services
	dbServerService     *service.DBServerService
	databaseMgmtService *service.DatabaseMgmtService
	dbBackupService     *service.DBBackupService
	sqlQueryService     *service.SQLQueryService

	// Deploy service
	deployService *service.DeployService

	// Environment config service
	envConfigService *service.EnvConfigService

	// Firewall service
	firewallService *service.FirewallService

	// Runtime services
	runtimeService        *service.RuntimeService
	runtimeVersionService *service.RuntimeVersionService
	packageManagerService *service.PackageManagerService

	// SSH service
	sshConfigService *service.SSHConfigService

	// Web server services
	webServerService *service.WebServerService
	websiteService   *service.WebsiteService

	// Terminal manager
	terminalManager *service.TerminalManager

	// File manager
	fileManager *service.FileManager

	// Cloud service (nil if disabled)
	cloudService *service.CloudService
}

// RouterDeps holds the shared service instances created once in main.go.
// Passing them in (instead of re-creating inside NewRouter) guarantees a
// single instance per service so that in-memory caches, background goroutines,
// and lifecycle hooks (Close, flush) all operate on the same state.
type RouterDeps struct {
	DB                   *sql.DB
	Executor             executor.CommandExecutor
	AuthService          *service.AuthService
	MonitorService       *service.MonitorService
	AuditService         *service.AuditService
	SessionService       *service.SessionService
	TotpService          *service.TOTPService
	AuditRepo            repository.AuditRepository
	ProcessManager       *service.ProcessManager
	SystemProcessService *service.SystemProcessService
	NotificationService  *service.NotificationService
	ServiceManager       *service.ServiceManager

	// Container services
	ContainerService *service.ContainerService
	DockerService    *service.DockerService
	ComposeService   *service.ComposeService
	VolumeService    *service.VolumeService
	NetworkService   *service.NetworkService

	// Cron service
	CronService *service.CronService

	// Database services
	DBServerService     *service.DBServerService
	DatabaseMgmtService *service.DatabaseMgmtService
	DBBackupService     *service.DBBackupService
	SQLQueryService     *service.SQLQueryService

	// Deploy service
	DeployService *service.DeployService

	// Environment config service
	EnvConfigService *service.EnvConfigService

	// Firewall service
	FirewallService *service.FirewallService

	// Runtime services
	RuntimeService        *service.RuntimeService
	RuntimeVersionService *service.RuntimeVersionService
	PackageManagerService *service.PackageManagerService

	// SSH service
	SSHConfigService *service.SSHConfigService

	// Web server services
	WebServerService *service.WebServerService
	WebsiteService   *service.WebsiteService

	// Notify + Alert (wired in main.go)
	NotifyService *service.NotifyService
	AlertService  *service.AlertService

	// Terminal manager
	TerminalManager *service.TerminalManager

	// File manager
	FileManager *service.FileManager

	// Cloud service (nil if disabled)
	CloudService *service.CloudService
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
		totpService:          deps.TotpService,
		alertService:         deps.AlertService,
		processManager:       deps.ProcessManager,
		systemProcessService: deps.SystemProcessService,
		notificationService:  deps.NotificationService,
		serviceManager:       deps.ServiceManager,

		// Container services
		containerService: deps.ContainerService,
		dockerService:    deps.DockerService,
		composeService:   deps.ComposeService,
		volumeService:    deps.VolumeService,
		networkService:   deps.NetworkService,

		// Cron service
		cronService: deps.CronService,

		// Database services
		dbServerService:     deps.DBServerService,
		databaseMgmtService: deps.DatabaseMgmtService,
		dbBackupService:     deps.DBBackupService,
		sqlQueryService:     deps.SQLQueryService,

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
	registerAuthRoutes(api, r.authService, r.auditService, r.sessionService, r.totpService, r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator, r.cfg.Auth.SessionTimeout)

	// Protected routes (JWT + SingleAdmin + Audit + Session Heartbeat)
	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(r.cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.RequireSingleAdmin(),
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
	registerDatabaseRoutes(protected, r.dbServerService, r.databaseMgmtService, r.dbBackupService, r.sqlQueryService)
	registerCronRoutes(protected, r.cronService, r.executor)
	registerFirewallRoutes(protected, r.firewallService, r.cfg.Server.Port)
	registerSSHRoutes(protected, r.sshConfigService)
	registerContainerRoutes(protected, r.containerService, r.dockerService, r.composeService, r.volumeService, r.networkService, r.auditService)
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
