# EasyServer 开发指南

## 环境要求

| 工具 | 版本要求 |
|------|----------|
| Go | 1.25+ |
| Node.js | 22+ |
| pnpm | 11+ |
| Make | 可选 |

> 后端仅面向 Linux（使用 Linux 专属系统调用），完整编译在 Linux / 服务器上进行；
> 前端构建可在任意平台完成。Windows 上可用 `GOOS=linux go build ./...` 做纯编译校验（不产出部署二进制）。

## 快速开始

### 1. 克隆仓库

```bash
git clone <repo-url> && cd EasyServer
```

### 2. 后端开发

```bash
# 安装依赖
go mod download

# 使用内置前端资源运行（需先构建前端，见第 4 步）
cp config.toml.example config.toml   # 首次生成配置
go run ./cmd/server -config config.toml

# 仅启动后端 API（前端由 Vite dev server 提供，端口 5173）
go run ./cmd/server -config config.toml -dev
```

- `-dev` 模式禁用内嵌前端，仅监听后端（默认 `8080`），API 供 Vite 前端代理调用。
- **生产环境启动不要加 `-dev`**——否则前端不会被内嵌服务。

### 3. 前端开发

```bash
cd web
pnpm install
pnpm dev      # 启动 Vite（端口 5173），/api/* 与 /ws/* 自动代理到后端 8080
```

### 4. 构建生产版本

```bash
# 一步构建（前端构建 + 嵌入 + 后端编译，带版本号）
make build

# 或分步执行（注意版本号注入 ldflags）
cd web && pnpm build
rm -rf internal/api/web/dist && mkdir -p internal/api/web/dist && cp -a web/dist/. internal/api/web/dist/
VERSION=$(git describe --tags --always)
go build -buildvcs=false -ldflags "-X easyserver/internal/infra.Version=$VERSION" -o easyserver ./cmd/server
```

产物为单二进制 `easyserver`，直接部署（systemd / nohup）。

## 配置 (config.toml)

配置为 **TOML** 格式，示例见 [`config.toml.example`](../config.toml.example)：

- `[server]` — 端口、监听地址、TLS、允许来源、限流、上传上限
- `[auth]` — JWT 密钥、会话/登录限流、IP 白名单、多端登录
- `[monitor]` / `[audit]` — 监控保留期、审计保留天数
- `[filemanager]` — 文件访问根路径（默认 `/opt/easyserver/data`）
- `[deploy]` — 加密密钥（用于 SSH 凭据加密）
- `[notify]` / `[alerts]` — Webhook 通知与告警规则
- `[logs]` — **全局运行日志**：`level`（默认 info，面板可运行时改并持久化）、`format`（text/json）、`max_size_mb`（轮转阈值）、`max_files`（保留份数）

> 数据库路径**固定**为 `/opt/easyserver/data/easyserver.db`（代码常量 `config.DataRoot` 派生），不在配置中。

## 全局运行日志

运行日志持久化到应用根目录 `easyserver.log`（默认），带等级过滤、源码定位（`函数@文件:行`）、按大小轮转：

```bash
tail -f /opt/easyserver/easyserver.log       # 实时跟踪
grep "level=ERROR" easyserver.log            # 只看错误
ls easyserver.log.*                          # 轮转历史文件
```

new code uses the `internal/infra/logger` package:

```go
import "easyserver/internal/infra/logger"

logger.Info("server started", "port", 8080)
logger.Error("operation failed", "err", errx.SafeError(err))
logger.With("module", "auth").Warn("rate limited", "ip", ip)
```

- 现有大量 `log.Printf` 通过 stdlib 桥接按 Info 透传（保留真实文件行号），随改动逐步迁移到分级调用。
- 架构可扩展：`logger.AddTarget`（面板流式/远程聚合）、`logger.AddHook`（错误告警/转发）、`format="json"`。

## 项目结构说明

```
EasyServer/
├── cmd/server/main.go       # 入口：解析参数、加载配置、初始化全局日志、启动 HTTP 服务
├── cmd/cli/                 # CLI 工具（reset-password/unlock/reset-totp/show-admin）
├── internal/
│   ├── api/                 # API 编排层：路由注册 + 内嵌前端资源(embed)
│   │   ├── router.go        # 统一路由注册
│   │   └── web/dist/        # 前端构建产物（嵌入用，构建时生成）
│   ├── auth/                # 认证领域（用户、JWT、TOTP、Session、扫码登录）
│   ├── audit/               # 审计日志领域（落库 + 批量写）
│   ├── cloud/               # 腾讯云集成
│   ├── container/           # Docker 容器管理
│   ├── cron/                # 计划任务
│   ├── database/            # 数据库管理（MySQL/PostgreSQL/Redis）
│   ├── deploy/              # 远程部署（SSH/SFTP）
│   ├── envconfig/           # 环境变量配置
│   ├── filemanager/         # 文件管理
│   ├── firewall/            # 防火墙
│   ├── httpx/               # HTTP 工具层：响应封装、WebSocket、中间件
│   │   └── middleware/      # 中间件（audit/cors/jwt/ratelimit/security/ipwhitelist/session/ws_auth/domain/timeout/error）
│   ├── infra/               # 基础设施层
│   │   ├── config/          # TOML 配置 + 原子快照仓库(Store)
│   │   ├── database/        # SQLite 初始化 + 迁移
│   │   ├── errx/            # 语义错误体系（脱敏 SafeError）
│   │   ├── logger/          # 全局运行日志（落盘/分级/定位/高可用/可扩展）
│   │   ├── mise/            # 运行环境管理器（mise）引导
│   │   ├── task/            # 后台任务执行器
│   │   └── version.go       # 构建版本号（ldflags 注入）
│   ├── monitor/             # 系统监控
│   ├── notification/ + notify/ # 通知服务（Webhook：钉钉/飞书/企微）
│   ├── runtimeenv/          # 运行环境管理（Node/Python/Go/Java）
│   ├── security/            # 安全运维（SSH 加固/CVE/登录防护/FIM）
│   ├── settings/            # 面板设置 + 设置 API
│   ├── ssh/                 # SSH 管理
│   ├── systemd/             # systemd 服务管理
│   ├── terminal/            # Web 终端（PTY）
│   └── web/                 # Web 服务器/站点管理
├── web/                     # React 前端
│   └── src/{pages,components,hooks,utils,services,store,types}
├── migrations/              # SQLite 迁移（000001-…）
├── deploy/                  # 部署脚本（install.sh、linux-deploy.md）
├── config.toml.example      # 配置示例
├── LICENSE                  # MIT 协议
├── Makefile                 # 构建任务
└── README.md                # 英文默认文档 / README_ZH.md 中文文档
```

## Make 命令

| 命令 | 说明 |
|------|------|
| `make build` | 构建生产二进制（前端构建 + 内嵌 + 带版本号） |
| `make build-web` | 仅构建前端并复制到嵌入目录 |
| `make build-linux` / `build-linux-arm64` | 交叉编译 Linux |
| `make dev` | 开发模式（air + vite 并行） |
| `make deps` | 安装依赖（go mod + pnpm） |
| `make clean` | 清理构建产物 |
| `make release` | 创建发布 tag |

> 代码提交前请自行跑通：后端 `gofmt -l .` + `go vet ./...` + `go test ./...`（Linux 环境/服务器），前端 `cd web && pnpm build`（tsc + vite）。

## 代码规范

### Go

- 遵循 [Go 官方编码规范](https://go.dev/doc/effective_go)
- 错误处理优先，避免 panic（goroutine 用 `infra/safego` 保护）
- 新功能走领域分层：`http/handler.go`、`service.go`、`repo.go`、`model.go`，数据访问走 Repository
- 运行日志用 `internal/infra/logger` 分级调用，敏感信息用 `errx.SafeError` 脱敏
- 避免硬编码：路径/常量统一从 `config.DataRoot` / 配置节派生

### TypeScript / React

- TypeScript 严格模式，函数式组件 + Hooks
- 页面组件 PascalCase / 工具函数 camelCase
- 改动后运行 `cd web && pnpm build` 检查类型错误

### 提交信息

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
type(scope): 中文描述

type: feat/fix/refactor/perf/test/docs/style/chore
scope: 业务领域（如 auth、monitor、container）
```

## 调试技巧

### 后端实时重载

```bash
go install github.com/air-verse/air@latest
make dev
```

### 运行日志排障

```bash
tail -f /opt/easyserver/easyserver.log        # 应用运行日志
journalctl -u easyserver -f                   # systemd 部署时
# 日志含 函数@文件:行 / 文件:行，可直接定位到出错代码
```

### SQLite 调试

```bash
sqlite3 ./data/easyserver.db "SELECT * FROM audit_logs ORDER BY id DESC LIMIT 20;"
```

## 常见问题

**Q: 前端请求报 CORS 错误？**
A: 开发时后端加 `-dev` 参数自动放行 localhost 来源；生产环境在 `[server] allowed_origins` 配置允许来源。

**Q: WebSocket 连接失败？**
A: 检查后端是否正常运行、`allowed_origins` 是否包含前端来源；Token 通过 `Sec-WebSocket-Protocol` 头传递。

**Q: 文件上传大文件超时？**
A: 上传大小受 `server.max_upload_size` 限制（默认 512MB），超时由 `httpx/middleware/timeout` 控制（文件路由 10 分钟）。

**Q: 启动后没日志文件？**
A: 检查 `config.toml` 是否有 `[logs]` 段（默认落盘应用根目录 `easyserver.log`）；日志文件打不开时降级 stderr 不影响运行，去 `/dev/stderr` 或 nohup 输出查看。