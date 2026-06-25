package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"easyserver/internal/config"
	"easyserver/internal/middleware"
	"easyserver/internal/model"
	"easyserver/internal/repository"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg                  *config.Config
	configPath           string
	db                   *sql.DB
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
}

// RouterDeps holds the shared service instances created once in main.go.
// Passing them in (instead of re-creating inside NewRouter) guarantees a
// single instance per service so that in-memory caches, background goroutines,
// and lifecycle hooks (Close, flush) all operate on the same state.
type RouterDeps struct {
	DB                   *sql.DB
	AuthService          *service.AuthService
	MonitorService       *service.MonitorService
	AuditService         *service.AuditService
	SessionService       *service.SessionService
	TotpService          *service.TOTPService
	AuditRepo            repository.AuditRepository
	ProcessManager       *service.ProcessManager
	SystemProcessService *service.SystemProcessService
	NotificationService  *service.NotificationService
}

func NewRouter(cfg *config.Config, configPath string, deps RouterDeps) *Router {
	// Wire notification + alert services onto the shared instances.
	notifyService := service.NewNotifyService(cfg.Notify.WebhookURL, cfg.Notify.Enabled)
	deps.AuthService.SetNotifyService(notifyService)

	// Initialize alert service
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
	deps.MonitorService.SetAlertService(alertService)

	return &Router{
		cfg:                  cfg,
		configPath:           configPath,
		db:                   deps.DB,
		auditRepo:            deps.AuditRepo,
		authService:          deps.AuthService,
		monitorService:       deps.MonitorService,
		auditService:         deps.AuditService,
		sessionService:       deps.SessionService,
		totpService:          deps.TotpService,
		alertService:         alertService,
		processManager:       deps.ProcessManager,
		systemProcessService: deps.SystemProcessService,
		notificationService:  deps.NotificationService,
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
	registerServiceRoutes(protected, wsGroup, r.cfg.Auth.JWTSecret, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerTerminalRoutes(protected, wsGroup, r.cfg.Auth.JWTSecret, r.auditService, r.cfg.Server.AllowedOrigins, r.cfg.Server.DevMode)
	registerFileRoutes(protected, r.cfg.FileManager.BasePath, r.auditService)
	registerAuditRoutes(protected, r.db, r.auditService, r.auditRepo)
	registerSettingsRoutes(protected, r.cfg, r.configPath, r.alertService)
	registerSystemRoutes(protected)
	registerCloudRoutes(protected, &r.cfg.TencentCloud, r.cfg.Server.Port)
	registerDeployRoutes(protected, r.db, r.cfg.Deploy.EncryptionKey)
	registerRuntimeRoutes(protected, r.db)
	registerEnvRoutes(protected, r.db)
	registerWebServerRoutes(protected, r.db)
	registerDatabaseRoutes(protected, r.db)
	registerCronRoutes(protected, r.db)
	registerFirewallRoutes(protected, r.db, r.cfg.Server.Port)
	registerSSHRoutes(protected)
	registerContainerRoutes(protected, r.auditService)
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
