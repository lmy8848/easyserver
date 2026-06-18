#!/bin/bash

# EasyServer 安装脚本
# 用法: bash install.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 打印带颜色的消息
info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否为 root 用户
check_root() {
    if [ "$EUID" -ne 0 ]; then
        error "请使用 root 用户运行此脚本"
        exit 1
    fi
}

# 检查系统要求
check_requirements() {
    info "检查系统要求..."

    # 检查 systemd
    if ! command -v systemctl &> /dev/null; then
        error "systemd 未安装"
        exit 1
    fi

    # 检查 journalctl
    if ! command -v journalctl &> /dev/null; then
        error "journalctl 未安装"
        exit 1
    fi

    info "系统要求检查通过"
}

# 创建目录
create_directories() {
    info "创建目录..."

    mkdir -p /opt/easyserver
    mkdir -p /opt/easyserver/data

    info "目录创建完成"
}

# 安装二进制文件
install_binary() {
    info "安装 EasyServer..."

    # 检查二进制文件是否存在
    if [ ! -f "easyserver-linux-amd64" ]; then
        error "未找到 easyserver-linux-amd64 文件"
        exit 1
    fi

    # 复制二进制文件
    cp easyserver-linux-amd64 /opt/easyserver/easyserver
    chmod +x /opt/easyserver/easyserver

    info "二进制文件安装完成"
}

# 生成配置文件
generate_config() {
    info "生成配置文件..."

    # 生成随机 JWT 密钥
    JWT_SECRET=$(openssl rand -base64 32)

    cat > /opt/easyserver/config.yaml << EOF
server:
  port: 8080
  host: 0.0.0.0
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

auth:
  jwt_secret: "${JWT_SECRET}"
  session_timeout: 24h
  idle_timeout: 30m
  max_login_attempts: 5
  lockout_duration: 15m
  rate_limit: 100
  rate_interval: 1m
  ip_whitelist: []

monitor:
  history_retention: 24h
  collect_interval: 1s

database:
  path: "/opt/easyserver/data/easyserver.db"

audit:
  enabled: true
  log_path: "/opt/easyserver/data/audit.log"

filemanager:
  base_path: "/"
EOF

    info "配置文件生成完成"
    warn "JWT 密钥已自动生成，请妥善保管"
}

# 创建 systemd 服务
create_service() {
    info "创建 systemd 服务..."

    cat > /etc/systemd/system/easyserver.service << EOF
[Unit]
Description=EasyServer Management Panel
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/easyserver
ExecStart=/opt/easyserver/easyserver -config /opt/easyserver/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=5
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

    # 重新加载 systemd
    systemctl daemon-reload

    info "systemd 服务创建完成"
}

# 启动服务
start_service() {
    info "启动 EasyServer..."

    systemctl start easyserver
    systemctl enable easyserver

    # 等待服务启动
    sleep 2

    # 检查服务状态
    if systemctl is-active --quiet easyserver; then
        info "EasyServer 启动成功"
    else
        error "EasyServer 启动失败"
        systemctl status easyserver
        exit 1
    fi
}

# 显示安装信息
show_info() {
    echo ""
    echo "========================================"
    echo "  EasyServer 安装完成!"
    echo "========================================"
    echo ""
    echo "访问地址: http://$(hostname -I | awk '{print $1}'):8080"
    echo ""
    echo "默认账号:"
    echo "  用户名: admin"
    echo "  密码: admin"
    echo ""
    echo "配置文件: /opt/easyserver/config.yaml"
    echo "数据目录: /opt/easyserver/data/"
    echo ""
    echo "常用命令:"
    echo "  启动: systemctl start easyserver"
    echo "  停止: systemctl stop easyserver"
    echo "  重启: systemctl restart easyserver"
    echo "  状态: systemctl status easyserver"
    echo "  日志: journalctl -u easyserver -f"
    echo ""
    echo "========================================"
}

# 主函数
main() {
    echo "========================================"
    echo "  EasyServer 安装脚本"
    echo "========================================"
    echo ""

    check_root
    check_requirements
    create_directories
    install_binary
    generate_config
    create_service
    start_service
    show_info
}

# 运行主函数
main
