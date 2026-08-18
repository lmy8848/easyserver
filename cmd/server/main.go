package main

import (
	"flag"
	"fmt"
	"log"

	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/logger"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to config file")
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

	// 全局日志：落盘应用根目录（可配置等级/格式/轮转），错误可定位到函数与文件行。
	// 面板设置可运行时改等级并持久化到配置，重启后仍生效；打不开日志文件时降级
	// stderr（本包已保证 sink 至少含 stderr），不阻断启动。
	if closer, lerr := logger.Init(cfg.Logs); lerr != nil {
		log.Printf("WARNING: logger init failed, degrade to stderr: %v", lerr)
	} else {
		defer closer.Close()
	}

	// 装配配置仓库：此后 App / api.Setup / 各服务只认 store（实时读最新快照）
	store := config.NewStore(cfg)

	app := NewApp(store, *devMode)
	app.Run()
}
