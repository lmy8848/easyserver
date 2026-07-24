---
layout: default
title: 部署指南
nav_order: 3
permalink: /deploy
---

# EasyServer Linux 部署手册

> 版本：v0.1.0-alpha | 更新日期：2026-07-23

---

## 1. 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Linux x86_64 (Ubuntu 22.04+, Debian 12+, CentOS 7+) |
| 内存 | ≥ 512MB |
| 磁盘 | ≥ 1GB |
| 端口 | 8080 (可配置) |
| 依赖 | systemd, journalctl |

---

## 2. 快速部署

### 2.1 上传文件

```bash
# 上传到服务器
scp easyserver-linux-amd64 user@server:/opt/easyserver/easyserver
scp config.yaml user@server:/opt/easyserver/
```

### 2.2 配置

```bash
# 登录服务器
ssh user@server

# 进入目录
cd /opt/easyserver

# 设置权限
chmod +x easyserver

# 编辑配置
vi config.yaml
```

**必须修改的配置：**

```yaml
auth:
  jwt_secret: "your-random-secret-here"  # 修改为随机字符串！
```

生成随机密钥：

```bash
openssl rand -base64 32
```

### 2.3 启动

```bash
# 测试运行
./easyserver -config config.yaml

# 访问测试
curl http://server-ip:8080/health
```

---

## 3. systemd 服务配置

### 3.1 创建服务文件

```bash
sudo vi /etc/systemd/system/easyserver.service
```

**服务文件内容：**

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
WorkingDirectory=/opt/easyserver
ExecStart=/opt/easyserver/easyserver -config /opt/easyserver/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30

# 安全限制
NoNewPrivileges=false
ProtectSystem=false
ProtectHome=false

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

# 环境变量
Environment=GOMAXPROCS=2

[Install]
WantedBy=multi-user.target
```

### 3.2 启动服务

```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start easyserver

# 查看状态
sudo systemctl status easyserver

# 设置开机自启
sudo systemctl enable easyserver

# 查看日志
sudo journalctl -u easyserver -f
```

---

## 4. 目录结构

```
/opt/easyserver/
├── easyserver          # 主程序
├── config.yaml         # 配置文件
└── data/               # 数据目录 (自动创建)
    ├── easyserver.db   # SQLite 数据库
    └── audit.log       # 审计日志
```

---

## 5. 配置说明

### 5.1 完整配置示例

```yaml
server:
  port: 8080                    # 监听端口
  host: 0.0.0.0                 # 监听地址
  serve_frontend: true           # 是否提供前端页面
  dev_mode: false               # 开发模式（生产环境设为 false）
  allowed_origins:              # 允许的 WebSocket Origin
    - "https://yourdomain.com"
  trusted_proxies:              # 可信反向代理 CIDR（默认 127.0.0.1）
    - "127.0.0.1"
  assets_rate_limit: 5000       # 静态资源每分钟请求限制
  assets_rate_interval: 1m
  max_upload_size: 536870912    # 上传大小限制（bytes, 0=默认 512MB）
  tls:
    enabled: false              # 是否启用 HTTPS
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"
  turnstile:
    site_key: ""                # Cloudflare Turnstile Site Key
    secret_key: ""              # Cloudflare Turnstile Secret Key
    enable_login: false
    enable_qr_login: false
    enable_public_share: false

auth:
  jwt_secret: "your-random-secret-32-bytes!!"  # JWT 密钥（必须修改!）
  session_timeout: 24h              # 会话超时
  idle_timeout: 30m                 # 空闲超时
  max_login_attempts: 5             # 最大登录尝试次数
  lockout_duration: 15m             # 锁定时长
  rate_limit: 1000                  # API 速率限制
  rate_interval: 1m                 # 速率限制间隔
  login_rate_limit: 10              # 登录接口速率限制
  login_rate_interval: 1m
  ip_whitelist: []                  # IP 白名单（空=允许所有）
  session_cleanup_interval: 5m      # 过期会话清理间隔
  allow_multi_session: false        # 允许多设备同时登录
  mobile_device_binding: true       # 移动设备绑定（仅 AllowMultiSession 开启时生效）

monitor:
  history_retention: 168h        # 历史数据保留时长（7天）
  collect_interval: 3s           # 采集间隔

database:
  path: "./data/easyserver.db"

audit:
  enabled: true
  log_path: "./data/audit.log"
  retention_days: 90

filemanager:
  base_path: "/opt/easyserver/data"      # 文件管理根目录

deploy:
  encryption_key: "your-32-byte-key"     # 部署加密密钥（必须修改!）

tencentcloud:
  enabled: false
  secret_id: ""
  secret_key: ""
  region: "ap-guangzhou"
  instance_id: ""

notify:
  enabled: false
  webhook_url: ""  # 钉钉/飞书/企微 Webhook URL

alerts:
  rules: []
    # - name: "CPU告警"
    #   metric: "cpu_percent"
    #   threshold: 90
    #   duration: 60
    #   enabled: true
```

### 5.2 环境变量覆盖

所有配置都可以通过 `EASYSERVER_` 前缀的环境变量覆盖：

| 变量名 | 说明 |
|--------|------|
| `EASYSERVER_JWT_SECRET` | JWT 密钥 |
| `EASYSERVER_ENCRYPTION_KEY` | 部署加密密钥 |
| `EASYSERVER_PORT` | 监听端口 |
| `EASYSERVER_HOST` | 监听地址 |
| `EASYSERVER_DB_PATH` | 数据库路径 |
| `EASYSERVER_TLS_ENABLED` | 启用 HTTPS |
| `EASYSERVER_TENCENTCLOUD_ENABLED` | 启用腾讯云 |
| `EASYSERVER_TENCENTCLOUD_SECRET_ID` | 腾讯云 Secret ID |
| `EASYSERVER_TENCENTCLOUD_SECRET_KEY` | 腾讯云 Secret Key |

---

## 6. Nginx 反向代理

### 6.1 安装 Nginx

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install nginx

# CentOS
sudo yum install epel-release
sudo yum install nginx
```

### 6.2 配置 Nginx

```bash
sudo vi /etc/nginx/conf.d/easyserver.conf
```

**配置内容：**

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # HTTP 重定向到 HTTPS（可选）
    # return 301 https://$server_name$request_uri;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### 6.3 启用配置

```bash
# 测试配置
sudo nginx -t

# 重启 Nginx
sudo systemctl restart nginx

# 设置开机自启
sudo systemctl enable nginx
```

---

## 7. HTTPS 配置

### 7.1 使用 Let's Encrypt

```bash
# 安装 Certbot
sudo apt install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期
sudo certbot renew --dry-run
```

### 7.2 使用自签名证书

```bash
# 生成自签名证书
sudo mkdir -p /etc/nginx/ssl
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/nginx/ssl/easyserver.key \
    -out /etc/nginx/ssl/easyserver.crt

# Nginx 配置
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /etc/nginx/ssl/easyserver.crt;
    ssl_certificate_key /etc/nginx/ssl/easyserver.key;

    # ... 其他配置同上
}
```

---

## 8. 防火墙配置

### 8.1 UFW (Ubuntu)

```bash
# 允许 SSH
sudo ufw allow 22/tcp

# 允许 HTTP
sudo ufw allow 80/tcp

# 允许 HTTPS
sudo ufw allow 443/tcp

# 启用防火墙
sudo ufw enable

# 查看状态
sudo ufw status
```

### 8.2 firewalld (CentOS)

```bash
# 允许 HTTP
sudo firewall-cmd --permanent --add-service=http

# 允许 HTTPS
sudo firewall-cmd --permanent --add-service=https

# 重新加载
sudo firewall-cmd --reload
```

---

## 9. 首次访问

1. 访问 `http://your-server-ip:8080` 或配置的域名
2. **首次启动自动生成管理员账号**，用户名 `admin`，密码显示在控制台输出中
3. **立即修改默认密码！**
4. 系统会强制要求修改密码

---

## 10. 常用命令

```bash
# 启动服务
sudo systemctl start easyserver

# 停止服务
sudo systemctl stop easyserver

# 重启服务
sudo systemctl restart easyserver

# 查看状态
sudo systemctl status easyserver

# 查看日志
sudo journalctl -u easyserver -f

# 查看最近 100 行日志
sudo journalctl -u easyserver -n 100

# 编辑配置
sudo vi /opt/easyserver/config.yaml

# 重启服务使配置生效
sudo systemctl restart easyserver
```

---

## 11. 备份与恢复

### 11.1 备份

```bash
# 备份数据目录
sudo tar -czf easyserver-backup-$(date +%Y%m%d).tar.gz /opt/easyserver/data/

# 备份配置
sudo cp /opt/easyserver/config.yaml /opt/easyserver/config.yaml.bak
```

### 11.2 恢复

```bash
# 停止服务
sudo systemctl stop easyserver

# 恢复数据
sudo tar -xzf easyserver-backup-20260723.tar.gz -C /

# 启动服务
sudo systemctl start easyserver
```

---

## 12. 故障排查

### 12.1 服务无法启动

```bash
# 查看详细日志
sudo journalctl -u easyserver -n 50 --no-pager

# 检查配置文件语法
./easyserver -config config.yaml

# 检查端口占用
sudo netstat -tlnp | grep 8080
sudo lsof -i :8080
```

### 12.2 无法访问

```bash
# 检查服务状态
sudo systemctl status easyserver

# 检查防火墙
sudo ufw status
sudo firewall-cmd --list-all

# 检查 Nginx
sudo nginx -t
sudo systemctl status nginx

# 检查端口监听
sudo ss -tlnp | grep 8080
```

### 12.3 数据库问题

```bash
# 检查数据库文件
ls -la /opt/easyserver/data/easyserver.db

# 检查磁盘空间
df -h

# 检查文件权限
ls -la /opt/easyserver/data/
```

---

## 13. 性能优化

### 13.1 系统优化

```bash
# 增加文件描述符限制
echo "* soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# 优化内核参数
echo "net.core.somaxconn = 65535" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_max_syn_backlog = 65535" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

### 13.2 应用优化

```yaml
# config.yaml
monitor:
  collect_interval: 5s  # 降低采集频率
  history_retention: 168h  # 减少历史数据保留
```

---

## 14. 升级流程

```bash
# 1. 备份数据
sudo tar -czf easyserver-backup-$(date +%Y%m%d).tar.gz /opt/easyserver/data/

# 2. 停止服务
sudo systemctl stop easyserver

# 3. 替换二进制
sudo cp easyserver-linux-amd64 /opt/easyserver/easyserver
sudo chmod +x /opt/easyserver/easyserver

# 4. 启动服务
sudo systemctl start easyserver

# 5. 验证
sudo systemctl status easyserver
curl http://server-ip:8080/health
```

> **注意**：升级前务必备份数据，建议先在测试环境验证。

---

## 15. 安全建议

1. **必须修改** `jwt_secret` 和 `encryption_key`，使用 `openssl rand -base64 32` 生成
2. **生产环境** 启用 HTTPS（通过 Nginx 反向代理或直接配置 TLS）
3. **配置 IP 白名单** 限制管理面板访问来源
4. **定期备份** 数据库和配置文件
5. **启用审计日志** 记录所有操作
6. **启用 2FA** 增强登录安全
7. **配置 Cloudflare Turnstile** 防暴力破解
