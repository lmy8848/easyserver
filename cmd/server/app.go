package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"easyserver/internal/api"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/mise"
)

// App is the process shell: owns the HTTP server, TCP listener, restart signal,
// and shutdown coordination. All application composition lives in internal/api.
type App struct {
	srv        *http.Server
	ln         net.Listener
	sig        *infra.Signal
	cfg        *config.Config
	configPath string
	devMode    bool
	shutdown   func()
}

// NewApp constructs the process shell. Application services are created later
// by api.Setup during Run.
func NewApp(cfg *config.Config, configPath string, devMode bool) *App {
	a := &App{
		cfg:        cfg,
		configPath: configPath,
		devMode:    devMode,
		sig:        infra.NewSignal(),
		srv: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
	return a
}

// Run builds the application via api.Setup, binds the listener, and enters the
// event loop (signals, hot restart).
func (a *App) Run() {
	// 进程级基础设施初始化：确保运行环境底层（mise）二进制到位。属于进程壳
	// 而非 HTTP 编排层（api.Setup），故放在这里。失败不阻断启动，留待后续报错。
	if err := mise.BootstrapMise(); err != nil {
		log.Printf("ERROR: Failed to bootstrap mise runtime manager: %v", err)
	}

	handler, shutdown := api.Setup(a.cfg, a.configPath, a.sig)
	a.shutdown = shutdown
	a.srv.Handler = handler

	if err := a.acquireListener(a.srv.Addr); err != nil {
		log.Fatalf("Failed to listen on %s: %v", a.srv.Addr, err)
	}

	if a.devMode {
		log.Printf("Starting EasyServer in DEV mode on %s (frontend on port 5173)", a.srv.Addr)
	} else {
		log.Printf("Starting EasyServer on %s", a.srv.Addr)
	}

	errCh := make(chan error, 1)
	go func() {
		err := a.serve()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
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
			a.gracefulShutdown(30 * time.Second)
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

// gracefulShutdown drains HTTP connections then shuts down application services.
func (a *App) gracefulShutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	if a.shutdown != nil {
		a.shutdown()
	}
}

// acquireListener inherits a parent FD (hot restart) or binds fresh.
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
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}
	a.ln = ln
	return nil
}

// hotRestart forks a child process and exits the parent.
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
		child, err := os.StartProcess(execPath, args, &os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Env:   os.Environ(),
		})
		if err != nil {
			log.Printf("app: restart failed: fork (force): %v", err)
			return
		}
		log.Printf("app: forked child PID %d (force restart), exiting parent", child.Pid)
		a.gracefulShutdown(10 * time.Second)
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

	a.gracefulShutdown(10 * time.Second)
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
