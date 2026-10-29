package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	"easyserver/internal/mobiledirect"
	"easyserver/internal/plugin"

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

	// 面板服务 token（panel.api.proxy 自请求用）：进程内随机生成、不持久化，
	// 仅 Host API 自请求时携带，对外不暴露。
	panelServiceToken := generateServiceToken()
	// 面板自身地址（插件 Host API 自请求用）：loopback + 实际监听端口
	panelScheme := "http"
	if cfg.Server.TLS.Enabled {
		panelScheme = "https"
	}
	panelBaseURL := fmt.Sprintf("%s://127.0.0.1:%d", panelScheme, cfg.Server.Port)

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
	authSvc.SetEventPublisher(plugin.PublishBusinessEvent) // 登录事件 → 插件总线
	if err := authSvc.InitDefaultAdmin(ctx); err != nil {
		log.Fatalf("init default admin: %v", err)
	}

	sessionSvc := auth.NewSessionService(ctx, &wg, store)

	// 移动端直连通道管理器：设备签名通过后铸造 15 分钟短期 JWT 并写入会话表，
	// 由直连端以 Cookie 转发给主引擎，复用既有鉴权与审计。
	mdStore := mobiledirect.NewStore(db)
	mintMobileToken := func(mctx context.Context, userID int64) (string, time.Time, error) {
		user, uerr := authSvc.GetUserByID(mctx, userID)
		if uerr != nil {
			return "", time.Time{}, uerr
		}
		ttl := 15 * time.Minute
		tok, terr := auth.GenerateToken(cfg.Auth.JWTSecret, userID, user.Username, ttl)
		if terr != nil {
			return "", time.Time{}, terr
		}
		expires := time.Now().Add(ttl)
		if serr := sessionSvc.CreateSession(mctx, tok, userID, "mobile-direct", "device-signature", "mobile-direct", expires); serr != nil {
			return "", time.Time{}, serr
		}
		return tok, expires, nil
	}
	panelSchemeM := "http"
	if cfg.Server.TLS.Enabled {
		panelSchemeM = "https"
	}
	refreshSvc := auth.NewRefreshService(db)
	mdManager := mobiledirect.NewManager(mdStore, fmt.Sprintf("%s://127.0.0.1:%d", panelSchemeM, cfg.Server.Port), mintMobileToken)

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
	portForwardRepo := firewall.NewPortForwardSQLiteRepository(db)
	portForwardService := firewall.NewPortForwardService(portForwardRepo, firewallService)
	portACLRepo := firewall.NewPortACLSQLiteRepository(db)
	portACLService := firewall.NewPortACLService(portACLRepo)

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
	g := newGroups(e, cfg, authSvc, sessionSvc, auditSvc, panelServiceToken)

	// 移动端直连接口的设备公钥管理（注册/列表/吊销）走受保护路由（通常经 CF）。
	// 直连端口只做签名校验与反向代理，见 internal/mobiledirect。
	mobiledirect.RegisterDeviceRoutes(g.Protected, mdStore, refreshSvc)

	// 移动端刷新令牌：POST /api/auth/refresh（公开，refresh 走请求体 + 设备绑定）
	authhttp.RegisterRefreshRoute(g.API, refreshSvc, cfg.Auth.JWTSecret, sessionSvc,
		int64(cfg.Auth.SessionTimeout.Duration().Seconds()),
		cfg.Auth.LoginRateLimit,
		int64(cfg.Auth.LoginRateInterval.Duration().Seconds()))

	// Domain route registration
	authhttp.RegisterRoutes(g.API, authSvc, auditSvc, sessionSvc, qrLoginService, refreshSvc, cfg.Auth.JWTSecret, g.sessionValidator, cfg.Auth.SessionTimeout.Duration(), cfg.Auth.LoginRateLimit, cfg.Auth.LoginRateInterval.Duration(), cfg)
	monitorhttp.RegisterRoutes(g.Protected, monitorSvc, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	systemdhttp.RegisterRoutes(g.Protected, serviceManager, cfg.Auth.JWTSecret, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	terminalhttp.RegisterRoutes(g.WS, terminalManager, cfg.Auth.JWTSecret, auditSvc, cfg.Server.AllowedOrigins, cfg.Server.DevMode)
	filemanagerhttp.RegisterRoutes(g.Protected, g.File, fileManager, g.maxUploadSize)
	audithttp.RegisterRoutes(g.Protected, db, auditRepo, auditSvc)
	settingshttp.RegisterRoutes(g.Protected, store, alertService, monitorSvc, sig, mdManager)
	runtimeenvhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), runtimeService, packageManagerService, mirrorService)
	envconfighttp.RegisterRoutes(g.Protected, envConfigService)
	webhttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), webServerSvc, websiteSvc)
	databasehttp.RegisterRoutes(g.Protected.Group("", middleware.WriteTimeout(10*time.Minute)), dbService)
	cronhttp.RegisterRoutes(g.Protected, cronService)
	firewallhttp.RegisterRoutes(g.Protected, firewallService, cfg.Server.Port)
	firewallhttp.RegisterPortForwardRoutes(g.Protected, portForwardService)

	// ── 插件系统基座（文档 docs/plugin-system.md 终稿 v1.0）──
	pluginRepo, err := plugin.NewSQLiteRepository(db, cfg.Auth.JWTSecret)
	if err != nil {
		log.Fatalf("plugin: 密钥加密初始化失败: %v", err)
	}
	pluginSup := plugin.NewSupervisor(pluginRepo, plugin.HostSocketPath, plugin.SocketDir, plugin.PluginsDir)
	pluginBus := plugin.NewEventBus()
	plugin.SetDefaultBus(pluginBus)
	pluginMgr := plugin.NewManager(plugin.ManagerOptions{
		Repo:       pluginRepo,
		Supervisor: pluginSup,
		Bus:        pluginBus,
		Storage:    plugin.NewPluginStorage(config.DataRoot + "/data/plugin-storage"),
		PluginsDir: plugin.PluginsDir,
		AuditFn: func(ctx context.Context, pluginName, summary string) {
			auditSvc.LogSecurityEvent(ctx, "plugin:"+pluginName, summary)
		},
		NotifyFn: func(ctx context.Context, title, content string) error {
			return notify.NewService(store).SendCustom(ctx, title, content)
		},
		// 暴露端口访问控制：插件经 Host API port.expose/revoke 落到防火墙 PortACL
		PortExposer: portACLExposerAdapter{svc: portACLService},
		// 面板服务级 API 代理（panel.api.proxy 只读白名单）
		PanelAPI: plugin.PanelAPIConfig{
			BaseURL: panelBaseURL,
			Token:   panelServiceToken,
			Domain:  cfg.Server.Domain,
		},
		// 插件源（市场入口）
		Sources: plugin.NewSourceRepository(db),
	})
	plugin.RegisterRoutes(g.Protected, pluginMgr)
	// 插件注册的 web 服务器类型（功能补充扩展点）：面板网站管理并入可选类型
	webServerSvc.SetPluginWebServers(func() []web.WebServer {
		contribs := pluginMgr.WebServers()
		out := make([]web.WebServer, 0, len(contribs))
		for _, c := range contribs {
			out = append(out, web.WebServer{
				Name: c.Name, DisplayName: c.DisplayName, Description: c.Description,
				InstallCmd: c.InstallCmd, UninstallCmd: c.UninstallCmd,
				ConfigPath: c.ConfigPath, ConfigFile: c.ConfigFile,
				SitesAvailable: c.SitesAvailable, SitesEnabled: c.SitesEnabled,
				ServiceName: c.ServiceName, BinaryPath: c.BinaryPath,
				DefaultPort: c.DefaultPort, LogDir: c.LogDir,
				TestCmd: c.TestCmd, VersionCmd: c.VersionCmd,
			})
		}
		return out
	})
	// 插件业务代理：NoRoute 兜底 + JWT 认证（与 protected 组同策略）
	plugin.SetProxyHandler(func(c *gin.Context) {
		middleware.JWTMiddleware(cfg.Auth.JWTSecret, g.sessionValidator)(c)
		if c.IsAborted() {
			return
		}
		plugin.NewProxyHandler(pluginMgr)(c)
	})
	pluginHost := plugin.NewHostAPI(plugin.HostSocketPath, pluginMgr)
	if err := pluginHost.Start(ctx); err != nil {
		log.Printf("plugin: Host API 启动失败: %v", err)
	}
	if err := pluginMgr.ScanPlugins(ctx); err != nil {
		log.Printf("plugin: 插件扫描失败: %v", err)
	}
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

	// ── 面板启动：端口转发规则重放（内核态与数据库对齐） ──
	if err := portForwardService.ReapplyAll(context.Background()); err != nil {
		log.Printf("firewall: 端口转发规则重放失败: %v", err)
	}
	portForwardService.WarnBackendOnce(context.Background())
	// 暴露端口访问控制重放（插件暴露端口的授权源白名单）
	if err := portACLService.ReapplyAll(context.Background()); err != nil {
		log.Printf("firewall: 暴露端口访问控制重放失败: %v", err)
	}

	// ── Background watchers ──
	watcherStop := make(chan struct{})
	cronService.StartWatcher(watcherStop)

	// ── 移动端直连通道（可选，独立 TLS 端口，绕过 CF）──
	if cfg.MobileDirect.Enabled {
		if err := mdManager.Apply(cfg.MobileDirect); err != nil {
			log.Printf("mobile direct: start failed: %v", err)
		}
	}

	// ── Shutdown function ──
	shutdown := func() {
		pluginSup.StopAll(context.Background())
		pluginMgr.Close()
		_ = pluginHost.Stop(context.Background())
		auditSvc.Close()
		close(watcherStop)
		cancel()
		wg.Wait()
		middleware.StopRateLimiter()
		middleware.StopSessionHeartbeatLimiter()
		mdManager.Stop()
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
	// 禁用尾斜杠 301 重定向：插件代理路径 /api/plugins/<id>/（带尾斜杠）若被
	// redirect 到无尾斜杠，会在 CF 下触发二次连接导致 CF 502；NoRoute 兜底
	// 已能处理两种形态（代理内剥前缀）
	e.RedirectTrailingSlash = false
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

	e.Use(middleware.RequestIDMiddleware(),
		middleware.RequestLoggerMiddleware(), middleware.PanicRecovery(),
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
func newGroups(e *gin.Engine, cfg *config.Config, authSvc *auth.AuthService, sessionSvc *auth.SessionService, auditSvc *audit.Service, serviceToken string) *routeGroups {
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
		// 服务级调用（panel.api.proxy 自请求）优先：命中服务 token 则跳过用户 JWT
		middleware.ServiceTokenMiddleware(serviceToken),
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

// portACLExposerAdapter 把防火墙 PortACLService 适配为插件可用的
// plugin.PortExposer（Host API port.expose / port.revoke / port.list 落点）。
type portACLExposerAdapter struct {
	svc *firewall.PortACLService
}

func (a portACLExposerAdapter) Expose(ctx context.Context, port int, protocol string, cidrs []string, remark string) error {
	return a.svc.Expose(ctx, port, protocol, cidrs, remark)
}

func (a portACLExposerAdapter) Revoke(ctx context.Context, port int, protocol string) error {
	return a.svc.Revoke(ctx, port, protocol)
}

func (a portACLExposerAdapter) List(ctx context.Context) ([]plugin.PortACLInfo, error) {
	items, err := a.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]plugin.PortACLInfo, 0, len(items))
	for _, it := range items {
		out = append(out, plugin.PortACLInfo{
			Port: it.Port, Protocol: it.Protocol, CIDRs: it.CIDRs, Remark: it.Remark,
		})
	}
	return out, nil
}

// generateServiceToken 生成面板服务 token（32 字节随机 hex），
// 供插件 Host API panel.api.proxy 自请求面板业务 API 时鉴权。
func generateServiceToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("generate service token: " + err.Error())
	}
	return hex.EncodeToString(b)
}
