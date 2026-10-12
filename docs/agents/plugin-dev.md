# 插件开发规范（Plugin Development）

插件是**独立进程**（Unix socket 与面板通信），完整设计见 git 历史中的
`docs/plugin-system.md`（3b33d12，未入 master）。本文档记录实现层面的硬性约定。

## 模块与构建

- 每个插件是**独立 Go module**（`plugins/<name>/` 自带 `go.mod`），`go` 指令 ≥ `1.26.0`。
- 插件不入主模块 `go test ./...` 范围；**CI 的 `Build plugins` step**（`.github/workflows/ci.yml`）
  会遍历 `plugins/*/` 逐个 `go build`，是插件的编译关卡。
- **golangci-lint 无法从主模块根目录 lint 嵌套独立模块**（`pattern ... does not contain main module`）。
  插件代码需在插件目录内跑 `golangci-lint run ./...` 验证；提交时若改动含插件文件，
  主模块钩子会因嵌套模块报错——将插件改动单独提交，并注明已单独 lint。
- 插件依赖需 `go mod tidy` 保持干净；升级依赖时跑 `govulncheck ./...` 确认无 CVE 残留。

## 权限（最小授权）

- `plugin.yaml` 的 `permissions` **只声明实际用到的能力**（如 `config.readwrite`、`log.write`、
  `audit.write`）。不要为了"可能用到"而声明 `exec.restricted`（等同 root shell）。
- 面板启用插件时管理员可**收缩已批准子集**（`POST /api/plugins/:id/approve`，不可超声明）；
  Host API 按批准子集校验，不按声明全集。

## 插件 UI（iframe 页面）

插件页面由面板 `PluginFrame` 以 iframe 承载（同源）。约定：

- **API 基址从 `location.pathname` 推导**，不要硬编码 `/api/plugins/<自己的名字>/...`：
  ```js
  function apiBase() {
    var p = window.location.pathname || '/';
    return p.charAt(p.length - 1) === '/' ? p : p + '/';
  }
  // 请求插件自身的 /api/* 路由：
  fetch(apiBase() + 'api/status', ...)
  ```
  原因：代理会剥离 `/api/plugins/<id>` 前缀；且 CF/nginx 会剥掉 iframe URL 的**尾斜杠**，
  `./api/status` 相对解析会错位到 `/api/plugins/api/status`。
- 插件 HTML 用 `//go:embed web` 打包进插件二进制，独立文件（非内联巨串）。
- 插件页面**可被 iframe 嵌入**：面板 `SecurityMiddleware` 对 `/api/plugins/*` 路径放行
  （`X-Frame-Options: SAMEORIGIN` + CSP `frame-ancestors 'self'` + 内联脚本 `'unsafe-inline'`），
  面板自身页面保持 `DENY`/`'none'`。**插件响应别手动再加 `X-Frame-Options: DENY`**。

## 日志

- **插件进程自身输出**：经 Host API `host.Log(ctx, ...)` 写
  `/var/log/easyserver/plugins/<id>.log`（面板 `GET /api/plugins/:id/logs` 查看）。
  **启动成功 / 配置更新 / 重启**都要写日志（如 `frps 已启动，控制端口 7000`），
  否则日志只剩上次退出消息、与 running 状态脱节。
- **面板侧插件管理动作**（扫描/安装/启停/重启/健康/异常）：后端插件子系统经
  `logManagerf`（`internal/plugin/manager_log.go`）双写
  `/var/log/easyserver/plugin-manager.log` + journald；前端「插件管理 → 系统日志」标签页查看
  （`GET /api/plugins/system-logs`）。

## API 响应

插件面板管理 API（`internal/plugin/routes.go`）遵循项目统一 `{code, message, data}`：
成功用 `httpx.Success`，错误用 `c.Error(errx.BadRequest/NotFound/...)` 由 ErrorHandler 渲染。
禁止裸 `c.JSON(gin.H{...})` 绕过统一格式。
