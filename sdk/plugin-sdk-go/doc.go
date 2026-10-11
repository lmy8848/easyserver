// Package sdk 是 EasyServer 插件的 Go 开发套件（文档 §16.1）。
//
// 设计目标：插件作者只需关注业务逻辑——
//   - Server：插件 HTTP 服务（Unix socket），自动提供 /health 与 /events，
//     内置中间件、JSON 辅助、优雅退出
//   - HostClient：面板 Host API 客户端（audit/config/log/event/exec/notify/storage）
//   - Logger：本地 + 面板侧双写日志
//   - Manifest：清单加载与校验辅助
//
// 快速开始：
//
//	srv := sdk.NewServer(sdk.Config{SocketPath: "my-plugin.sock"})
//	srv.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
//	    sdk.JSON(w, http.StatusOK, map[string]string{"msg": "hi"})
//	})
//	srv.OnEvent(func(ctx context.Context, e *sdk.Event) { /* 幂等处理 */ })
//	go srv.Serve()
//	hc := sdk.NewHostClient()
//	hc.Audit(ctx, "插件启动")
//	srv.Graceful() // 阻塞至 SIGTERM/SIGINT，自动关闭
package sdk

// APIVersion 是插件协议版本（与面板 manifest 校验区间对齐）。
const APIVersion = 1
