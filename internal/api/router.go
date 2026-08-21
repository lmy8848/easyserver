package api

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"easyserver/internal/domain/alert"
	"easyserver/internal/domain/audit"
	audithttp "easyserver/internal/domain/audit/http"
	"easyserver/internal/domain/auth"
	authhttp "easyserver/internal/domain/auth/http"
	"easyserver/internal/domain/container"
	containerhttp "easyserver/internal/domain/container/http"
	"easyserver/internal/domain/cron"
	cronhttp "easyserver/internal/domain/cron/http"
	dbdomain "easyserver/internal/domain/database"
	databasehttp "easyserver/internal/domain/database/http"
	"easyserver/internal/domain/envconfig"
	envconfighttp "easyserver/internal/domain/envconfig/http"
	"easyserver/internal/domain/filemanager"
	filemanagerhttp "easyserver/internal/domain/filemanager/http"
	"easyserver/internal/domain/firewall"
	firewallhttp "easyserver/internal/domain/firewall/http"
	"easyserver/internal/domain/monitor"
	monitorhttp "easyserver/internal/domain/monitor/http"
	"easyserver/internal/domain/notification"
	notificationhttp "easyserver/internal/domain/notification/http"
	"easyserver/internal/domain/notify"
	"easyserver/internal/domain/runtimeenv"
	runtimeenvhttp "easyserver/internal/domain/runtimeenv/http"
	"easyserver/internal/domain/security"
	securityhttp "easyserver/internal/domain/security/http"
	settingshttp "easyserver/internal/domain/settings/http"
	"easyserver/internal/domain/ssh"
	sshhttp "easyserver/internal/domain/ssh/http"
	"easyserver/internal/domain/systemd"
	systemdhttp "easyserver/internal/domain/systemd/http"
	"easyserver/internal/domain/terminal"
	terminalhttp "easyserver/internal/domain/terminal/http"
	"easyserver/internal/domain/web"
	webhttp "easyserver/internal/domain/web/http"
	websecurity "easyserver/internal/domain/web/security"
	websecurityhttp "easyserver/internal/domain/web/security/http"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
	"easyserver/internal/infra/mise"

	"github.com/gin-gonic/gin"
)

// Setup constructs the entire application service graph, registers all HTTP
// routes, and returns the handler plus a shutdown function. It is the single
// composition root for the HTTP layer. store 是配置唯一权威：需要实时配置的
// 服务持有 store 每次读最新快照；启动参数类服务取一次 cfg := store.Get()。
func Setup(store *config.Store, sig *infra.Signal) (http.Handler, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// 启动快照：一次性初始化参数（数据库路径、base_path、密钥等）从此取。
	cfg := store.Get()

	// ── Infrastructure ──

	db, err := database.Init()
	if err != nil {
		log.Fatalf("init database: %v", err)
	}

	miseProvider := mise.NewProvider()

	// ── Shared services (depended upon by others) ──

	notifyService := notify.NewService(store)

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
	// 未配置任何告警规则时 seed 默认规则（CPU/内存/磁盘），保证新安装默认有
	// 磁盘等关键告警；用户可在设置页修改/删除。
	if len(alertRules) == 0 {
		alertRules = defaultAlertRules()
	}
	alertService.SetRules(alertRules)

	auditRepo := audit.NewSQLiteRepository(db)
	auditSvc := audit.NewService(ctx, &wg, auditRepo, store)

	// ── Auth ──

	userRepo := auth.NewSQLiteUserRepository(db)
	totpRepo := auth.NewTOTPRepository(db)

	authSvc := auth.NewAuthService(ctx, &wg, store, userRepo, auditSvc, totpRepo, notifyService)
	if err := authSvc.InitDefaultAdmin(ctx); err != nil {
		log.Fatalf("init default admin: %v", err)
	}

	sessionSvc := auth.NewSessionService(ctx, &wg, store)

	qrLoginService := auth.NewQRLoginService(store, sessionSvc)

	// ── Monitor ──

	monitorRepo := monitor.NewSQLiteRepository(db)
	monitorSvc := monitor.NewMonitorService(ctx, &wg, monitorRepo, store, alertService, auditSvc)

	// ── Terminal ──

	terminalManager := terminal.NewManager(ctx, &wg, cfg.Auth.IdleTimeout.Duration())

	// ── Domain services (no background goroutines) ──

	cronRepo := cron.NewSQLiteRepository(db)

	envConfigRepo := envconfig.NewSQLiteRepository(db)
	envConfigService := envconfig.NewService(envConfigRepo)

	// runtimeService 必须先建：cron/systemd 用它校验运行时绑定（ADR-0009 目录权威）。
	runtimeService := runtimeenv.NewService(miseProvider)
	// 镜像源以 config.toml 文件为权威（文件即权威，独立于环境变量 API）。
	mirrorService := runtimeenv.NewMirrorService(mise.NewEnvStore())
	packageManagerService := runtimeenv.NewPackageService(miseProvider)

	cronService := cron.NewServiceWithSink(cronRepo, miseProvider, runtimeService, notificationService)

	serviceManager := systemd.NewServiceManager(runtimeService, miseProvider)

	container.SetAuthEnv()
	containerService := container.NewService()

	dbRepo := dbdomain.NewSQLiteRepository(db)
	dbService := dbdomain.NewServiceWithSink(dbRepo, dbdomain.NewSocketContainerRuntime(), notificationService)

	firewallRepo := firewall.NewSQLiteRepository(db)
	firewallService := firewall.NewService(firewallRepo, cfg.Server.Port)

	sshConfigService := ssh.NewService()

	webServerRepo := web.NewSQLiteServerRepository(db)
	websiteRepo := web.NewSQLiteWebsiteRepository(db)
	webServerSvc := web.NewService(webServerRepo)
	webServerSvc.SeedPredefinedWebServers(ctx)

	securityRepo := websecurity.NewSQLiteSecurityRepository(db)
	securitySvc := websecurity.NewSecurityService(securityRepo, firewallService)
	websiteSvc := web.NewWebsiteService(websiteRepo, webServerRepo, securityRepo)

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

	// ── HTTP Engine + Routes ──

	e := newEngine(cfg)
	g := newGroups(e, cfg, authSvc, sessionSvc, auditSvc)

	// Domain route registration
	authhttp.RegisterRoutes(g.API, authSvc, auditSvc, sessionSvc, qrLoginService, cfg.Auth.JWTSecret, g.sessionValidator, cfg.Auth.SessionTimeout.Duration(), cfg.Auth.LoginRateLimit, cfg.Auth.LoginRateInterval.Duration(), cfg)
	monitorhttp.RegisterRoutes(g.Protected, g.WS, monitorSvc, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	systemdhttp.RegisterRoutes(g.Protected, g.WS, serviceManager, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	terminalhttp.RegisterRoutes(g.WS, terminalManager, cfg.Auth.JWTSecret, auditSvc, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	filemanagerhttp.RegisterRoutes(g.Protected, g.File, fileManager, g.maxUploadSize)
	audithttp.RegisterRoutes(g.Protected, db, auditRepo)
	settingshttp.RegisterRoutes(g.Protected, store, alertService, monitorSvc, sig)
	runtimeenvhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), runtimeService, packageManagerService, mirrorService)
	envconfighttp.RegisterRoutes(g.Protected, envConfigService)
	webhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), webServerSvc, websiteSvc)
	databasehttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), dbService)
	cronhttp.RegisterRoutes(g.Protected, cronService)
	firewallhttp.RegisterRoutes(g.Protected, firewallService, cfg.Server.Port)
	sshhttp.RegisterRoutes(g.Protected, sshConfigService)
	containerhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), containerService)
	notificationhttp.RegisterRoutes(g.Protected, notificationService)
	securityhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), security.NewService(db, cfg.Path))
	if securitySvc != nil {
		secHandler := websecurityhttp.NewSecurityHandler(securitySvc)
		secHandler.RegisterRoutes(g.Protected.Group("/websites"))
	}
	filemanagerhttp.RegisterShareRoutes(g.Protected, fileShareRepo, fileManager, cfg)
	filemanagerhttp.RegisterPublicShareRoute(g.API, fileShareRepo, fileManager, cfg.Auth.RateLimit, cfg.Auth.RateInterval.Duration(), cfg)

	// Static frontend (in dev mode, ServeWeb is a no-op via embed_dev.go)
	e.Use(middleware.RateLimitMiddleware("assets", cfg.Server.AssetsRateLimit, cfg.Server.AssetsRateInterval.Duration()))
	ServeWeb(e)

	// ── Background watchers ──
	watcherStop := make(chan struct{})
	cronService.StartWatcher(watcherStop)

	// ── Shutdown function ──
	shutdown := func() {
		auditSvc.Close()
		close(watcherStop)
		cancel()
		wg.Wait()
		middleware.StopRateLimiter()
		middleware.StopSessionHeartbeatLimiter()
		db.Close()
	}

	return e, shutdown
}

// defaultAlertRules 返回未配置告警规则时的 seed 默认规则。
func defaultAlertRules() []alert.AlertRule {
	return []alert.AlertRule{
		{ID: 1, Name: "CPU 告警", Metric: "cpu_percent", Threshold: 90, Duration: 60, Enabled: true},
		{ID: 2, Name: "内存告警", Metric: "mem_percent", Threshold: 85, Duration: 60, Enabled: true},
		{ID: 3, Name: "磁盘告警", Metric: "disk_percent", Threshold: 90, Duration: 60, Enabled: true},
	}
}

// routeGroups holds the pre-configured gin router groups.
type routeGroups struct {
	API              *gin.RouterGroup
	Protected        *gin.RouterGroup
	WS               *gin.RouterGroup
	File             *gin.RouterGroup
	maxUploadSize    int64
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

	ipWhitelist := middleware.NewIPWhitelist(cfg.Auth.IPWhitelist)
	middleware.RegisterIPWhitelist(ipWhitelist)

	e.Use(gin.Logger(), gin.Recovery(),
		middleware.ErrorHandler(),
		middleware.DomainRedirectMiddleware(cfg.Server.Domain, cfg.Server.ForceDomain),
		middleware.SecurityMiddleware(cspNonce, cfg.Server.AllowedOrigins, cfg.Server.DevMode),
		middleware.CORSMiddleware(cfg.Server.AllowedOrigins, cfg.Server.DevMode),
		middleware.IPWhitelistMiddleware(ipWhitelist),
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

	sessionValidator := func(token string) (bool, error) {
		return sessionSvc.IsSessionValid(context.Background(), token)
	}

	api := e.Group("/api")
	api.Use(
		middleware.MaxBodySizeMiddleware(maxUploadSize),
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval.Duration()),
	)

	protected := api.Group("")
	protected.Use(
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator),
		middleware.SessionHeartbeatMiddleware(sessionSvc.UpdateActivity, cfg.Auth.SessionTimeout.Duration()),
		middleware.AuditMiddleware(auditSvc),
	)

	// File upload sub-group
	fileRoutes := api.Group("/files",
		middleware.ReadTimeout(10*time.Minute),
		middleware.WriteTimeout(10*time.Minute),
	)
	fileRoutes.Use(
		middleware.RateLimitMiddleware("api", cfg.Auth.RateLimit, cfg.Auth.RateInterval.Duration()),
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, sessionValidator),
		middleware.SessionHeartbeatMiddleware(sessionSvc.UpdateActivity, cfg.Auth.SessionTimeout.Duration()),
		middleware.AuditMiddleware(auditSvc),
	)

	wsGroup := e.Group("/ws")
	wsGroup.Use(middleware.WSAuthMiddleware(cfg.Auth.JWTSecret, sessionValidator))

	return &routeGroups{
		API:              api,
		Protected:        protected,
		WS:               wsGroup,
		File:             fileRoutes,
		maxUploadSize:    maxUploadSize,
		sessionValidator: sessionValidator,
	}
}
