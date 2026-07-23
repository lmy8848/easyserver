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
	"easyserver/internal/fileshare"
	filesharehttp "easyserver/internal/fileshare/http"
	"easyserver/internal/firewall"
	firewallhttp "easyserver/internal/firewall/http"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/executor"
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

	"github.com/gin-gonic/gin"
)

// RouterDeps holds the shared service instances created once in main.go.
// Passing them in (instead of re-creating inside NewRouter) guarantees a
// single instance per service so that in-memory caches, background goroutines,
// and lifecycle hooks (Close, flush) all operate on the same state.
type RouterDeps struct {
	DB                  *sql.DB
	Executor            executor.CommandExecutor
	AuthService         *auth.AuthService
	MonitorService      *monitor.MonitorService
	AuditService        *audit.Service
	SessionService      *auth.SessionService
	AuditRepo           audit.Repository
	NotificationService *notification.Service
	ServiceManager      *systemd.ServiceManager

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

	// Signal for hot restart (FD passing). Handlers call Request; the App
	// event loop acts on it.
	Signal *infra.Signal
}

func Setup(cfg *config.Config, configPath string, deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.MaxMultipartMemory = 64 << 20 // 64 MB multipart memory (rest goes to temp disk)

	// Trust only configured reverse proxies for X-Forwarded-For. Gin's default
	// trusts 0.0.0.0/0, which lets any client spoof ClientIP and bypass IP
	// whitelist, rate limiting and audit logging. Default to localhost (same-
	// host nginx). Front by a CDN by setting server.trusted_proxies to its ranges.
	trustedProxies := cfg.Server.TrustedProxies
	if trustedProxies == nil {
		trustedProxies = []string{"127.0.0.1"}
	}
	if err := e.SetTrustedProxies(trustedProxies); err != nil {
		log.Printf("router: invalid trusted_proxies %v, falling back to RemoteAddr only: %v", trustedProxies, err)
		_ = e.SetTrustedProxies(nil)
	}

	// Create IP whitelist
	ipWhitelist := middleware.NewIPWhitelist(&cfg.Auth)

	// Validators
	tokenValidator := func(userID int64, tokenString string, issuedAt time.Time) (bool, error) {
		return deps.AuthService.IsUserTokenInvalidated(context.Background(), userID, issuedAt)
	}
	sessionValidator := func(token string) (bool, error) {
		return deps.SessionService.IsSessionValid(context.Background(), token)
	}

	// Initialize CSP nonce: injects nonce into <script> tags of embedded index.html
	cspNonce := InitCSPNonce()

	// Global middleware (no rate limiter — tiered limiters are applied per group below)
	e.Use(gin.Logger(), gin.Recovery(),
		httpx.ErrorHandler(),
		middleware.DomainRedirectMiddleware(cfg.Server.Domain, cfg.Server.RedirectMode, cfg.Server.WwwHandling),
		middleware.SecurityMiddleware(cspNonce),
		middleware.CORSMiddleware(cfg.Server.AllowedOrigins, cfg.Server.DevMode),
		middleware.IPWhitelistMiddleware(ipWhitelist),
	)

	// Health check (no rate limit)
	e.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API routes — Tier 2: general API limiter
	// MaxUploadSize from config (default 512MB); 0 = use default
	maxUploadSize := cfg.Server.MaxUploadSize
	if maxUploadSize <= 0 {
		maxUploadSize = 512 << 20 // 512 MB default
	}
	api := e.Group("/api")
	api.Use(
		middleware.MaxBodySizeMiddleware(maxUploadSize),
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval),
	)

	// Auth routes (public + protected)
	authhttp.RegisterRoutes(api, deps.AuthService, deps.AuditService, deps.SessionService, deps.QRLoginService, cfg.Auth.JWTSecret, sessionValidator, tokenValidator, cfg.Auth.SessionTimeout, cfg.Auth.LoginRateLimit, cfg.Auth.LoginRateInterval, cfg)

	// Protected routes (JWT + SingleAdmin + Audit + Session Heartbeat)
	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return deps.AuthService.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(deps.SessionService, cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(deps.AuditService),
		middleware.CSRFMiddleware(),
	)

	// File upload sub-group: extended timeouts for large uploads, no MaxBodySize (use Gin's multipart memory limit instead)
	fileRoutes := api.Group("/files",
		middleware.ReadTimeout(10*time.Minute),
		middleware.WriteTimeout(10*time.Minute),
	)
	fileRoutes.Use(
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval),
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator),
		middleware.UserIPWhitelistMiddleware(func(userID int64) (string, error) {
			return deps.AuthService.GetIPWhitelist(context.Background(), userID)
		}),
		middleware.SessionHeartbeatMiddleware(deps.SessionService, cfg.Auth.SessionTimeout),
		middleware.AuditMiddleware(deps.AuditService),
		middleware.CSRFMiddleware(),
	)

	// WebSocket routes
	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(cfg.Auth.JWTSecret, sessionValidator, tokenValidator))

	// Register domain routes
	monitorhttp.RegisterRoutes(protected, wsGroup, deps.MonitorService, deps.Executor, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	systemdhttp.RegisterRoutes(protected, wsGroup, deps.ServiceManager, deps.Executor, cfg.Auth.JWTSecret, deps.AuditService, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	terminalhttp.RegisterRoutes(protected, wsGroup, deps.TerminalManager, cfg.Auth.JWTSecret, deps.AuditService, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	filemanagerhttp.RegisterRoutes(protected, fileRoutes, deps.FileManager, maxUploadSize)
	audithttp.RegisterRoutes(protected, deps.DB, deps.AuditService, deps.AuditRepo)
	settingshttp.RegisterRoutes(protected, cfg, configPath, deps.AlertService, deps.MonitorService, deps.Executor, deps.Signal)
	cloudhttp.RegisterRoutes(protected, deps.CloudService, &cfg.TencentCloud, cfg.Server.Port)
	deployhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), deps.DeployService)
	runtimeenvhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), deps.RuntimeService, deps.PackageManagerService)
	envconfighttp.RegisterRoutes(protected, deps.EnvConfigService)
	webhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), deps.WebServerService, deps.WebsiteService)
	dbserverhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), deps.DBServerService, deps.DatabaseMgmtService)
	cronhttp.RegisterRoutes(protected, deps.CronService, deps.Executor)
	firewallhttp.RegisterRoutes(protected, deps.FirewallService, cfg.Server.Port)
	sshhttp.RegisterRoutes(protected, deps.SSHConfigService)
	containerhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), deps.ContainerService, deps.AuditService)
	notificationhttp.RegisterRoutes(protected, deps.NotificationService)
	securityhttp.RegisterRoutes(protected.Group("", middleware.WriteTimeout(10*time.Minute)), security.NewService(deps.Executor))
	filesharehttp.RegisterRoutes(protected, deps.FileShareRepo, deps.FileManager, cfg)

	// Public file share routes (no auth): /share/:token/info + /share/:token/download.
	// /share/:token itself is NOT registered so it falls through to the SPA fallback.
	filesharehttp.RegisterPublicShareRoute(e, deps.FileShareRepo, deps.FileManager, cfg.Auth.RateLimit, cfg.Auth.RateInterval, cfg)

	// Tier 1: static assets limiter (applied to all frontend routes including SPA fallback)
	if cfg.Server.ServeFrontend {
		e.Use(middleware.RateLimitMiddleware("assets", cfg.Server.AssetsRateLimit, cfg.Server.AssetsRateInterval))
		ServeWeb(e)
	}

	return e
}
