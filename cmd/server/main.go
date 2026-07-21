package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"easyserver/internal/api"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/launcher"
	"easyserver/internal/infra/version"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	devMode := flag.Bool("dev", false, "run in development mode (no embed, API only)")
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(version.GetFullVersionString())
		return
	}

	if args := flag.Args(); len(args) > 0 {
		runCLI(args[0], *configPath)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	validateConfig(cfg, *devMode)

	services, err := wire(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Acquire listener: inherit FD from parent (hot restart) or bind fresh.
	// Launcher holds it so the settings handler can pass the FD to a child on restart.
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	ln, err := launcher.AcquireListener(addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	services.Launcher = launcher.New(ln)

	srv := &http.Server{
		Addr:         addr,
		Handler:      api.Setup(cfg, *configPath, services.RouterDeps),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second, // long-running routes override via middleware.WriteTimeout
		IdleTimeout:  60 * time.Second,
	}

	if *devMode {
		log.Printf("Starting EasyServer in DEV mode on %s (frontend on port 5173)", srv.Addr)
	} else {
		log.Printf("Starting EasyServer on %s", srv.Addr)
	}

	infra.Go(func() {
		var err error
		if cfg.Server.TLS.Enabled {
			err = srv.ServeTLS(ln, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			err = srv.Serve(ln)
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	services.Shutdown()
	log.Println("Server exited properly")
}
