package main

import (
	"flag"
	"fmt"
	"log"

	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	devMode := flag.Bool("dev", false, "run in development mode (no embed, API only)")
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(infra.GetFullVersionString())
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
	if err := config.Validate(cfg, *devMode); err != nil {
		log.Fatalf("config: %v", err)
	}

	// 装配配置仓库：此后 App / api.Setup / 各服务只认 store（实时读最新快照），
	// cfg 对象不再往下传。
	store := config.NewStore(cfg)

	app := NewApp(store, *devMode)
	app.Run()
}
