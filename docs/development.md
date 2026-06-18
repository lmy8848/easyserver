# EasyServer 开发指南

## 环境要求

| 工具 | 版本要求 |
|------|----------|
| Go | 1.21+ |
| Node.js | 18+ |
| pnpm | 8+ (推荐) / npm |
| Make | 可选 |

## 快速开始

### 1. 克隆仓库

```bash
git clone <repo-url> && cd EasyServer
```

### 2. 后端开发

```bash
# 安装依赖
go mod download

# 使用内置前端资源运行（需先构建前端）
go run ./cmd/server -config config.yaml

# 仅启动后端 API（前端由 Vite dev server 提供）
go run ./cmd/server -config config.yaml -dev
```

`-dev` 模式下后端禁用 `embed`，监听 `8080` 端口，前端访问 `5173` 端口。

### 3. 前端开发

```bash
cd web
pnpm install
pnpm run dev    # 启动 Vite 开发服务器（端口 5173，代理 API 到 8080）
```

`vite.config.ts` 已配置代理规则，`/api/*` 和 `/ws/*` 自动转发到后端。

### 4. 构建生产版本

```bash
# 一步构建（前端构建 + 后端嵌入 + 编译）
make build

# 或分步执行
cd web && pnpm run build
cd .. && go build -o easyserver ./cmd/server
```

产物为单二进制 `easyserver`，直接部署。

### 5. Docker 构建

```dockerfile
# 多阶段构建示例
FROM node:18 AS frontend
WORKDIR /web
COPY web/ .
RUN pnpm install && pnpm run build

FROM golang:1.21 AS backend
WORKDIR /app
COPY . .
COPY --from=frontend /web/dist ./web/dist
RUN go build -o easyserver ./cmd/server

FROM debian:bookworm-slim
COPY --from=backend /app/easyserver /usr/local/bin/
COPY config.yaml /etc/easyserver/
EXPOSE 8080
ENTRYPOINT ["easyserver", "-config", "/etc/easyserver/config.yaml"]
```

## 项目结构说明

```
EasyServer/
├── cmd/server/main.go       # 入口，解析命令行参数，启动 HTTP 服务
├── internal/
│   ├── api/                 # HTTP 路由注册 + 请求处理
│   │   ├── router.go        # 统一路由注册
│   │   ├── auth.go
│   │   ├── monitor.go
│   │   ├── service.go
│   │   ├── terminal.go
│   │   ├── filemanager.go
│   │   ├── user.go
│   │   └── cloud.go
│   ├── middleware/           # Gin 中间件
│   │   ├── jwt.go           # JWT 验证
│   │   ├── cors.go          # CORS 配置
│   │   ├── audit.go         # 操作审计
│   │   ├── ratelimit.go     # 速率限制
│   │   └── recovery.go      # panic 恢复
│   ├── model/                # 数据模型 + 数据库操作
│   │   ├── user.go
│   │   ├── audit.go
│   │   └── monitor.go
│   └── service/              # 业务逻辑层
│       ├── auth.go
│       ├── monitor.go
│       ├── service.go
│       ├── terminal.go
│       ├── filemanager.go
│       └── cloud.go
├── web/                      # React 前端
│   ├── src/
│   │   ├── pages/            # 页面组件
│   │   ├── components/       # 通用 UI 组件
│   │   ├── hooks/            # 自定义 Hooks
│   │   ├── utils/            # 工具函数
│   │   ├── services/         # API 调用封装
│   │   ├── store/            # 全局状态（Zustand）
│   │   ├── types/            # TypeScript 类型定义
│   │   └── App.tsx           # 路由配置
│   ├── package.json
│   └── vite.config.ts
├── config.yaml               # 默认配置文件
└── Makefile                  # 构建任务
```

## 代码规范

### Go

- 遵循 [Go 官方编码规范](https://go.dev/doc/effective_go)
- 错误处理优先，避免 panic
- 所有导出函数、类型、常量必须有 godoc 注释
- 使用 `internal/` 隔离外部引用

### TypeScript / React

- 使用 TypeScript 严格模式
- 组件使用函数式组件 + Hooks
- 文件名采用 PascalCase（组件）和 camelCase（工具/服务）
- CSS Modules 或 Ant Design Token 系统，避免全局样式污染

### 提交信息

参考 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 新功能
fix: 修复
docs: 文档变更
refactor: 重构
perf: 性能优化
test: 测试
chore: 构建/工具变更
```

## 调试技巧

### 后端实时重载

推荐使用 [`air`](https://github.com/air-verse/air)：

```bash
# 安装 air
go install github.com/air-verse/air@latest

# 运行（自动监听文件变更重启）
air
```

### 前端 Mock 数据

在 `web/src/services/` 下添加 mock 文件，开发时通过环境变量切换：

```typescript
const baseURL = import.meta.env.DEV ? '/api' : '/api';
```

### SQLite 调试

```bash
# 使用 sqlite3 CLI 查看数据
sqlite3 ./data/easyserver.db

# 导出审计日志
sqlite3 ./data/easyserver.db "SELECT * FROM audit_logs;" > audit_export.csv
```

## 常见问题

**Q: 前端请求报 CORS 错误？**
A: 确保后端添加了 `-dev` 参数或前端 vite.config.ts 中 proxy 配置正确。

**Q: WebSocket 连接失败？**
A: 检查后端是否正常运行，前端 `VITE_WS_URL` 环境变量是否指向正确的后端地址。

**Q: 文件上传大文件超时？**
A: 检查 `config.yaml` 中的 `server.write_timeout` 和 `server.read_timeout` 配置。
