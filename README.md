# EasyServer

**一站式 Linux 服务器管理面板** — Go + React 构建，轻量、安全、功能全面。

[English](README_EN.md) · [文档网站](https://lmy8848.github.io/easyserver/) · [API 文档](docs/api-reference.md) · [部署指南](docs/linux-deploy.md)

![EasyServer Dashboard](docs/screenshot.jpg)

---

## 功能特性

- **系统监控** — CPU / 内存 / 磁盘 / 网络实时监控，历史图表
- **进程管理** — systemd 服务管理，进程守护与自动重启
- **Web Terminal** — 浏览器终端，基于 PTY 的真实 Shell
- **文件管理** — 在线浏览 / 编辑 / 上传 / 下载 / 压缩解压
- **Web 服务器** — Nginx / Apache 安装、配置、站点管理
- **数据库** — MySQL / PostgreSQL / Redis 多版本管理
- **容器管理** — Docker / Compose / 镜像 / 存储卷 / 网络
- **防火墙** — iptables / nftables 规则管理
- **运行环境** — Node.js / Python / Go / Java 运行时管理（via mise）
- **计划任务** — Cron 任务管理，支持脚本和文档
- **远程部署** — SSH 远程服务器管理，一键部署
- **通知告警** — Webhook 通知（钉钉 / 飞书 / 企业微信）+ 告警规则
- **审计日志** — 完整操作审计，支持导出和完整性验证
- **2FA 认证** — TOTP 双因素认证，支持备用码
- **扫码登录** — 手机端扫码登录（QR Login）
- **安全扫描** — CVE 漏洞扫描（osv.dev 查询 + 一键升级）
- **文件外链** — 安全的文件分享链接，支持密码/过期控制
- **端口监控** — 实时监听端口查看

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

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25 + Gin + SQLite (WAL) + WebSocket + JWT |
| 前端 | React 19 + TypeScript + Ant Design 6 + Vite 8 |
| 部署 | Docker 多阶段构建 + systemd |

---

## 文档

| 文档 | 说明 |
|------|------|
| [文档网站](https://lmy8848.github.io/easyserver/) | 完整使用文档 |
| [API 文档](docs/api-reference.md) | 完整接口文档 |
| [Linux 部署手册](docs/linux-deploy.md) | 二进制部署 + systemd + Nginx |

---

## 系统要求

| 项目 | 最低要求 | 推荐配置 |
|------|----------|----------|
| 操作系统 | Linux x86_64 | Ubuntu 22.04+ / Debian 12+ |
| 内存 | 512MB | 1GB+ |
| 磁盘 | 1GB | 5GB+ |
| 端口 | 8080 | 可配置 |

---

## 开发

```bash
# 后端（开发模式，需 air）
make dev

# 或手动
go build -tags dev -o easyserver ./cmd/server
./easyserver -config config.yaml -dev

# 前端（热更新）
cd web
pnpm install
pnpm dev
# 访问 http://localhost:5173
```

---

## 安全建议

1. **必须修改** `jwt_secret` 和 `encryption_key`，使用 `openssl rand -base64 32` 生成
2. **生产环境** 启用 HTTPS（通过 Nginx 反向代理或直接配置）
3. **配置 IP 白名单** 限制管理面板访问来源
4. **定期备份** 数据库和配置文件

---

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送分支 (`git push origin feature/amazing-feature`)
5. 提交 Pull Request
