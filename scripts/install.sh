#!/bin/bash
# EasyServer 安装脚本:安装二进制 + 注册 systemd 服务
# 用法: bash scripts/install.sh [版本号]   # 版本号默认 latest

set -euo pipefail

VERSION="${1:-latest}"
INSTALL_DIR=/opt/easyserver
BINARY=/usr/local/bin/easyserver
SERVICE=/etc/systemd/system/easyserver.service
BASE_URL=https://github.com/lmy8848/easyserver/releases/download

info() { echo -e "\033[0;32m[INFO]\033[0m $*"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $*"; }
error() { echo -e "\033[0;31m[ERROR]\033[0m $*"; }

[ "$EUID" -eq 0 ] || { error "请以 root 用户运行"; exit 1; }
command -v systemctl >/dev/null 2>&1 || { error "systemd 不可用"; exit 1; }

mkdir -p "${INSTALL_DIR}/data"

# --- 安装二进制 ---
if [ -f ./easyserver-linux-amd64 ]; then
    info "使用本地 easyserver-linux-amd64"
    cp ./easyserver-linux-amd64 "$BINARY"
else
    info "从 GitHub Releases 下载 easyserver-linux-amd64 @ ${VERSION} ..."
    TMP="$(mktemp -d)"
    trap 'rm -rf "$TMP"' EXIT
    curl -fsSL -o "${TMP}/easyserver-linux-amd64" "${BASE_URL}/${VERSION}/easyserver-linux-amd64"
    curl -fsSL -o "${TMP}/checksums-sha256.txt" "${BASE_URL}/${VERSION}/checksums-sha256.txt"
    (cd "$TMP" && grep easyserver-linux-amd64 checksums-sha256.txt | sha256sum -c -)
    cp "${TMP}/easyserver-linux-amd64" "$BINARY"
fi
chmod +x "$BINARY"
info "二进制安装完成"

if [ ! -f "${INSTALL_DIR}/config.toml" ]; then
    info "创建空 config.toml"
    touch "${INSTALL_DIR}/config.toml"
else
    warn "config.toml 已存在,跳过创建"
fi

# --- systemd 服务 ---
# SECURITY NOTE: 以 root 运行是系统管理功能(服务控制/防火墙/包管理)所必需的;
# 高安全环境可改为专用用户并配置相应 sudo 权限
warn "服务将以 root 用户运行,这是系统管理功能所必需的"
info "注册 systemd 服务..."
cat > "$SERVICE" <<EOF
[Unit]
Description=EasyServer Management Panel
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${BINARY} -config ${INSTALL_DIR}/config.toml
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
systemctl daemon-reload
systemctl enable --now easyserver

# --- 等待首次启动并抓取随机生成的管理员账号与密码 ---
# 首次安装时程序会在启动日志打印随机账号与密码;已初始化过的实例无此行,超时后跳过
ADMIN_USERNAME=""
ADMIN_PASSWORD=""
for i in $(seq 1 30); do
    ADMIN_INFO=$(journalctl -u easyserver --no-pager -n 300 2>/dev/null | grep -A4 管理员账号 || true)
    if [ -n "$ADMIN_INFO" ]; then
        ADMIN_USERNAME=$(echo "$ADMIN_INFO" | sed -n 's/^用户名: *//p' | head -1)
        ADMIN_PASSWORD=$(echo "$ADMIN_INFO" | sed -n 's/^密码: *//p' | head -1)
        [ -n "$ADMIN_USERNAME" ] && [ -n "$ADMIN_PASSWORD" ] && break
    fi
    sleep 1
done

echo ""
echo "========================================"
echo "  EasyServer 安装完成!"
echo "========================================"
echo "访问地址:"
LAN_IP=$(hostname -I | awk '{print $1}')
echo "  内网: http://${LAN_IP}:8080"
WAN_IP=$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)
if [ -n "$WAN_IP" ]; then
    echo "  外网: http://${WAN_IP}:8080"
fi
if [ -n "$ADMIN_USERNAME" ] && [ -n "$ADMIN_PASSWORD" ]; then
    echo "管理员账号:"
    echo "  用户名: ${ADMIN_USERNAME}"
    echo "  密码:   ${ADMIN_PASSWORD}"
    echo "  请登录后立即修改密码!"
else
    echo "管理员账号: (首次安装信息见启动日志,查看:)"
    echo "  journalctl -u easyserver --no-pager | grep -A4 管理员账号"
fi
echo "配置文件: ${INSTALL_DIR}/config.toml"
echo "数据目录: ${INSTALL_DIR}/data/"
echo "运行日志: ${INSTALL_DIR}/easyserver.log"
echo "常用命令: systemctl {start|stop|restart|status} easyserver"
echo "实时日志: journalctl -u easyserver -f"
echo "命令行工具: easyserver --help"
echo "========================================"
