package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/alert"
	"easyserver/internal/api"
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
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
	"easyserver/internal/infra/executor"
	"easyserver/internal/monitor"
	"easyserver/internal/notification"
	"easyserver/internal/notify"
	"easyserver/internal/qrlogin"
	"easyserver/internal/runtimeenv"
	"easyserver/internal/ssh"
	"easyserver/internal/systemd"
	"easyserver/internal/systemprocess"
	"easyserver/internal/terminal"
	"easyserver/internal/web"
)

// App 是应用运行时容器：持有 HTTP server、TCP listener、重启信号、所有
// 领域服务（内嵌 RouterDeps）以及 LIFO 清理栈。合并了原 appServices 和
// internal/infra/server.Server 的职责。
type App struct {
	api.RouterDeps
	srv        *http.Server
	ln         net.Listener
	sig        *infra.Signal
	cfg        *config.Config
	configPath string
	devMode    bool
	cleanups   []func()
}

// newApp 构造 App 壳：创建 http.Server（Handler 后设）和重启信号。
// 领域服务由 wire() 填充 RouterDeps 字段，清理函数由 wire 注册。
func NewApp(cfg *config.Config, configPath string, devMode bool) *App {
	a := &App{
		cfg:        cfg,
		configPath: configPath,
		devMode:    devMode,
		sig:        infra.NewSignal(),
		srv: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second, // long-running routes override via middleware.WriteTimeout
			IdleTimeout:  60 * time.Second,
		},
	}
	a.Signal = a.sig // 注入 RouterDeps.Signal 给 settings handler
	return a
}

func (a *App) onCleanup(fn func()) { a.cleanups = append(a.cleanups, fn) }

// SetHandler 设置 HTTP handler（gin engine）。
func (a *App) SetHandler(h http.Handler) { a.srv.Handler = h }

// acquireListener 继承父进程 FD（热重启）或新建绑定。
func (a *App) acquireListener(addr string) error {
	if inheritFD := os.Getenv("EASYSERVER_INHERIT_FD"); inheritFD != "" {
		if fdNum, err := strconv.Atoi(inheritFD); err == nil {
			f := os.NewFile(uintptr(fdNum), "listener")
			if f != nil {
				ln, err := net.FileListener(f)
				f.Close()
				if err == nil {
					log.Printf("app: inherited listener from parent on %s", addr)
					a.ln = ln
					return nil
				}
			}
		}
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	a.ln = ln
	return nil
}

// Run 绑定 listener、注册路由并进入事件循环。wire 构造 App 后由 main 调用。
func (a *App) Run() {
	if err := a.acquireListener(a.srv.Addr); err != nil {
		log.Fatalf("Failed to listen on %s: %v", a.srv.Addr, err)
	}
	a.SetHandler(api.Setup(a.cfg, a.configPath, a.RouterDeps))

	if a.devMode {
		log.Printf("Starting EasyServer in DEV mode on %s (frontend on port 5173)", a.srv.Addr)
	} else {
		log.Printf("Starting EasyServer on %s", a.srv.Addr)
	}

	errCh := make(chan error, 1)
	go func() {
		err := a.serve()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errCh:
			log.Fatalf("Failed to start server: %v", err)
		case <-quit:
			log.Println("Shutting down server...")
			a.shutdown(30 * time.Second)
			log.Println("Server exited properly")
			return
		case opts := <-a.sig.C():
			a.hotRestart(opts)
		}
	}
}

func (a *App) serve() error {
	if a.cfg.Server.TLS.Enabled {
		return a.srv.ServeTLS(a.ln, a.cfg.Server.TLS.CertFile, a.cfg.Server.TLS.KeyFile)
	}
	return a.srv.Serve(a.ln)
}

// shutdown 关闭 HTTP server（等待活动连接最多 timeout）并执行 LIFO 清理。
func (a *App) shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	for i := len(a.cleanups) - 1; i >= 0; i-- {
		a.cleanups[i]()
	}
}

// hotRestart fork 子进程并退出父进程。
//
// Force 模式：先关 listener，子进程在新地址重新绑定，不继承 FD。
//
// Graceful 模式：listener FD 传给子进程实现零中断重启。fork 后父进程
// 执行资源清理并等待活动连接收尾（最多 10s）再退出 - 修复了原先 os.Exit
// 跳过清理的问题。子进程持有 dup 出的 FD 副本，父进程关 listener 不影响它。
func (a *App) hotRestart(opts infra.RestartOpts) {
	log.Printf("app: restarting panel (force=%v)...", opts.Force)

	execPath, err := os.Executable()
	if err != nil {
		log.Printf("app: restart failed: resolve executable: %v", err)
		return
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		log.Printf("app: restart failed: resolve symlink: %v", err)
		return
	}

	args := []string{execPath, "-config", opts.ConfigPath}
	if opts.DevMode {
		args = append(args, "-dev")
	}

	if opts.Force {
		// Fork child BEFORE closing the listener: if fork fails, the old
		// listener stays open and the service continues serving on the
		// previous address instead of going down with no recovery path.
		// The child does not inherit the FD and calls net.Listen fresh.
		child, err := os.StartProcess(execPath, args, &os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Env:   os.Environ(),
		})
		if err != nil {
			log.Printf("app: restart failed: fork (force): %v", err)
			return
		}
		log.Printf("app: forked child PID %d (force restart), exiting parent", child.Pid)
		a.shutdown(10 * time.Second)
		os.Exit(0)
	}

	listenerFile := a.dupListenerFile()
	if listenerFile == nil {
		log.Printf("app: restart failed: no listener available")
		return
	}
	childEnv := append(os.Environ(), "EASYSERVER_INHERIT_FD=3")
	child, err := os.StartProcess(execPath, args, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, listenerFile},
		Env:   childEnv,
	})
	listenerFile.Close()
	if err != nil {
		log.Printf("app: restart failed: fork: %v", err)
		return
	}
	log.Printf("app: forked child PID %d, exiting parent", child.Pid)

	a.shutdown(10 * time.Second)
	os.Exit(0)
}

func (a *App) dupListenerFile() *os.File {
	tcpLn, ok := a.ln.(*net.TCPListener)
	if !ok {
		return nil
	}
	f, err := tcpLn.File()
	if err != nil {
		return nil
	}
	return f
}

// wire builds every service from cfg, starts the required background goroutines,
// and registers their cleanups. Caller must Shutdown() the result.
func (a *App) wire() error {
	if err := runtimeenv.BootstrapMise(); err != nil {
		log.Printf("ERROR: Failed to bootstrap mise runtime manager: %v", err)
	}

	ctx := context.Background()

	db, err := database.Init(a.cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	a.DB = db
	a.onCleanup(func() { db.Close() })

	// Repositories — only auditRepo is exposed downstream.
	userRepo := auth.NewSQLiteUserRepository(db)
	sessionRepo := auth.NewSQLiteSessionRepository(db, a.cfg.Auth.IdleTimeout)
	tokenRepo := auth.NewSQLiteTokenRepository(db)
	auditRepo := audit.NewSQLiteRepository(db)
	activityRepo := auth.NewSQLiteActivityRepository(db)
	totpRepo := auth.NewTOTPRepository(db)
	monitorRepo := monitor.NewSQLiteRepository(db)
	a.AuditRepo = auditRepo

	// Auth + default admin
	authSvc := auth.NewAuthService(a.cfg.Auth.MaxLoginAttempts, a.cfg.Auth.LockoutDuration)
	authSvc.SetRepositories(userRepo, tokenRepo, activityRepo, totpRepo)
	if err := authSvc.InitDefaultAdmin(ctx); err != nil {
		return fmt.Errorf("init default admin: %w", err)
	}
	a.AuthService = authSvc
	// Stop auth cleanup loops on shutdown BEFORE db.Close() so they don't touch
	// a closed database. Registered first so it runs last (LIFO cleanup order).
	a.onCleanup(func() { authSvc.Close() })

	// Monitor service + collection goroutine
	var monitorWg sync.WaitGroup
	monitorSvc := monitor.NewMonitorService(monitorRepo, a.cfg.Monitor.CollectInterval, a.cfg.Monitor.HistoryRetention)
	monitorWg.Add(1)
	infra.Go(func() {
		defer monitorWg.Done()
		monitorSvc.Start()
	})
	a.MonitorService = monitorSvc
	a.onCleanup(func() {
		monitorSvc.Stop()
		monitorWg.Wait()
	})

	// Shared command executor
	cmdExec := executor.NewOSExecutor()
	a.Executor = cmdExec

	// Audit + system event monitor — audit cleanup writes SERVER_STOP then flushes.
	auditSvc := audit.NewService(db, auditRepo, a.cfg.Audit.RetentionDays)
	a.AuditService = auditSvc
	a.onCleanup(func() {
		auditSvc.LogSystemEvent(context.Background(), "面板停止")
		auditSvc.Close()
	})

	monitorSvc.SetAuditService(auditSvc)

	// Session service + cleanup goroutine
	sessionSvc := auth.NewSessionService(sessionRepo)
	a.SessionService = sessionSvc

	// QR login service (scan-to-login): depends on session service for coexisting web sessions.
	a.QRLoginService = qrlogin.NewService(qrlogin.NewSQLiteRepository(db), a.cfg.Auth.JWTSecret, a.cfg.Auth.SessionTimeout, sessionSvc)
	sessionDone := make(chan struct{})
	infra.Go(func() {
		ticker := time.NewTicker(a.cfg.Auth.SessionCleanupInterval)
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
	a.onCleanup(func() { close(sessionDone) })

	// Package-level rate limiters
	a.onCleanup(middleware.StopRateLimiter)
	a.onCleanup(middleware.StopSessionHeartbeatLimiter)

	// System process service
	a.SystemProcessService = systemprocess.NewService()

	// Notification
	notificationRepo := notification.NewSQLiteRepository(db)
	a.NotificationService = notification.NewService(notificationRepo)

	// Container, systemd, cron
	a.ContainerService = container.NewService(cmdExec)
	cronRepo := cron.NewSQLiteRepository(db)
	a.ServiceManager = systemd.NewServiceManager(cmdExec)
	a.ServiceManager.SetRuntimeLookup(cronRepo) // 注入 runtime 查询，托管 unit 绑定 runtime 时补 lang/exact
	a.CronService = cron.NewService(cronRepo, cmdExec)
	// Sync enabled cron tasks to /etc/cron.d on startup so schedules take effect
	// after a panel restart (the CRUD methods also sync on change).
	if err := a.CronService.SyncToSystemCrontab(ctx); err != nil {
		log.Printf("cron: startup sync to system crontab failed: %v", err)
	}

	// Database services (with seeding)
	dbServerRepo := dbserver.NewSQLiteRepository(db)
	dbServerService := dbserver.NewService(cmdExec, dbServerRepo)
	dbServerService.SeedPredefinedServers(ctx)
	a.DBServerService = dbServerService

	databaseMgmtRepo := database_mgmt.NewSQLiteRepository(db)
	a.DatabaseMgmtService = database_mgmt.NewService(databaseMgmtRepo, cmdExec)

	// Deploy
	deployRepo := deploy.NewSQLiteRepository(db)
	deploySvc, err := deploy.NewService(deployRepo, a.cfg.Deploy.EncryptionKey)
	if err != nil {
		return fmt.Errorf("init deploy service: %w", err)
	}
	a.DeployService = deploySvc

	// Env config (with seeding)
	envConfigRepo := envconfig.NewSQLiteRepository(db)
	envConfigService := envconfig.NewService(envConfigRepo)
	a.EnvConfigService = envConfigService

	// Firewall (protect SSH + panel port from being accidentally blocked)
	firewallRepo := firewall.NewSQLiteRepository(db)
	a.FirewallService = firewall.NewService(firewallRepo, cmdExec, a.cfg.Server.Port)

	// Runtime + package
	runtimeRepo := runtimeenv.NewSQLiteRepository(db)
	runtimeService := runtimeenv.NewService(runtimeRepo, cmdExec, envConfigService)
	if err := runtimeService.Init(ctx); err != nil {
		log.Printf("ERROR: Failed to init runtime service: %v", err)
	}
	a.RuntimeService = runtimeService
	a.PackageManagerService = runtimeenv.NewPackageService(cmdExec)

	// SSH
	a.SSHConfigService = ssh.NewService(cmdExec)

	// Web server + websites (with seed)
	webServerRepo := web.NewSQLiteServerRepository(db)
	websiteRepo := web.NewSQLiteWebsiteRepository(db)
	webServerSvc := web.NewService(webServerRepo, cmdExec)
	webServerSvc.SeedPredefinedWebServers(ctx)
	a.WebServerService = webServerSvc
	a.WebsiteService = web.NewWebsiteService(websiteRepo, webServerRepo, cmdExec)

	// Notify + alert (wired into auth + monitor)
	notifyService := notify.NewService(a.cfg.Notify.WebhookURL, a.cfg.Notify.Enabled)
	authSvc.SetNotifyService(notifyService)
	a.NotifyService = notifyService

	alertService := alert.NewService(notifyService, a.NotificationService)
	alertRules := make([]alert.AlertRule, 0, len(a.cfg.Alerts.Rules))
	for i, rule := range a.cfg.Alerts.Rules {
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
	a.AlertService = alertService

	// Terminal manager
	terminalManager := terminal.NewManager(cmdExec)
	terminalManager.StartIdleTimeout(a.cfg.Auth.IdleTimeout)
	a.TerminalManager = terminalManager
	a.onCleanup(func() { terminalManager.StopIdleTimeout() })

	// File manager
	fileManager, err := filemanager.NewManager(a.cfg.FileManager.BasePath)
	if err != nil {
		return fmt.Errorf("init file manager: %w", err)
	}
	a.FileManager = fileManager

	// File share repository + startup cleanup
	a.FileShareRepo = fileshare.NewSQLiteShareRepository(db)
	if deleted, err := a.FileShareRepo.DeleteExpired(ctx); err != nil {
		log.Printf("ERROR: failed to cleanup expired file shares on startup: %v", err)
	} else if deleted > 0 {
		log.Printf("Cleaned up %d expired file shares on startup", deleted)
	}

	// Cloud service (optional)
	if a.cfg.TencentCloud.Enabled {
		cloudService, err := cloud.NewService(
			a.cfg.TencentCloud.SecretID,
			a.cfg.TencentCloud.SecretKey,
			a.cfg.TencentCloud.Region,
			a.cfg.TencentCloud.InstanceID,
		)
		if err != nil {
			log.Printf("WARNING: failed to init cloud service: %v", err)
		} else {
			a.CloudService = cloudService
		}
	}

	auditSvc.LogSystemEvent(ctx, "面板启动")
	return nil
}
