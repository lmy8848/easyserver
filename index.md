---
layout: default
title: 首页
nav_order: 1
permalink: /
---

# EasyServer

**一站式 Linux 服务器管理面板** — Go + React 构建，轻量、安全、功能全面。

[快速开始](#快速开始) · [功能特性](#功能特性) · [部署指南](#部署指南) · [API 文档](api-reference.md)

---

## 快速开始

### Docker 部署（推荐）

```bash
docker run -d \
  --name easyserver \
  -p 8080:8080 \
  -v /opt/easyserver/data:/app/data \
  -e EASYSERVER_JWT_SECRET="$(openssl rand -base64 32)" \
  -e EASYSERVER_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  lmy8848/easyserver:latest
```

访问 `http://your-server:8080`，首次启动自动生成管理员账号（密码显示在控制台）。

### 二进制部署

```bash
# 下载最新版本
wget https://github.com/lmy8848/easyserver/releases/latest/download/easyserver-linux-amd64
chmod +x easyserver-linux-amd64

# 生成配置
cat > config.yaml << 'EOF'
server:
  port: 8080
  host: 0.0.0.0
  serve_frontend: true
auth:
  jwt_secret: "your-random-secret-32-bytes!!"
database:
  path: "./data/easyserver.db"
filemanager:
  base_path: "/opt/easyserver/data"
EOF

# 启动
./easyserver-linux-amd64 -config config.yaml
```

---

## 功能特性

### 核心管理

| 功能 | 说明 |
|------|------|
| **系统监控** | CPU / 内存 / 磁盘 / 网络实时监控，支持历史图表 |
| **进程管理** | systemd 服务管理，进程守护与自动重启 |
| **终端** | 浏览器 Web Terminal，基于 PTY 的真实 Shell |
| **文件管理** | 在线文件浏览 / 编辑 / 上传 / 下载 / 压缩解压 |

### 服务管理

| 功能 | 说明 |
|------|------|
| **Web 服务器** | Nginx / Apache 安装、配置、站点管理 |
| **数据库** | MySQL / PostgreSQL / Redis 多版本安装与管理 |
| **容器** | Docker / Docker Compose / 镜像 / 存储卷 / 网络 |
| **防火墙** | iptables / nftables 规则管理，支持模板和批量操作 |

### 运维工具

| 功能 | 说明 |
|------|------|
| **运行环境** | Node.js / Python / Go / Java 等运行时管理（via mise） |
| **计划任务** | Cron 任务管理，支持脚本和文档 |
| **远程部署** | SSH 远程服务器管理，一键部署 |
| **通知** | Webhook 通知（钉钉 / 飞书 / 企业微信）+ 告警规则 |

### 安全特性

| 功能 | 说明 |
|------|------|
| **2FA 认证** | TOTP 双因素认证，支持备用码 |
| **扫码登录** | 手机端扫码登录（QR Login） |
| **安全扫描** | CVE 漏洞扫描（osv.dev 查询 + 一键升级） |
| **JWT + Session** | 安全的令牌管理，支持会话踢出 |
| **IP 白名单** | 按用户配置 IP 访问限制 |
| **密码策略** | 强制密码复杂度，登录失败锁定 |
| **CSP + HSTS** | 完整的安全响应头 |
| **审计追踪** | 所有操作记录审计日志 |

### 其他

| 功能 | 说明 |
|------|------|
| **文件外链** | 安全的文件分享链接，支持密码/过期/下载次数控制 |
| **端口监控** | 实时监听端口查看，含进程信息 |
| **腾讯云** | 腾讯云轻量应用服务器集成（实例、防火墙、快照、监控） |

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Gin + SQLite (WAL) + WebSocket + JWT |
| 前端 | React 19 + TypeScript + Ant Design 6 + Vite 8 |
| 部署 | Docker 多阶段构建 + systemd |
| 数据库 | SQLite (WAL 模式) |

---

## 部署指南

详细部署文档请参考：

- [Linux 部署手册](linux-deploy.md) — 二进制部署 + systemd + Nginx 反向代理
- [Docker 部署](#docker-部署推荐) — 容器化部署
- [API 接口文档](api-reference.md) — 完整 API 文档
- [开发指南](development.md) — 项目结构、Make 命令、开发流程

### 系统要求

| 项目 | 最低要求 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Linux x86_64 | Ubuntu 22.04+ / Debian 12+ |
| 内存 | 512MB | 1GB+ |
| 磁盘 | 1GB | 5GB+ |
| 端口 | 8080 | 可配置 |

### Docker Compose 部署

```yaml
version: '3.8'
services:
  easyserver:
    image: lmy8848/easyserver:latest
    container_name: easyserver
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
      - ./config.yaml:/app/config.yaml:ro
    environment:
      - EASYSERVER_JWT_SECRET=${JWT_SECRET}
      - EASYSERVER_ENCRYPTION_KEY=${ENCRYPTION_KEY}
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO", "/dev/null", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      retries: 3
```

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `EASYSERVER_JWT_SECRET` | JWT 密钥（≥32字节） | 必填 |
| `EASYSERVER_ENCRYPTION_KEY` | 部署加密密钥（≥32字节） | 必填 |
| `EASYSERVER_PORT` | 监听端口 | 8080 |
| `EASYSERVER_HOST` | 监听地址 | 0.0.0.0 |
| `EASYSERVER_DB_PATH` | 数据库路径 | ./data/easyserver.db |
| `EASYSERVER_TLS_ENABLED` | 启用 HTTPS | false |

---

## 配置文件

```yaml
server:
  port: 8080
  host: 0.0.0.0
  serve_frontend: true
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

auth:
  jwt_secret: "your-random-secret"       # 必须修改！
  session_timeout: 24h
  idle_timeout: 30m
  max_login_attempts: 5
  lockout_duration: 15m
  rate_limit: 1000
  rate_interval: 1m

monitor:
  history_retention: 168h
  collect_interval: 3s

database:
  path: "./data/easyserver.db"

audit:
  enabled: true
  retention_days: 90

filemanager:
  base_path: "/opt/easyserver/data"      # 文件管理根目录

deploy:
  encryption_key: "your-32-byte-key"     # 必须修改！
```

---

## 安全建议

1. **必须修改** `jwt_secret` 和 `encryption_key`，使用 `openssl rand -base64 32` 生成
2. **生产环境** 启用 HTTPS（通过 Nginx 反向代理或直接配置）
3. **配置 IP 白名单** 限制管理面板访问来源
4. **定期备份** 数据库和配置文件
5. **启用审计日志** 记录所有操作
6. **启用 2FA** 增强登录安全
7. **配置 Cloudflare Turnstile** 防暴力破解

---

## 常用命令

```bash
# Docker
docker logs -f easyserver           # 查看日志
docker restart easyserver           # 重启
docker exec -it easyserver bash     # 进入容器

# Systemd
sudo systemctl status easyserver    # 查看状态
sudo journalctl -u easyserver -f    # 实时日志
sudo systemctl restart easyserver   # 重启
```

---

## API 文档

完整的 API 接口文档请参考 [api-reference.md](api-reference.md)。

---

## 许可证

MIT License

---

## 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送分支 (`git push origin feature/amazing-feature`)
5. 提交 Pull Request
