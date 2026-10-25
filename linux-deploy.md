---
layout: default
title: 部署指南
nav_order: 3
permalink: /deploy
---

# EasyServer Linux 部署手册

> 版本：v0.2.x | 更新日期：2026-10-25

---

## 1. 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Linux x86_64 / arm64（Debian 12+、Ubuntu 22.04+ 等） |
| 内存 | ≥ 512MB |
| 磁盘 | ≥ 1GB |
| 端口 | 8080（可配置；建议经反向代理或内置 TLS 对外） |
| 依赖 | systemd（用于服务托管）；无其他运行时依赖（单二进制，前端已内嵌） |

---

## 2. 快速部署（推荐：一键安装）

```bash
curl -fsSL https://raw.githubusercontent.com/lmy8848/easyserver/master/scripts/install.sh | bash
```

脚本自动完成：

1. 从 GitHub Releases 下载最新 `easyserver-linux-amd64` 并校验 sha256
2. 安装到 `/usr/local/bin/easyserver`
3. 创建 `/opt/easyserver/config.toml`（已存在则跳过，升级安全）
4. 注册并启动 systemd 服务（`systemctl enable --now easyserver`）
5. 打印访问地址与**首次启动自动生成的随机管理员账号**（请立即登录修改密码）

安装指定版本：`bash install.sh v0.2.0`。

---

## 3. 手动部署

### 3.1 下载与安装

```bash
# 从 Releases 下载（amd64 / arm64 均有提供，附 checksums-sha256.txt）
wget https://github.com/lmy8848/easyserver/releases/latest/download/easyserver-linux-amd64
sha256sum -c <(grep easyserver-linux-amd64 checksums-sha256.txt)   # 可选校验
install -m 755 easyserver-linux-amd64 /usr/local/bin/easyserver

mkdir -p /opt/easyserver/data
```

### 3.2 配置（config.toml，TOML 格式）

```bash
cat > /opt/easyserver/config.toml <<'EOF'
[server]
port = 8080
host = "0.0.0.0"
domain = ""            # 对外域名（可选；设置后配合 force_domain 强制跳转）
force_domain = false
trusted_proxies = ["127.0.0.1"]  # 前置反代时填其 CIDR（如 Nginx/Cloudflare）

[server.tls]
enabled = false        # 内置 TLS（也可用 Nginx/Caddy 反代终止 TLS）
cert_file = ""
key_file = ""

[auth]
# jwt_secret 留空时首次启动自动生成并持久化；也可用环境变量 EASYSERVER_JWT_SECRET 指定
session_timeout = "24h"
ip_whitelist = []      # 空 = 允许所有来源

[filemanager]
base_path = "/opt/easyserver/data"
EOF
```

完整字段见仓库 [config.toml.example](https://github.com/lmy8848/easyserver/blob/master/config.toml.example)。

### 3.3 测试运行

```bash
cd /opt/easyserver && easyserver -config config.toml
curl http://127.0.0.1:8080/health
```

首次启动日志会打印随机管理员账号（`journalctl` 或控制台查看）。

---

## 4. systemd 服务

`/etc/systemd/system/easyserver.service`（与官方安装脚本一致）：

```ini
[Unit]
Description=EasyServer Management Panel
Documentation=https://github.com/lmy8848/easyserver
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
# 以 root 运行为系统管理功能（服务控制/防火墙/包管理）所必需；
# 高安全环境可改为专用用户并配置相应 sudo 权限
WorkingDirectory=/opt/easyserver
ExecStart=/usr/local/bin/easyserver -config /opt/easyserver/config.toml
Restart=always
RestartSec=5
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload && systemctl enable --now easyserver
systemctl status easyserver
journalctl -u easyserver -f          # 实时日志
```

---

## 5. 目录结构

```
/opt/easyserver/
├── config.toml            # 配置（TOML）
├── easyserver.log         # 全局运行日志（自动轮转，默认 10MB×3）
├── data/
│   ├── easyserver.db      # SQLite（业务数据 + 审计 + 插件注册）
│   └── plugin-storage/    # 插件私有文件区
└── plugins/               # 插件目录（每插件一个子目录，含 plugin.yaml）

/run/easyserver/           # 运行时 socket（tmpfs，重启即清）
/var/log/easyserver/       # 插件子系统日志
    ├── plugin-manager.log # 插件管理动作（安装/启停/审批/卸载）
    └── plugins/<id>.log   # 各插件自身日志
```

---

## 6. 升级

```bash
# 方式一：重跑安装脚本（配置与数据保留，自动重启服务）
curl -fsSL https://raw.githubusercontent.com/lmy8848/easyserver/master/scripts/install.sh | bash

# 方式二：手动替换二进制
install -m 755 easyserver-linux-amd64 /usr/local/bin/easyserver
systemctl restart easyserver
```

- 数据库 schema 迁移内嵌于二进制，启动时自动执行（升级只需换二进制）
- 插件升级在面板「插件管理 → 导入 / 升级插件」上传同名包即可（保留配置与权限）

---

## 7. 常见问题

| 问题 | 处理 |
|------|------|
| 忘记管理员密码 | 见启动日志首次生成的账号；或参考 README 的密码重置说明 |
| 端口被占用 | 修改 `config.toml` 的 `server.port` 后 `systemctl restart easyserver` |
| 前置 Nginx/Cloudflare | 配置 `trusted_proxies` 为代理 CIDR，否则客户端 IP 显示为代理地址 |
| 插件启动失败 | 面板「插件管理 → 系统日志」查看动作日志；单插件日志见 `/var/log/easyserver/plugins/` |
| 运行日志排查 | `/opt/easyserver/easyserver.log`（文件）或 `journalctl -u easyserver`（控制台） |
