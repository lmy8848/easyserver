package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"easyserver/internal/infra/config"
)

// healthcheckCmd 探测本地 /health 端点判断服务是否正常启动。
// 用于手动探活与排错（监控脚本、运维检查等）：退出码 0 表示服务健康。
// 监听地址为通配（0.0.0.0 / ::）时拨 127.0.0.1。
func healthcheckCmd(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: load config: %v\n", err)
		os.Exit(1)
	}
	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s:%d/health", host, cfg.Server.Port)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %s: %v\n", url, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: %s: HTTP %d\n", url, resp.StatusCode)
		os.Exit(1)
	}
	fmt.Printf("ok: %s\n", url)
}
