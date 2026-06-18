package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/api"
	"easyserver/internal/config"
	"easyserver/internal/database"
	"easyserver/internal/service"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	devMode := flag.Bool("dev", false, "run in development mode (no embed, API only)")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Check for weak JWT secret
	if len(cfg.Auth.JWTSecret) < 32 {
		if !*devMode {
			log.Println("ERROR: JWT secret must be at least 32 bytes.")
			log.Println("Please set a strong secret in config.yaml or use EASYSERVER_JWT_SECRET environment variable.")
			os.Exit(1)
		}
		log.Println("WARNING: JWT secret is too short. Using in development mode only!")
	}

	// Check for default JWT secret
	defaultSecrets := []string{
		"easyserver-secret-key-change-me",
		"change-me-to-a-random-secret",
		"change-me-to-a-random-secret-at-least-32-bytes-long",
	}
	for _, defaultSecret := range defaultSecrets {
		if cfg.Auth.JWTSecret == defaultSecret {
			if !*devMode {
				log.Println("ERROR: JWT secret is set to a default value.")
				log.Println("Please set a strong secret in config.yaml or use EASYSERVER_JWT_SECRET environment variable.")
				os.Exit(1)
			}
			log.Println("WARNING: Using default JWT secret in development mode. Change it for production!")
			break
		}
	}

	// Validate file manager base path
	if cfg.FileManager.BasePath == "" {
		log.Fatalf("FATAL: filemanager.base_path is required")
	}
	if cfg.FileManager.BasePath == "/" {
		log.Fatalf("FATAL: filemanager.base_path cannot be '/' for security reasons")
	}

	// Check for default deploy encryption key
	defaultDeployKeys := []string{
		"change-me-to-a-random-32-byte-key!!",
		"change-me-to-a-random-32-byte-key",
	}
	for _, defaultKey := range defaultDeployKeys {
		if cfg.Deploy.EncryptionKey == defaultKey {
			if !*devMode {
				log.Println("ERROR: deploy.encryption_key is set to a default value.")
				log.Println("Please set a strong key in config.yaml or use EASYSERVER_ENCRYPTION_KEY environment variable.")
				os.Exit(1)
			}
			log.Println("WARNING: Using default deploy encryption key in development mode. Change it for production!")
			break
		}
	}

	// Initialize database
	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize auth service and default admin
	authService := service.NewAuthService(db, cfg.Auth.MaxLoginAttempts, cfg.Auth.LockoutDuration)
	if err := authService.InitDefaultAdmin(); err != nil {
		log.Fatalf("Failed to initialize default admin: %v", err)
	}

	// Initialize monitor service with WaitGroup for proper shutdown
	var monitorWg sync.WaitGroup
	monitorService := service.NewMonitorService(db, cfg.Monitor.CollectInterval, cfg.Monitor.HistoryRetention)
	monitorWg.Add(1)
	go func() {
		defer monitorWg.Done()
		monitorService.Start()
	}()

	// Initialize audit service and system event monitor
	auditService := service.NewAuditService(db)
	systemMonitor := service.NewSystemEventMonitor(auditService)
	systemMonitor.Start()

	// Initialize session service and start cleanup
	sessionService := service.NewSessionService(db)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := sessionService.CleanupExpiredSessions(); err != nil {
				log.Printf("session cleanup error: %v", err)
			}
		}
	}()

	// Log server start
	auditService.LogSystemEvent("SERVER_START", "EasyServer started")

	// Setup router with shared monitor service instance
	router := api.NewRouter(cfg, db, monitorService)
	r := router.Setup()

	// Create HTTP server with graceful shutdown support
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if *devMode {
		log.Printf("Starting EasyServer in DEV mode on %s", addr)
		log.Printf("Frontend should be served separately on port 5173")
	} else {
		log.Printf("Starting EasyServer on %s", addr)
	}

	// Start server in goroutine
	go func() {
		if cfg.Server.TLS.Enabled {
			if err := srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start server: %v", err)
			}
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server gracefully
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Stop system event monitor
	systemMonitor.Stop()

	// Log server shutdown
	auditService.LogSystemEvent("SERVER_STOP", "EasyServer stopped")

	// Stop monitor service
	monitorService.Stop()
	monitorWg.Wait()

	log.Println("Server exited properly")
}
