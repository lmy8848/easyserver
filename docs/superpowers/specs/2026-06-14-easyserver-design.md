# EasyServer 设计文档

> 通用 Linux 服务器管理面板

## 概述

EasyServer 是一个基于 Go + React 的 Linux 服务器管理面板，提供系统监控、服务管理、Web 终端、文件管理和腾讯云轻量应用服务器管理功能。采用单二进制嵌入式架构，部署简单。

## 技术栈

### 后端
- Go 1.21+
- Gin（HTTP 框架）
- creack/pty（PTY 伪终端）
- gorilla/websocket（WebSocket）
- embed（嵌入前端资源）
- SQLite（历史数据存储）
- tencentcloud-sdk-go（腾讯云 API）

### 前端
- React 18 + TypeScript
- Vite（构建工具）
- Ant Design（UI 组件库）
- xterm.js（Web 终端）
- ECharts（监控图表）
- Monaco Editor（代码编辑器）

## 项目结构

```
EasyServer/
├── cmd/
│   └── server/
│       └── main.go            # 入口
├── internal/
│   ├── api/                   # HTTP 路由和处理器
│   │   ├── auth.go            # 认证相关
│   │   ├── monitor.go         # 系统监控
│   │   ├── service.go         # 服务管理
│   │   ├── terminal.go        # 终端 WebSocket
│   │   ├── filemanager.go     # 文件管理
│   │   ├── user.go            # 用户管理
│   │   └── cloud.go           # 腾讯云管理
│   ├── middleware/            # 中间件（JWT、CORS、审计）
│   ├── model/                 # 数据模型
│   └── service/               # 业务逻辑
│       ├── auth.go            # 认证服务
│       ├── monitor.go         # 监控数据采集
│       ├── service.go         # systemd 交互
│       ├── terminal.go        # PTY/SSH 管理
│       ├── filemanager.go     # 文件操作
│       └── cloud.go           # 腾讯云 API 交互
├── web/                       # React 前端
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx      # 系统监控
│   │   │   ├── Services.tsx       # 服务管理
│   │   │   ├── Terminal.tsx       # 终端
│   │   │   ├── FileManager.tsx    # 文件管理
│   │   │   ├── Users.tsx          # 用户管理
│   │   │   ├── Cloud.tsx          # 腾讯云管理
│   │   │   └── Login.tsx          # 登录
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── utils/
│   │   └── App.tsx
│   ├── package.json
│   └── vite.config.ts
├── config.yaml                # 配置文件
├── go.mod
├── go.sum
└── Makefile
```

## 核心功能

### 1. 系统监控

实时采集并展示服务器状态指标。

**数据源：**
- CPU：`/proc/stat`
- 内存：`/proc/meminfo`
- 磁盘：`/proc/diskstats` + `syscall.Statfs`
- 网络：`/proc/net/dev`

**实现方式：**
- 后端每秒采集一次数据
- 通过 WebSocket 推送到前端
- SQLite 存储历史数据，保留 24 小时
- 前端使用 ECharts 展示实时折线图和历史图表

**API：**
```
GET  /api/monitor/stats       # 获取当前系统状态
GET  /api/monitor/history     # 获取历史数据（支持时间范围）
WS   /ws/monitor              # WebSocket 实时推送
```

### 2. 服务管理

通过 systemd 管理 Linux 服务。

**功能：**
- 查看所有服务列表及状态
- 启动、停止、重启服务
- 设置/取消开机自启
- 实时查看服务日志（journalctl）

**实现方式：**
- 使用 `systemctl` 命令执行操作
- 使用 `journalctl` 读取日志
- 通过 WebSocket 流式推送日志

**API：**
```
GET    /api/services                # 服务列表
GET    /api/services/:name          # 服务详情
POST   /api/services/:name/start    # 启动
POST   /api/services/:name/stop     # 停止
POST   /api/services/:name/restart  # 重启
POST   /api/services/:name/enable   # 开机自启
POST   /api/services/:name/disable  # 取消自启
GET    /api/services/:name/logs     # 日志（支持 WebSocket）
```

### 3. Web 终端

浏览器内访问服务器终端。

**功能：**
- 基于 xterm.js 的完整终端体验
- 支持多标签页
- 支持调整字体大小
- 操作审计记录

**实现方式：**
- 前端 xterm.js 通过 WebSocket 连接
- 后端使用 creack/pty 创建本地 PTY 伪终端
- 双向数据流转发
- 记录所有操作到审计日志

**API：**
```
WS /ws/terminal/:id   # WebSocket 终端连接
```

### 4. 文件管理

服务器文件的浏览和编辑。

**功能：**
- 目录浏览（表格形式 + 面包屑导航）
- 文件操作：创建、删除、重命名、移动、复制
- 内置代码编辑器（Monaco Editor）
- 文件上传（拖拽支持）和下载

**API：**
```
GET    /api/files?path=            # 列出目录
POST   /api/files/mkdir            # 创建目录
POST   /api/files/upload           # 上传文件
GET    /api/files/download?path=   # 下载文件
PUT    /api/files/rename           # 重命名
DELETE /api/files?path=            # 删除
POST   /api/files/move             # 移动
POST   /api/files/copy             # 复制
GET    /api/files/content?path=    # 读取内容
PUT    /api/files/content          # 保存内容
```

### 5. 腾讯云管理

集成腾讯云轻量应用服务器 API，管理云端资源。

**功能：**
- 实例管理：查看实例状态、开关机重启
- 监控数据：查看云端 CPU、内存、带宽等监控
- 防火墙规则：查看和管理云端防火墙规则
- 快照管理：创建快照、查看快照列表、回滚快照
- 流量监控：查看流量包使用情况

**实现方式：**
- 使用腾讯云 Go SDK (`tencentcloud-sdk-go-lighthouse`)
- 需要配置腾讯云 SecretId 和 SecretKey
- 支持查看历史监控数据（云端存储）

**API：**
```
GET    /api/cloud/instances               # 获取实例列表
GET    /api/cloud/instances/:id           # 获取实例详情
POST   /api/cloud/instances/:id/start     # 启动实例
POST   /api/cloud/instances/:id/stop      # 停止实例
POST   /api/cloud/instances/:id/restart   # 重启实例
GET    /api/cloud/monitor/:id             # 获取监控数据
GET    /api/cloud/firewall/:id            # 获取防火墙规则
POST   /api/cloud/firewall/:id            # 添加防火墙规则
DELETE /api/cloud/firewall/:id/:ruleId    # 删除防火墙规则
GET    /api/cloud/snapshots               # 获取快照列表
POST   /api/cloud/snapshots               # 创建快照
POST   /api/cloud/snapshots/:id/apply     # 回滚快照
GET    /api/cloud/traffic                 # 获取流量使用情况
```

## 认证与安全

### JWT 认证流程
1. 用户提交用户名密码
2. 后端验证密码（bcrypt 比对）
3. 验证通过生成 JWT Token（有效期 24 小时）
4. 前端存储 Token，请求时携带 `Authorization: Bearer <token>`
5. 中间件验证 Token 并解析用户信息

### 多用户与角色

| 权限 | admin | operator | viewer |
|------|-------|----------|--------|
| 查看监控 | ✓ | ✓ | ✓ |
| 服务管理 | ✓ | ✓ | ✗ |
| 终端访问 | ✓ | ✓ | ✗ |
| 文件管理 | ✓ | ✓ | ✗ |
| 用户管理 | ✓ | ✗ | ✗ |
| 腾讯云管理 | ✓ | ✗ | ✗ |

### 安全措施

- **密码存储**：bcrypt 哈希
- **登录保护**：失败 5 次锁定 15 分钟
- **IP 白名单**：可选配置
- **会话管理**：空闲 30 分钟超时，支持强制登出
- **操作审计**：记录所有 API 调用
- **HTTPS**：支持自签证书或 Let's Encrypt

## 配置文件

```yaml
server:
  port: 8080
  host: 0.0.0.0
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

auth:
  jwt_secret: "your-secret-key"
  session_timeout: 24h
  idle_timeout: 30m
  max_login_attempts: 5
  lockout_duration: 15m
  ip_whitelist: []

monitor:
  history_retention: 24h
  collect_interval: 1s

database:
  path: "./data/easyserver.db"

audit:
  enabled: true
  log_path: "./data/audit.log"

tencentcloud:
  enabled: false
  secret_id: ""
  secret_key: ""
  region: "ap-guangzhou"
  instance_id: ""  # 当前服务器的实例 ID
```

## 前端界面

### 布局
```
┌─────────────────────────────────────────────────┐
│  Logo   EasyServer                    用户名 ▼  │
├──────────┬──────────────────────────────────────┤
│          │                                      │
│ 系统监控 │         主内容区域                     │
│ 服务管理 │                                      │
│ 终端访问 │                                      │
│ 文件管理 │                                      │
│ 用户管理 │                                      │
│ 腾讯云   │                                      │
│          │                                      │
└──────────┴──────────────────────────────────────┘
```

### 页面说明

1. **系统监控页**：顶部 4 个指标卡片 + 实时折线图 + 历史图表
2. **服务管理页**：左侧服务列表 + 右侧详情/操作/日志
3. **终端访问页**：全屏终端，支持多标签
4. **文件管理页**：左侧目录树 + 右侧文件列表 + 编辑器
5. **用户管理页**：用户列表表格 + 创建/编辑弹窗（仅 admin）
6. **腾讯云管理页**：实例信息 + 监控图表 + 防火墙规则 + 快照管理

## 部署

### 构建
```bash
# 构建前端
cd web && npm run build

# 构建后端（嵌入前端资源）
cd .. && go build -o easyserver ./cmd/server
```

### 运行
```bash
./easyserver -config config.yaml
```

### 首次使用
1. 启动后访问 `http://server-ip:8080`
2. 使用默认管理员账号登录（admin/admin）
3. 系统强制要求修改默认密码
4. 开始使用

## 后续扩展

- Docker 容器管理
- 本地防火墙规则管理（iptables/nftables）
- 定时任务管理（cron）
- 系统更新管理
- 告警通知（邮件/Webhook）
- 多服务器管理（管理多台轻量服务器）
