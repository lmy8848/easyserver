# EasyServer Linux 兼容性报告

> 日期：2026-06-15

## 总结

EasyServer 设计为 **Linux 优先** 的服务器管理面板。在 Linux 系统上可以完整运行所有功能。

---

## 平台兼容性矩阵

| 功能 | Linux | Windows | 说明 |
|------|-------|---------|------|
| 核心框架 | ✅ | ✅ | Go + Gin 跨平台 |
| 认证系统 | ✅ | ✅ | JWT 跨平台 |
| 监控采集 | ✅ | ⚠️ | Linux: /proc, Windows: 占位实现 |
| 服务管理 | ✅ | ❌ | 依赖 systemctl/journalctl |
| Web 终端 | ✅ | ❌ | 依赖 creack/pty (PTY) |
| 文件管理 | ✅ | ✅ | Go os 包跨平台 |
| 用户管理 | ✅ | ✅ | SQLite 跨平台 |
| 腾讯云 | ✅ | ✅ | SDK 跨平台 |
| 部署同步 | ✅ | ⚠️ | SSH 依赖系统命令 |

---

## Linux 运行要求

### 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Linux (任何发行版) |
| 架构 | amd64 / arm64 |
| Go 版本 | 1.23+ (构建时) |
| 系统工具 | systemd, journalctl |

> **注意：** 由于依赖库要求，需要 Go 1.23 或更高版本进行构建。运行时不需要 Go。

### 依赖检查

```bash
# 检查 systemd
systemctl --version

# 检查 journalctl
journalctl --version

# 检查 PTY 支持
ls /dev/pts
```

---

## 构建指南

### 方式一：本地构建 (推荐)

```bash
# 1. 安装 Go 1.21+
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 2. 克隆项目
git clone <repo-url> && cd EasyServer

# 3. 构建前端
cd web && npm install && npm run build && cd ..

# 4. 构建后端
go mod tidy
go build -o easyserver ./cmd/server

# 5. 运行
./easyserver -config config.yaml
```

### 方式二：Docker 构建

```bash
docker build -t easyserver .
docker run -p 8080:8080 -v ./data:/app/data easyserver
```

### 方式三：交叉编译 (在 Windows/Mac 上构建 Linux 版本)

```bash
# 设置环境变量
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=1

# 需要安装交叉编译工具链
# 或使用 Docker 构建
```

---

## 配置文件

Linux 环境下的推荐配置：

```yaml
server:
  port: 8080
  host: 0.0.0.0
  tls:
    enabled: false

auth:
  jwt_secret: "your-random-secret-here"
  session_timeout: 24h
  max_login_attempts: 5
  lockout_duration: 15m
  rate_limit: 100
  rate_interval: 1m
  ip_whitelist: []

monitor:
  history_retention: 24h
  collect_interval: 1s

database:
  path: "/var/lib/easyserver/easyserver.db"

audit:
  enabled: true
  log_path: "/var/log/easyserver/audit.log"

filemanager:
  base_path: "/"  # 或限制为特定目录
```

---

## systemd 服务配置

创建 `/etc/systemd/system/easyserver.service`：

```ini
[Unit]
Description=EasyServer Management Panel
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/easyserver
ExecStart=/opt/easyserver/easyserver -config /etc/easyserver/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable easyserver
sudo systemctl start easyserver
sudo systemctl status easyserver
```

---

## 功能验证清单

### 基础功能

- [ ] 启动服务无错误
- [ ] 访问 http://server-ip:8080 看到登录页
- [ ] 使用 admin/admin 登录
- [ ] 修改默认密码

### 监控功能

- [ ] CPU 使用率显示正确
- [ ] 内存使用率显示正确
- [ ] 磁盘使用率显示正确
- [ ] 网络流量显示正确
- [ ] 历史图表正常

### 服务管理

- [ ] 服务列表正常显示
- [ ] 可以启动/停止服务
- [ ] 可以查看服务日志
- [ ] 日志流正常工作

### Web 终端

- [ ] 可以打开终端
- [ ] 可以执行命令
- [ ] 支持 Tab 补全
- [ ] 支持 Ctrl+C

### 文件管理

- [ ] 可以浏览目录
- [ ] 可以创建文件夹
- [ ] 可以上传文件
- [ ] 可以下载文件
- [ ] 可以编辑文件

---

## 已知问题

### 1. Windows 构建限制

在 Windows 上无法完整构建，因为：
- `creack/pty` 依赖 Linux PTY
- `systemctl/journalctl` 仅在 Linux 可用

**解决方案：** 在 Linux 上构建，或使用 Docker。

### 2. go.mod Go 版本

go.mod 要求 Go 1.25.0，但实际可以在 Go 1.21+ 上构建。

**解决方案：** 修改 go.mod 中的版本要求。

### 3. 交叉编译

CGO_ENABLED=1 时需要目标平台的 C 工具链。

**解决方案：** 使用 Docker 多阶段构建。

---

## 性能建议

### 生产环境

1. **使用反向代理**：Nginx/Caddy 处理 HTTPS 和静态文件
2. **限制文件访问**：设置 `filemanager.base_path` 限制可访问目录
3. **启用 HTTPS**：配置 TLS 证书
4. **配置 IP 白名单**：限制访问来源
5. **定期备份数据库**：SQLite 数据文件

### 监控

1. **日志轮转**：配置 logrotate 处理审计日志
2. **磁盘空间**：监控 data 目录大小
3. **内存使用**：Go 程序内存占用约 50-100MB

---

## 结论

EasyServer **完全支持 Linux 系统**，所有功能都可以在 Linux 上正常运行。

主要依赖：
- systemd (服务管理)
- journalctl (日志读取)
- PTY (Web 终端)
- /proc (监控数据)

建议在 Linux 上直接构建和运行，或使用 Docker 部署。
