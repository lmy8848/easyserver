package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"easyserver/internal/api"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	devMode := flag.Bool("dev", false, "run in development mode (no embed, API only)")
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(api.GetFullVersionString())
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

	router := api.NewRouter(cfg, *configPath, services.RouterDeps)
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router.Setup(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second, // long-running routes override via middleware.WriteTimeout
		IdleTimeout:  60 * time.Second,
	}

	if *devMode {
		log.Printf("Starting EasyServer in DEV mode on %s (frontend on port 5173)", srv.Addr)
	} else {
		log.Printf("Starting EasyServer on %s", srv.Addr)
	}

	// Try to inherit listener from parent (hot restart via FD passing)
	addr := srv.Addr
	var ln net.Listener
	if inheritFD := os.Getenv("EASYSERVER_INHERIT_FD"); inheritFD != "" {
		if fdNum, err := strconv.Atoi(inheritFD); err == nil {
			f := os.NewFile(uintptr(fdNum), "listener")
			if f != nil {
				ln, err = net.FileListener(f)
				f.Close()
				if err == nil {
					log.Printf("restart: inherited listener from parent on %s", addr)
				}
			}
		}
	}
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("Failed to listen on %s: %v", addr, err)
		}
	}
	api.SetListener(ln)

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
