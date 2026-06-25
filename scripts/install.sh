#!/bin/bash

# EasyServer 安装脚本（包装器）
# 此脚本委托给 deploy/install.sh，避免维护两份重复代码
# 用法: bash scripts/install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_SCRIPT="${SCRIPT_DIR}/../deploy/install.sh"

if [ ! -f "$DEPLOY_SCRIPT" ]; then
    echo "[ERROR] 未找到 deploy/install.sh"
    echo "请从项目根目录运行此脚本，或直接使用 deploy/install.sh"
    exit 1
fi

exec bash "$DEPLOY_SCRIPT" "$@"
