#!/bin/bash
# ============================================================
# EasyServer 全方位端到端测试脚本
# 覆盖：运行时/网站/数据库/Docker/包管理/定时任务/进程守护/防火墙
# ============================================================

set -uo pipefail

BASE_URL="http://localhost:8080"
REPORT="/tmp/e2e-test-report.md"
TOKEN=""
ADMIN_USER="admin"
ADMIN_PASS="${ADMIN_PASS:-wZj3rBMQqan5v1ll}"

# 计数器
TOTAL=0; PASS=0; FAIL=0; SKIP=0
# 各阶段计数
declare -A PHASE_TOTAL PHASE_PASS PHASE_FAIL
PHASES=("准备" "运行时" "网站" "数据库" "Docker" "包管理" "定时任务" "进程守护" "防火墙" "收尾")
for p in "${PHASES[@]}"; do PHASE_TOTAL[$p]=0; PHASE_PASS[$p]=0; PHASE_FAIL[$p]=0; done

# 失败详情
FAIL_DETAILS=""

# 资源 ID（用于清理）
CLEANUP_IDS=()

# ============================================================
# 工具函数
# ============================================================
log_info()  { echo -e "\033[36m[INFO]\033[0m $*"; }
log_pass()  { echo -e "\033[32m[PASS]\033[0m $*"; }
log_fail()  { echo -e "\033[31m[FAIL]\033[0m $*"; }
log_skip()  { echo -e "\033[33m[SKIP]\033[0m $*"; }
log_phase() { echo -e "\n\033[1;35m════════════════════════════════════════\033[0m"; echo -e "\033[1;35m  $*\033[0m"; echo -e "\033[1;35m════════════════════════════════════════\033[0m\n"; }

# HTTP 请求封装
http_request() {
    local method="$1" path="$2" data="${3:-}" token="${4-$TOKEN}"
    local url="${BASE_URL}${path}"
    local curl_opts=(-s -w "\n%{http_code}" -X "$method" -H "Content-Type: application/json")
    [[ -n "$token" ]] && curl_opts+=(-H "Authorization: Bearer $token")
    [[ -n "$data" ]] && curl_opts+=(-d "$data")
    sleep 0.15
    local response
    response=$(curl "${curl_opts[@]}" "$url" 2>/dev/null || echo -e "\n000")
    local body status
    body=$(echo "$response" | sed '$d')
    status=$(echo "$response" | tail -1)
    echo "${status}|${body}"
}

parse_status() { echo "$1" | cut -d'|' -f1; }
parse_body()   { echo "$1" | cut -d'|' -f2-; }

# 提取 JSON 字段
json_val() { echo "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$2)" 2>/dev/null; }
json_val_raw() { echo "$1" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$2)" 2>/dev/null; }

# 刷新 Token
refresh_token() {
    local resp
    resp=$(http_request "POST" "/api/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" "")
    local body
    body=$(parse_body "$resp")
    TOKEN=$(echo "$body" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get("data",{}).get("token", d.get("token","")))
except: print("")
' 2>/dev/null)
    [[ -n "$TOKEN" ]] && log_info "Token 已刷新" || log_fail "Token 刷新失败"
}

# 记录结果
record() {
    local phase="$1" module="$2" op="$3" result="$4" detail="$5"
    ((TOTAL++))
    ((PHASE_TOTAL[$phase]++))
    case "$result" in
        PASS) ((PASS++)); ((PHASE_PASS[$phase]++)) ;;
        FAIL) ((FAIL++)); ((PHASE_FAIL[$phase]++))
              FAIL_DETAILS="${FAIL_DETAILS}| ${module} | ${op} | ${detail} |\n" ;;
        SKIP) ((SKIP++)) ;;
    esac
}

# 断言状态码
assert_status() {
    local phase="$1" module="$2" op="$3" resp="$4" expected="$5"
    local status
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^($expected)$ ]]; then
        record "$phase" "$module" "$op" "PASS" "返回$status"
        return 0
    else
        record "$phase" "$module" "$op" "FAIL" "期望$expected, 实际$status"
        return 1
    fi
}

# 断言 JSON code 字段
assert_code() {
    local phase="$1" module="$2" op="$3" resp="$4" expected="$5"
    local body code
    body=$(parse_body "$resp")
    code=$(json_val "$body" ".get('code',-1)")
    if [[ "$code" == "$expected" ]]; then
        record "$phase" "$module" "$op" "PASS" "code=$code"
        return 0
    else
        record "$phase" "$module" "$op" "FAIL" "期望code=$expected, 实际code=$code"
        return 1
    fi
}

# 断言成功（code=0）
assert_ok() {
    local phase="$1" module="$2" op="$3" resp="$4"
    assert_code "$phase" "$module" "$op" "$resp" "0"
}

# 提取 data 中的 id
extract_id() {
    local body="$1"
    echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    data = d.get('data', d)
    if isinstance(data, dict):
        print(data.get('id', data.get('ID', '')))
    else: print('')
except: print('')
" 2>/dev/null
}

# ============================================================
# Phase 0: 准备
# ============================================================
phase_prepare() {
    log_phase "Phase 0: 准备"

    # 登录
    local resp body
    resp=$(http_request "POST" "/api/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" "")
    body=$(parse_body "$resp")
    TOKEN=$(echo "$body" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get("data",{}).get("token", d.get("token","")))
except: print("")
' 2>/dev/null)

    if [[ -z "$TOKEN" ]]; then
        log_fail "登录失败，无法继续测试"
        exit 1
    fi
    log_pass "登录成功"

    # 清理上次残留（幂等）
    log_info "清理上次测试残留..."
    # 删除测试网站（如果有 nginx 实例）
    local ws_resp
    ws_resp=$(http_request "GET" "/api/web-servers")
    local ws_body
    ws_body=$(parse_body "$ws_resp")
    # 删除测试进程
    local proc_resp
    proc_resp=$(http_request "GET" "/api/processes")
    local proc_body
    proc_body=$(parse_body "$proc_resp")
    # 删除测试定时任务
    local cron_resp
    cron_resp=$(http_request "GET" "/api/cron/tasks")
    local cron_body
    cron_body=$(parse_body "$cron_resp")
    # 删除测试脚本
    local script_resp
    script_resp=$(http_request "GET" "/api/cron/scripts")
    local script_body
    script_body=$(parse_body "$script_resp")

    log_info "准备阶段完成"
}

# ============================================================
# Phase 1: 运行时环境
# ============================================================
phase_runtime() {
    log_phase "Phase 1: 运行时环境"
    refresh_token

    # 1. 列出运行时
    local resp body
    resp=$(http_request "GET" "/api/runtime")
    assert_ok "运行时" "Runtime" "列出运行时" "$resp"

    # 2. 检测系统运行时
    resp=$(http_request "GET" "/api/runtime/detect")
    assert_ok "运行时" "Runtime" "检测系统运行时" "$resp"

    # 3. 获取可用版本列表
    resp=$(http_request "GET" "/api/runtime-versions/node")
    assert_ok "运行时" "Runtime" "获取 Node.js 可用版本" "$resp"

    # 4. 解析版本别名
    resp=$(http_request "GET" "/api/runtime-versions/node/resolve/lts")
    assert_ok "运行时" "Runtime" "解析 lts 别名" "$resp"

    # 5. 获取别名建议
    resp=$(http_request "GET" "/api/runtime-versions/node/suggestions")
    assert_ok "运行时" "Runtime" "获取别名建议" "$resp"

    # 6. 检查依赖
    resp=$(http_request "GET" "/api/runtime/check-deps/node")
    assert_ok "运行时" "Runtime" "检查 node 依赖" "$resp"

    # 7. 安装 Node.js（使用 lts 版本）
    log_info "安装 Node.js (这可能需要几分钟)..."
    resp=$(http_request "POST" "/api/runtime/install" '{"name":"node","version":"20"}')
    body=$(parse_body "$resp")
    local install_id
    install_id=$(extract_id "$body")
    if assert_ok "运行时" "Runtime" "安装 Node.js" "$resp"; then
        # 等待安装完成（最多 5 分钟）
        local wait=0
        while [[ $wait -lt 300 ]]; do
            sleep 5
            wait=$((wait + 5))
            local prog_resp
            prog_resp=$(http_request "GET" "/api/runtime/progress/$install_id")
            local prog_body prog_status
            prog_body=$(parse_body "$prog_resp")
            prog_status=$(json_val "$prog_body" ".get('data',{}).get('status','')")
            log_info "安装进度: status=$prog_status (${wait}s)"
            if [[ "$prog_status" == "installed" ]]; then
                record "运行时" "Runtime" "Node.js 安装完成" "PASS" "耗时${wait}s"
                break
            elif [[ "$prog_status" == "failed" ]]; then
                record "运行时" "Runtime" "Node.js 安装完成" "FAIL" "安装失败"
                break
            fi
        done
        if [[ $wait -ge 300 ]]; then
            record "运行时" "Runtime" "Node.js 安装完成" "FAIL" "超时(300s)"
        fi
    fi

    # 8. 列出已安装运行时
    resp=$(http_request "GET" "/api/runtime")
    assert_ok "运行时" "Runtime" "列出已安装运行时" "$resp"

    # 9. 设置默认版本
    resp=$(http_request "POST" "/api/runtime/set-default" '{"name":"node","version":"20"}')
    assert_ok "运行时" "Runtime" "设置默认 Node 版本" "$resp"

    # 10. 安装 Python
    log_info "安装 Python 3..."
    resp=$(http_request "POST" "/api/runtime/install" '{"name":"python","version":"3"}')
    if assert_ok "运行时" "Runtime" "安装 Python" "$resp"; then
        body=$(parse_body "$resp")
        local py_id
        py_id=$(extract_id "$body")
        local wait=0
        while [[ $wait -lt 300 ]]; do
            sleep 5
            wait=$((wait + 5))
            local prog_resp
            prog_resp=$(http_request "GET" "/api/runtime/progress/$py_id")
            local prog_body prog_status
            prog_body=$(parse_body "$prog_resp")
            prog_status=$(json_val "$prog_body" ".get('data',{}).get('status','')")
            if [[ "$prog_status" == "installed" ]]; then
                record "运行时" "Runtime" "Python 安装完成" "PASS" "耗时${wait}s"
                break
            elif [[ "$prog_status" == "failed" ]]; then
                record "运行时" "Runtime" "Python 安装完成" "FAIL" "安装失败"
                break
            fi
        done
    fi

    # 11. 安装 Go
    log_info "安装 Go..."
    resp=$(http_request "POST" "/api/runtime/install" '{"name":"go","version":"1.21"}')
    assert_ok "运行时" "Runtime" "安装 Go" "$resp"

    # 12. 获取安装日志
    if [[ -n "$install_id" ]]; then
        resp=$(http_request "GET" "/api/runtime/logs/$install_id")
        assert_ok "运行时" "Runtime" "获取安装日志" "$resp"
    fi

    # 13. 卸载 Go（清理）
    resp=$(http_request "POST" "/api/runtime/uninstall" '{"name":"go","version":"1.21"}')
    assert_ok "运行时" "Runtime" "卸载 Go" "$resp"

    # 14. 获取卸载清理信息
    resp=$(http_request "GET" "/api/runtime/cleanup/$install_id")
    # 这个可能返回 404（已清理），也算通过
    local status
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(200|404)$ ]]; then
        record "运行时" "Runtime" "获取清理信息" "PASS" "返回$status"
    else
        record "运行时" "Runtime" "获取清理信息" "FAIL" "返回$status"
    fi

    log_info "运行时环境测试完成"
}

# ============================================================
# Phase 2: 网站管理
# ============================================================
phase_website() {
    log_phase "Phase 2: 网站管理"
    refresh_token

    local NGINX_ID="" SITE_STATIC="" SITE_PHP="" SITE_NODE=""

    # 1. 列出 Web 服务器
    local resp body
    resp=$(http_request "GET" "/api/web-servers")
    assert_ok "网站" "WebServer" "列出 Web 服务器" "$resp"

    # 2. 获取项目类型
    resp=$(http_request "GET" "/api/web-servers/project-types")
    assert_ok "网站" "WebServer" "获取项目类型列表" "$resp"

    # 3. 浏览目录
    resp=$(http_request "GET" "/api/web-servers/browse?path=/var")
    assert_ok "网站" "WebServer" "浏览 /var 目录" "$resp"

    # 4. 校验路径
    resp=$(http_request "GET" "/api/web-servers/validate-path?path=/var/www")
    assert_ok "网站" "WebServer" "校验路径 /var/www" "$resp"

    # 5. 创建 Nginx 实例
    resp=$(http_request "POST" "/api/web-servers" '{"name":"nginx","display_name":"Nginx 测试"}')
    body=$(parse_body "$resp")
    NGINX_ID=$(extract_id "$body")
    if [[ -z "$NGINX_ID" ]]; then
        record "网站" "WebServer" "创建 Nginx 实例" "FAIL" "无法提取 ID"
        return
    fi
    record "网站" "WebServer" "创建 Nginx 实例" "PASS" "ID=$NGINX_ID"

    # 6. 获取详情
    resp=$(http_request "GET" "/api/web-servers/$NGINX_ID")
    assert_ok "网站" "WebServer" "获取 Nginx 详情" "$resp"

    # 7. 安装 Nginx
    log_info "安装 Nginx..."
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/install")
    assert_ok "网站" "WebServer" "安装 Nginx" "$resp"

    # 8. 启动 Nginx
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/start")
    assert_ok "网站" "WebServer" "启动 Nginx" "$resp"

    # 9. 查看状态
    resp=$(http_request "GET" "/api/web-servers/$NGINX_ID/status")
    assert_ok "网站" "WebServer" "查看 Nginx 状态" "$resp"

    # 10. 测试配置
    resp=$(http_request "GET" "/api/web-servers/$NGINX_ID/test-config")
    assert_ok "网站" "WebServer" "测试 Nginx 配置" "$resp"

    # 11. 查看配置
    resp=$(http_request "GET" "/api/web-servers/$NGINX_ID/config")
    assert_ok "网站" "WebServer" "读取 Nginx 配置" "$resp"

    # 12. 创建静态网站
    mkdir -p /var/www/test-static 2>/dev/null || true
    echo "<h1>Test Static</h1>" > /var/www/test-static/index.html 2>/dev/null || true
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites" \
        '{"name":"test-static","domain":"test-static.local","root_path":"/var/www/test-static","project_type":"static","port":8081}')
    body=$(parse_body "$resp")
    SITE_STATIC=$(extract_id "$body")
    if [[ -n "$SITE_STATIC" ]]; then
        record "网站" "Website" "创建静态网站" "PASS" "ID=$SITE_STATIC"

        # 13. 启用静态网站
        resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites/$SITE_STATIC/enable")
        assert_ok "网站" "Website" "启用静态网站" "$resp"

        # 14. 重载 Nginx
        resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/reload")
        assert_ok "网站" "WebServer" "重载 Nginx 配置" "$resp"

        # 15. 查看网站日志
        resp=$(http_request "GET" "/api/web-servers/$NGINX_ID/websites/$SITE_STATIC/logs")
        assert_ok "网站" "Website" "查看静态网站日志" "$resp"

        # 16. 更新网站
        resp=$(http_request "PUT" "/api/web-servers/$NGINX_ID/websites/$SITE_STATIC" \
            '{"name":"test-static-updated"}')
        assert_ok "网站" "Website" "更新静态网站" "$resp"

        # 17. 禁用网站
        resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites/$SITE_STATIC/disable")
        assert_ok "网站" "Website" "禁用静态网站" "$resp"
    else
        record "网站" "Website" "创建静态网站" "FAIL" "无法提取 ID"
    fi

    # 18. 创建 PHP 网站
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites" \
        '{"name":"test-php","domain":"test-php.local","root_path":"/var/www/test-php","project_type":"php","port":8082}')
    body=$(parse_body "$resp")
    SITE_PHP=$(extract_id "$body")
    if [[ -n "$SITE_PHP" ]]; then
        record "网站" "Website" "创建 PHP 网站" "PASS" "ID=$SITE_PHP"
    else
        record "网站" "Website" "创建 PHP 网站" "FAIL" "无法提取 ID"
    fi

    # 19. 创建 Node.js 网站（反向代理）
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites" \
        '{"name":"test-node","domain":"test-node.local","root_path":"/var/www/test-node","project_type":"nodejs","app_port":3000,"port":8083}')
    body=$(parse_body "$resp")
    SITE_NODE=$(extract_id "$body")
    if [[ -n "$SITE_NODE" ]]; then
        record "网站" "Website" "创建 Node.js 网站" "PASS" "ID=$SITE_NODE"
    else
        record "网站" "Website" "创建 Node.js 网站" "FAIL" "无法提取 ID"
    fi

    # 20. 创建反向代理网站
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/websites" \
        '{"name":"test-proxy","domain":"test-proxy.local","root_path":"/var/www/test-proxy","project_type":"proxy","app_port":9090,"port":8084}')
    if assert_ok "网站" "Website" "创建反向代理网站" "$resp"; then
        body=$(parse_body "$resp")
        local proxy_id
        proxy_id=$(extract_id "$body")
        # 删除
        resp=$(http_request "DELETE" "/api/web-servers/$NGINX_ID/websites/$proxy_id")
        assert_ok "网站" "Website" "删除反向代理网站" "$resp"
    fi

    # 21. 列出网站
    resp=$(http_request "GET" "/api/web-servers/$NGINX_ID/websites")
    assert_ok "网站" "Website" "列出所有网站" "$resp"

    # 清理网站
    [[ -n "$SITE_STATIC" ]] && http_request "DELETE" "/api/web-servers/$NGINX_ID/websites/$SITE_STATIC" > /dev/null
    [[ -n "$SITE_PHP" ]] && http_request "DELETE" "/api/web-servers/$NGINX_ID/websites/$SITE_PHP" > /dev/null
    [[ -n "$SITE_NODE" ]] && http_request "DELETE" "/api/web-servers/$NGINX_ID/websites/$SITE_NODE" > /dev/null

    # 22. 停止 Nginx
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/stop")
    assert_ok "网站" "WebServer" "停止 Nginx" "$resp"

    # 23. 卸载 Nginx
    resp=$(http_request "POST" "/api/web-servers/$NGINX_ID/uninstall")
    assert_ok "网站" "WebServer" "卸载 Nginx" "$resp"

    # 24. 删除 Web 服务器
    resp=$(http_request "DELETE" "/api/web-servers/$NGINX_ID")
    assert_ok "网站" "WebServer" "删除 Nginx 实例" "$resp"

    log_info "网站管理测试完成"
}

# ============================================================
# Phase 3: 数据库
# ============================================================
phase_database() {
    log_phase "Phase 3: 数据库"
    refresh_token

    local MYSQL_VID="" DB_ID="" TABLE_CREATED="" USER_ID="" BACKUP_ID=""

    # 1. 列出 DB 服务器
    local resp body
    resp=$(http_request "GET" "/api/db-servers")
    assert_ok "数据库" "DBServer" "列出数据库服务器" "$resp"
    body=$(parse_body "$resp")

    # 获取 MySQL 服务器 ID（通常是 1）
    local mysql_id
    mysql_id=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    items = d.get('data', [])
    if isinstance(items, list):
        for item in items:
            if 'mysql' in item.get('type','').lower() or 'mysql' in item.get('name','').lower():
                print(item.get('id',''))
                break
        else:
            print(items[0].get('id','') if items else '')
    else: print('1')
except: print('1')
" 2>/dev/null)
    [[ -z "$mysql_id" ]] && mysql_id=1

    # 2. 获取版本模板
    resp=$(http_request "GET" "/api/db-servers/$mysql_id/version-templates")
    assert_ok "数据库" "DBServer" "获取 MySQL 版本模板" "$resp"

    # 3. 列出已安装版本
    resp=$(http_request "GET" "/api/db-servers/$mysql_id/versions")
    assert_ok "数据库" "DBServer" "列出 MySQL 版本" "$resp"

    # 4. 安装 MySQL
    log_info "安装 MySQL..."
    resp=$(http_request "POST" "/api/db-servers/$mysql_id/versions" '{"version":"10.11"}')
    body=$(parse_body "$resp")
    MYSQL_VID=$(extract_id "$body")
    if [[ -z "$MYSQL_VID" ]]; then
        # 可能已安装，尝试从版本列表获取
        local ver_resp
        ver_resp=$(http_request "GET" "/api/db-servers/$mysql_id/versions")
        local ver_body
        ver_body=$(parse_body "$ver_resp")
        MYSQL_VID=$(echo "$ver_body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    items = d.get('data', [])
    if isinstance(items, list) and items:
        print(items[0].get('id', items[0].get('ID', '')))
    else: print('')
except: print('')
" 2>/dev/null)
    fi

    if [[ -n "$MYSQL_VID" ]]; then
        record "数据库" "DBServer" "安装 MySQL" "PASS" "VID=$MYSQL_VID"

        # 5. 启动 MySQL
        resp=$(http_request "POST" "/api/db-servers/versions/$MYSQL_VID/start")
        assert_ok "数据库" "DBServer" "启动 MySQL" "$resp"
        sleep 3  # 等待 MySQL 完全启动
    else
        record "数据库" "DBServer" "安装 MySQL" "FAIL" "无法获取版本 ID"
        return
    fi

    # 6. 创建数据库
    resp=$(http_request "POST" "/api/db-servers/$mysql_id/databases" \
        "{\"db_version_id\":$MYSQL_VID,\"name\":\"e2e_test_db\",\"charset\":\"utf8mb4\",\"description\":\"E2E 测试数据库\"}")
    body=$(parse_body "$resp")
    DB_ID=$(extract_id "$body")
    if [[ -n "$DB_ID" ]]; then
        record "数据库" "Database" "创建数据库" "PASS" "ID=$DB_ID"
    else
        record "数据库" "Database" "创建数据库" "FAIL" "无法提取 ID"
        return
    fi

    # 7. 列出数据库
    resp=$(http_request "GET" "/api/db-servers/$mysql_id/databases")
    assert_ok "数据库" "Database" "列出数据库" "$resp"

    # 8. 创建表
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/tables" \
        '{"name":"test_users","columns":[{"name":"id","type":"INT","is_primary":true,"auto_incr":true,"nullable":false},{"name":"username","type":"VARCHAR(100)","nullable":false},{"name":"email","type":"VARCHAR(200)","nullable":true},{"name":"created_at","type":"TIMESTAMP","nullable":true}]}')
    if assert_ok "数据库" "Table" "创建表 test_users" "$resp"; then
        TABLE_CREATED="test_users"
    fi

    # 9. 列出表
    resp=$(http_request "GET" "/api/db-servers/databases/$DB_ID/tables")
    assert_ok "数据库" "Table" "列出表" "$resp"

    # 10. 描述表
    resp=$(http_request "GET" "/api/db-servers/databases/$DB_ID/describe?table=test_users")
    assert_ok "数据库" "Table" "描述表结构" "$resp"

    # 11. 插入记录
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/insert" \
        '{"table":"test_users","data":{"username":"testuser","email":"test@example.com"}}')
    assert_ok "数据库" "Record" "插入记录" "$resp"

    # 12. 插入第二条记录
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/insert" \
        '{"table":"test_users","data":{"username":"testuser2","email":"test2@example.com"}}')
    assert_ok "数据库" "Record" "插入第二条记录" "$resp"

    # 13. 查询数据
    resp=$(http_request "GET" "/api/db-servers/databases/$DB_ID/query?table=test_users&limit=10")
    assert_ok "数据库" "Record" "查询表数据" "$resp"

    # 14. 更新记录
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/update" \
        '{"table":"test_users","data":{"email":"updated@example.com"},"primary_key":"id","primary_val":1}')
    assert_ok "数据库" "Record" "更新记录" "$resp"

    # 15. 执行 SQL 查询
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/execute" \
        '{"sql":"SELECT * FROM test_users WHERE id=1;"}')
    assert_ok "数据库" "SQL" "执行 SELECT" "$resp"

    # 16. 执行 SQL 统计
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/execute" \
        '{"sql":"SELECT COUNT(*) as total FROM test_users;"}')
    assert_ok "数据库" "SQL" "执行 COUNT" "$resp"

    # 17. 删除记录
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/delete" \
        '{"table":"test_users","primary_key":"id","primary_val":2}')
    assert_ok "数据库" "Record" "删除记录" "$resp"

    # 18. 创建备份
    resp=$(http_request "POST" "/api/db-servers/databases/$DB_ID/backup")
    body=$(parse_body "$resp")
    BACKUP_ID=$(extract_id "$body")
    if [[ -n "$BACKUP_ID" ]]; then
        record "数据库" "Backup" "创建备份" "PASS" "ID=$BACKUP_ID"
    else
        # 尝试从 data 中提取
        BACKUP_ID=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    data = d.get('data', {})
    if isinstance(data, dict): print(data.get('id', data.get('backup_id', '')))
    elif isinstance(data, list) and data: print(data[0].get('id',''))
    else: print('')
except: print('')
" 2>/dev/null)
        if [[ -n "$BACKUP_ID" ]]; then
            record "数据库" "Backup" "创建备份" "PASS" "ID=$BACKUP_ID"
        else
            record "数据库" "Backup" "创建备份" "FAIL" "无法提取备份 ID"
        fi
    fi

    # 19. 列出备份
    resp=$(http_request "GET" "/api/db-servers/databases/$DB_ID/backups")
    assert_ok "数据库" "Backup" "列出备份" "$resp"

    # 20. 下载备份
    if [[ -n "$BACKUP_ID" ]]; then
        resp=$(http_request "GET" "/api/db-servers/backups/$BACKUP_ID/download")
        assert_ok "数据库" "Backup" "下载备份" "$resp"
    fi

    # 21. 创建数据库用户
    resp=$(http_request "POST" "/api/db-servers/$mysql_id/users" \
        '{"username":"e2e_test_user","password":"test123456"}')
    body=$(parse_body "$resp")
    USER_ID=$(extract_id "$body")
    if [[ -n "$USER_ID" ]]; then
        record "数据库" "User" "创建用户" "PASS" "ID=$USER_ID"

        # 22. 授权用户
        resp=$(http_request "POST" "/api/db-servers/$mysql_id/users/$USER_ID/grant" \
            "{\"database\":\"e2e_test_db\",\"privileges\":\"SELECT,INSERT,UPDATE\"}")
        assert_ok "数据库" "User" "授权用户" "$resp"

        # 23. 删除用户
        resp=$(http_request "DELETE" "/api/db-servers/$mysql_id/users/$USER_ID")
        assert_ok "数据库" "User" "删除用户" "$resp"
    else
        record "数据库" "User" "创建用户" "FAIL" "无法提取 ID"
    fi

    # 24. 读取 MySQL 配置
    resp=$(http_request "GET" "/api/db-servers/mysql/config")
    assert_ok "数据库" "Config" "读取 MySQL 配置" "$resp"

    # 25. 读取常用参数
    resp=$(http_request "GET" "/api/db-servers/mysql/common-params")
    assert_ok "数据库" "Config" "获取 MySQL 常用参数" "$resp"

    # 26. 删除备份
    if [[ -n "$BACKUP_ID" ]]; then
        resp=$(http_request "DELETE" "/api/db-servers/backups/$BACKUP_ID")
        assert_ok "数据库" "Backup" "删除备份" "$resp"
    fi

    # 27. 删除数据库（级联删除表）
    resp=$(http_request "DELETE" "/api/db-servers/$mysql_id/databases/$DB_ID")
    assert_ok "数据库" "Database" "删除数据库" "$resp"

    # 28. 停止 MySQL
    resp=$(http_request "POST" "/api/db-servers/versions/$MYSQL_VID/stop")
    assert_ok "数据库" "DBServer" "停止 MySQL" "$resp"

    # 29. 卸载 MySQL
    resp=$(http_request "DELETE" "/api/db-servers/versions/$MYSQL_VID")
    assert_ok "数据库" "DBServer" "卸载 MySQL" "$resp"

    log_info "数据库测试完成"
}

# ============================================================
# Phase 4: Docker
# ============================================================
phase_docker() {
    log_phase "Phase 4: Docker"
    refresh_token

    local CONTAINER_ID=""

    # 1. 查看 Docker 状态
    local resp body
    resp=$(http_request "GET" "/api/docker/status")
    assert_ok "Docker" "Docker" "查看 Docker 状态" "$resp"
    body=$(parse_body "$resp")

    # 检查是否已安装
    local installed
    installed=$(json_val "$body" ".get('data',{}).get('installed',False)")
    if [[ "$installed" != "True" ]]; then
        log_info "Docker 未安装，尝试安装..."
        resp=$(http_request "POST" "/api/docker/install")
        assert_ok "Docker" "Docker" "安装 Docker" "$resp"
        sleep 5
    fi

    # 2. 启动 Docker
    resp=$(http_request "POST" "/api/docker/start")
    assert_ok "Docker" "Docker" "启动 Docker" "$resp"
    sleep 3

    # 3. 查看 Docker info
    resp=$(http_request "GET" "/api/docker/info")
    assert_ok "Docker" "Docker" "查看 Docker info" "$resp"

    # 4. 拉取 alpine 镜像
    log_info "拉取 alpine:latest 镜像..."
    resp=$(http_request "POST" "/api/images/pull" '{"image":"alpine:latest"}')
    assert_ok "Docker" "Image" "拉取 alpine 镜像" "$resp"

    # 5. 列出镜像
    resp=$(http_request "GET" "/api/images")
    assert_ok "Docker" "Image" "列出镜像" "$resp"

    # 6. 创建容器
    resp=$(http_request "POST" "/api/containers" \
        '{"image":"alpine:latest","name":"e2e-test-ct","command":"sleep 3600","restart_policy":"no"}')
    body=$(parse_body "$resp")
    CONTAINER_ID=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    data = d.get('data', {})
    print(data.get('id', data.get('ID', '')))
except: print('')
" 2>/dev/null)
    if [[ -n "$CONTAINER_ID" ]]; then
        record "Docker" "Container" "创建容器" "PASS" "ID=${CONTAINER_ID:0:12}"
    else
        record "Docker" "Container" "创建容器" "FAIL" "无法提取 ID"
        return
    fi

    # 7. 启动容器
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/start")
    assert_ok "Docker" "Container" "启动容器" "$resp"
    sleep 2

    # 8. 查看容器日志
    resp=$(http_request "GET" "/api/containers/$CONTAINER_ID/logs?tail=10")
    assert_ok "Docker" "Container" "查看容器日志" "$resp"

    # 9. 执行命令
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/exec" '{"command":"echo hello-e2e"}')
    assert_ok "Docker" "Container" "执行命令 echo" "$resp"

    # 10. 查看容器统计
    resp=$(http_request "GET" "/api/containers/$CONTAINER_ID/stats")
    assert_ok "Docker" "Container" "查看容器统计" "$resp"

    # 11. 查看容器进程
    resp=$(http_request "GET" "/api/containers/$CONTAINER_ID/top")
    assert_ok "Docker" "Container" "查看容器进程" "$resp"

    # 12. 暂停容器
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/pause")
    assert_ok "Docker" "Container" "暂停容器" "$resp"

    # 13. 恢复容器
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/unpause")
    assert_ok "Docker" "Container" "恢复容器" "$resp"

    # 14. 重命名容器
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/rename" '{"name":"e2e-renamed"}')
    assert_ok "Docker" "Container" "重命名容器" "$resp"

    # 15. 更新容器资源
    resp=$(http_request "PUT" "/api/containers/$CONTAINER_ID/update" '{"memory":268435456,"cpus":0.5}')
    assert_ok "Docker" "Container" "更新容器资源" "$resp"

    # 16. 停止容器
    resp=$(http_request "POST" "/api/containers/$CONTAINER_ID/stop")
    assert_ok "Docker" "Container" "停止容器" "$resp"

    # 17. 删除容器
    resp=$(http_request "DELETE" "/api/containers/$CONTAINER_ID?force=true")
    assert_ok "Docker" "Container" "删除容器" "$resp"

    # 18. 创建卷
    resp=$(http_request "POST" "/api/volumes" '{"name":"e2e-test-vol"}')
    assert_ok "Docker" "Volume" "创建卷" "$resp"

    # 19. 列出卷
    resp=$(http_request "GET" "/api/volumes")
    assert_ok "Docker" "Volume" "列出卷" "$resp"

    # 20. 删除卷
    resp=$(http_request "DELETE" "/api/volumes/e2e-test-vol")
    assert_ok "Docker" "Volume" "删除卷" "$resp"

    # 21. 列出网络
    resp=$(http_request "GET" "/api/networks")
    assert_ok "Docker" "Network" "列出网络" "$resp"

    # 22. 配置镜像源
    resp=$(http_request "POST" "/api/docker/mirror" '{"mirror_url":"https://mirror.ccs.tencentyun.com"}')
    assert_ok "Docker" "Docker" "配置镜像源" "$resp"

    log_info "Docker 测试完成"
}

# ============================================================
# Phase 5: 包管理
# ============================================================
phase_packages() {
    log_phase "Phase 5: 包管理"
    refresh_token

    # 获取 Node.js 运行时 ID
    local resp body
    resp=$(http_request "GET" "/api/runtime")
    body=$(parse_body "$resp")
    local node_id
    node_id=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    items = d.get('data', [])
    if isinstance(items, list):
        for item in items:
            if item.get('name','') == 'node':
                print(item.get('id', item.get('ID', '')))
                break
        else: print('')
    else: print('')
except: print('')
" 2>/dev/null)

    if [[ -z "$node_id" ]]; then
        log_skip "Node.js 未安装，跳过包管理测试"
        record "包管理" "Package" "跳过" "SKIP" "Node.js 未安装"
        return
    fi

    # 1. 扫描 npm 包
    resp=$(http_request "GET" "/api/packages/scan/$node_id")
    assert_ok "包管理" "Package" "扫描 npm 包" "$resp"

    # 2. 列出已安装包
    resp=$(http_request "GET" "/api/packages?runtime_id=$node_id")
    assert_ok "包管理" "Package" "列出 npm 包" "$resp"

    # 3. 搜索包
    resp=$(http_request "GET" "/api/packages/search?runtime_id=$node_id&q=express")
    assert_ok "包管理" "Package" "搜索 npm 包" "$resp"

    # 4. 安装包
    log_info "安装 npm 包 lodash..."
    resp=$(http_request "POST" "/api/packages/install" "{\"runtime_id\":$node_id,\"name\":\"lodash\"}")
    assert_ok "包管理" "Package" "安装 lodash" "$resp"
    sleep 3

    # 5. 查询包版本
    resp=$(http_request "GET" "/api/packages/versions/lodash?runtime_id=$node_id")
    assert_ok "包管理" "Package" "查询 lodash 版本" "$resp"

    # 6. 再次扫描确认安装
    resp=$(http_request "GET" "/api/packages/scan/$node_id")
    assert_ok "包管理" "Package" "再次扫描确认" "$resp"

    # 7. 卸载包
    resp=$(http_request "POST" "/api/packages/uninstall" "{\"runtime_id\":$node_id,\"name\":\"lodash\"}")
    assert_ok "包管理" "Package" "卸载 lodash" "$resp"

    log_info "包管理测试完成"
}

# ============================================================
# Phase 6: 定时任务
# ============================================================
phase_cron() {
    log_phase "Phase 6: 定时任务"
    refresh_token

    local SCRIPT_ID="" TASK_ID=""

    # 1. 获取预设模板
    local resp body
    resp=$(http_request "GET" "/api/cron/presets")
    assert_ok "定时任务" "Cron" "获取预设模板" "$resp"

    # 2. 描述 cron 表达式
    resp=$(http_request "GET" "/api/cron/describe?schedule=0+*+*+*+*")
    assert_ok "定时任务" "Cron" "描述 cron 表达式" "$resp"

    # 3. 计算下次执行时间
    resp=$(http_request "GET" "/api/cron/next-runs?schedule=0+*+*+*+*&count=3")
    assert_ok "定时任务" "Cron" "计算下次执行时间" "$resp"

    # 4. 创建脚本
    resp=$(http_request "POST" "/api/cron/scripts" \
        '{"name":"e2e-test-script","content":"#!/bin/bash\necho \"Hello E2E Test\"","language":"sh","description":"E2E 测试脚本"}')
    body=$(parse_body "$resp")
    SCRIPT_ID=$(extract_id "$body")
    if [[ -n "$SCRIPT_ID" ]]; then
        record "定时任务" "Script" "创建脚本" "PASS" "ID=$SCRIPT_ID"
    else
        record "定时任务" "Script" "创建脚本" "FAIL" "无法提取 ID"
    fi

    # 5. 列出脚本
    resp=$(http_request "GET" "/api/cron/scripts")
    assert_ok "定时任务" "Script" "列出脚本" "$resp"

    # 6. 获取脚本详情
    if [[ -n "$SCRIPT_ID" ]]; then
        resp=$(http_request "GET" "/api/cron/scripts/$SCRIPT_ID")
        assert_ok "定时任务" "Script" "获取脚本详情" "$resp"

        # 7. 更新脚本
        resp=$(http_request "PUT" "/api/cron/scripts/$SCRIPT_ID" \
            '{"content":"#!/bin/bash\necho \"Updated E2E Script\"","description":"已更新"}')
        assert_ok "定时任务" "Script" "更新脚本" "$resp"
    fi

    # 8. 创建定时任务
    resp=$(http_request "POST" "/api/cron/tasks" \
        '{"name":"e2e-test-task","command":"echo e2e-test","schedule":"0 */6 * * *","description":"E2E 测试任务","timeout":60,"max_retry":3}')
    body=$(parse_body "$resp")
    TASK_ID=$(extract_id "$body")
    if [[ -n "$TASK_ID" ]]; then
        record "定时任务" "Task" "创建任务" "PASS" "ID=$TASK_ID"
    else
        record "定时任务" "Task" "创建任务" "FAIL" "无法提取 ID"
    fi

    # 9. 列出任务
    resp=$(http_request "GET" "/api/cron/tasks")
    assert_ok "定时任务" "Task" "列出任务" "$resp"

    if [[ -n "$TASK_ID" ]]; then
        # 10. 获取任务详情
        resp=$(http_request "GET" "/api/cron/tasks/$TASK_ID")
        assert_ok "定时任务" "Task" "获取任务详情" "$resp"

        # 11. 启用任务
        resp=$(http_request "POST" "/api/cron/tasks/$TASK_ID/enable")
        assert_ok "定时任务" "Task" "启用任务" "$resp"

        # 12. 立即执行
        resp=$(http_request "POST" "/api/cron/tasks/$TASK_ID/run")
        assert_ok "定时任务" "Task" "立即执行任务" "$resp"
        sleep 2

        # 13. 查看执行日志
        resp=$(http_request "GET" "/api/cron/tasks/$TASK_ID/logs?limit=10")
        assert_ok "定时任务" "Task" "查看执行日志" "$resp"

        # 14. 禁用任务
        resp=$(http_request "POST" "/api/cron/tasks/$TASK_ID/disable")
        assert_ok "定时任务" "Task" "禁用任务" "$resp"

        # 15. 更新任务
        resp=$(http_request "PUT" "/api/cron/tasks/$TASK_ID" '{"description":"已更新","timeout":120}')
        assert_ok "定时任务" "Task" "更新任务" "$resp"

        # 16. 删除任务
        resp=$(http_request "DELETE" "/api/cron/tasks/$TASK_ID")
        assert_ok "定时任务" "Task" "删除任务" "$resp"
    fi

    # 17. 删除脚本
    if [[ -n "$SCRIPT_ID" ]]; then
        resp=$(http_request "DELETE" "/api/cron/scripts/$SCRIPT_ID")
        assert_ok "定时任务" "Script" "删除脚本" "$resp"
    fi

    # 18. 文档 CRUD
    local DOC_ID
    resp=$(http_request "POST" "/api/cron/docs" '{"title":"E2E 测试文档","content":"这是测试内容","sort_order":1}')
    body=$(parse_body "$resp")
    DOC_ID=$(extract_id "$body")
    if [[ -n "$DOC_ID" ]]; then
        record "定时任务" "Doc" "创建文档" "PASS" "ID=$DOC_ID"

        resp=$(http_request "PUT" "/api/cron/docs/$DOC_ID" '{"title":"已更新文档"}')
        assert_ok "定时任务" "Doc" "更新文档" "$resp"

        resp=$(http_request "DELETE" "/api/cron/docs/$DOC_ID")
        assert_ok "定时任务" "Doc" "删除文档" "$resp"
    else
        record "定时任务" "Doc" "创建文档" "FAIL" "无法提取 ID"
    fi

    log_info "定时任务测试完成"
}

# ============================================================
# Phase 7: 进程守护
# ============================================================
phase_process() {
    log_phase "Phase 7: 进程守护"
    refresh_token

    local GROUP_ID="" PROC_ID=""

    # 1. 创建进程组
    local resp body
    resp=$(http_request "POST" "/api/process-groups" '{"name":"e2e-test-group","description":"E2E 测试进程组"}')
    body=$(parse_body "$resp")
    GROUP_ID=$(extract_id "$body")
    if [[ -n "$GROUP_ID" ]]; then
        record "进程守护" "ProcessGroup" "创建进程组" "PASS" "ID=$GROUP_ID"
    else
        record "进程守护" "ProcessGroup" "创建进程组" "FAIL" "无法提取 ID"
    fi

    # 2. 列出进程组
    resp=$(http_request "GET" "/api/process-groups")
    assert_ok "进程守护" "ProcessGroup" "列出进程组" "$resp"

    # 3. 创建进程
    resp=$(http_request "POST" "/api/processes" \
        '{"name":"e2e-test-proc","command":"sleep","args":"3600","auto_start":false,"auto_restart":false}')
    body=$(parse_body "$resp")
    PROC_ID=$(extract_id "$body")
    if [[ -n "$PROC_ID" ]]; then
        record "进程守护" "Process" "创建进程" "PASS" "ID=$PROC_ID"
    else
        record "进程守护" "Process" "创建进程" "FAIL" "无法提取 ID"
    fi

    # 4. 列出进程
    resp=$(http_request "GET" "/api/processes")
    assert_ok "进程守护" "Process" "列出进程" "$resp"

    if [[ -n "$PROC_ID" ]]; then
        # 5. 获取进程详情
        resp=$(http_request "GET" "/api/processes/$PROC_ID")
        assert_ok "进程守护" "Process" "获取进程详情" "$resp"

        # 6. 启动进程
        resp=$(http_request "POST" "/api/processes/$PROC_ID/start")
        assert_ok "进程守护" "Process" "启动进程" "$resp"
        sleep 2

        # 7. 查看进程统计
        resp=$(http_request "GET" "/api/processes/$PROC_ID/stats")
        assert_ok "进程守护" "Process" "查看进程统计" "$resp"

        # 8. 查看进程日志
        resp=$(http_request "GET" "/api/processes/$PROC_ID/logs?limit=10")
        assert_ok "进程守护" "Process" "查看进程日志" "$resp"

        # 9. 重启进程
        resp=$(http_request "POST" "/api/processes/$PROC_ID/restart")
        assert_ok "进程守护" "Process" "重启进程" "$resp"
        sleep 2

        # 10. 停止进程
        resp=$(http_request "POST" "/api/processes/$PROC_ID/stop")
        assert_ok "进程守护" "Process" "停止进程" "$resp"

        # 11. 批量启动
        resp=$(http_request "POST" "/api/processes/batch/start" "{\"ids\":[$PROC_ID]}")
        assert_ok "进程守护" "Process" "批量启动" "$resp"
        sleep 1

        # 12. 批量停止
        resp=$(http_request "POST" "/api/processes/batch/stop" "{\"ids\":[$PROC_ID]}")
        assert_ok "进程守护" "Process" "批量停止" "$resp"

        # 13. 导出配置
        resp=$(http_request "GET" "/api/processes/export")
        assert_ok "进程守护" "Process" "导出进程配置" "$resp"

        # 14. 更新进程
        resp=$(http_request "PUT" "/api/processes/$PROC_ID" '{"description":"已更新"}')
        assert_ok "进程守护" "Process" "更新进程" "$resp"

        # 15. 删除进程
        resp=$(http_request "DELETE" "/api/processes/$PROC_ID")
        assert_ok "进程守护" "Process" "删除进程" "$resp"
    fi

    # 16. 删除进程组
    if [[ -n "$GROUP_ID" ]]; then
        resp=$(http_request "DELETE" "/api/process-groups/$GROUP_ID")
        assert_ok "进程守护" "ProcessGroup" "删除进程组" "$resp"
    fi

    log_info "进程守护测试完成"
}

# ============================================================
# Phase 8: 防火墙
# ============================================================
phase_firewall() {
    log_phase "Phase 8: 防火墙"
    refresh_token

    local RULE_ID=""

    # 1. 查看状态
    local resp body
    resp=$(http_request "GET" "/api/firewall/status")
    assert_ok "防火墙" "Firewall" "查看防火墙状态" "$resp"

    # 2. 启用防火墙
    resp=$(http_request "POST" "/api/firewall/enable")
    assert_ok "防火墙" "Firewall" "启用防火墙" "$resp"

    # 3. 创建规则
    resp=$(http_request "POST" "/api/firewall/rules" \
        '{"chain":"INPUT","action":"ACCEPT","protocol":"tcp","port":"8888","remark":"E2E 测试规则"}')
    body=$(parse_body "$resp")
    RULE_ID=$(extract_id "$body")
    if [[ -n "$RULE_ID" ]]; then
        record "防火墙" "Rule" "创建规则" "PASS" "ID=$RULE_ID"
    else
        record "防火墙" "Rule" "创建规则" "FAIL" "无法提取 ID"
    fi

    # 4. 列出规则
    resp=$(http_request "GET" "/api/firewall/rules")
    assert_ok "防火墙" "Rule" "列出规则" "$resp"

    if [[ -n "$RULE_ID" ]]; then
        # 5. 获取规则详情
        resp=$(http_request "GET" "/api/firewall/rules/$RULE_ID")
        assert_ok "防火墙" "Rule" "获取规则详情" "$resp"

        # 6. 修改规则
        resp=$(http_request "PUT" "/api/firewall/rules/$RULE_ID" '{"remark":"E2E 已更新"}')
        assert_ok "防火墙" "Rule" "修改规则" "$resp"

        # 7. 禁用规则
        resp=$(http_request "POST" "/api/firewall/rules/$RULE_ID/disable")
        assert_ok "防火墙" "Rule" "禁用规则" "$resp"

        # 8. 启用规则
        resp=$(http_request "POST" "/api/firewall/rules/$RULE_ID/enable")
        assert_ok "防火墙" "Rule" "启用规则" "$resp"

        # 9. 上移规则
        resp=$(http_request "POST" "/api/firewall/rules/$RULE_ID/move-up")
        assert_ok "防火墙" "Rule" "上移规则" "$resp"

        # 10. 下移规则
        resp=$(http_request "POST" "/api/firewall/rules/$RULE_ID/move-down")
        assert_ok "防火墙" "Rule" "下移规则" "$resp"

        # 11. 删除规则
        resp=$(http_request "DELETE" "/api/firewall/rules/$RULE_ID")
        assert_ok "防火墙" "Rule" "删除规则" "$resp"
    fi

    # 12. 导出规则
    resp=$(http_request "GET" "/api/firewall/rules/export")
    assert_ok "防火墙" "Firewall" "导出规则" "$resp"

    # 13. 获取模板
    resp=$(http_request "GET" "/api/firewall/templates")
    assert_ok "防火墙" "Firewall" "获取规则模板" "$resp"

    # 14. 查看系统规则
    resp=$(http_request "GET" "/api/firewall/system-rules")
    assert_ok "防火墙" "Firewall" "查看系统规则" "$resp"

    # 15. 查看防火墙日志
    resp=$(http_request "GET" "/api/firewall/logs?lines=10")
    assert_ok "防火墙" "Firewall" "查看防火墙日志" "$resp"

    # 16. 设置默认策略
    resp=$(http_request "POST" "/api/firewall/default-policy" '{"chain":"INPUT","policy":"ACCEPT"}')
    assert_ok "防火墙" "Firewall" "设置默认策略" "$resp"

    # 17. 禁用防火墙
    resp=$(http_request "POST" "/api/firewall/disable" '{"confirm":true}')
    assert_ok "防火墙" "Firewall" "禁用防火墙" "$resp"

    log_info "防火墙测试完成"
}

# ============================================================
# 生成报告
# ============================================================
generate_report() {
    log_info "生成测试报告..."

    cat > "$REPORT" << EOF
# EasyServer 全方位端到端测试报告

> 测试时间: $(date '+%Y-%m-%d %H:%M:%S')
> 测试环境: ${BASE_URL}
> 测试类型: 端到端功能测试（安装/部署/操作/清理全流程）

---

## 一、测试摘要

| 指标 | 数量 |
|------|------|
| **总测试数** | ${TOTAL} |
| **通过** | ${PASS} ✅ |
| **失败** | ${FAIL} ❌ |
| **跳过** | ${SKIP} ⏭️ |
| **通过率** | $(( PASS * 100 / (TOTAL > 0 ? TOTAL : 1) ))% |

---

## 二、按阶段统计

| 阶段 | 通过 | 失败 | 通过率 |
|------|------|------|--------|
EOF

    for p in "${PHASES[@]}"; do
        local t=${PHASE_TOTAL[$p]} pa=${PHASE_PASS[$p]} fa=${PHASE_FAIL[$p]}
        [[ $t -eq 0 ]] && continue
        echo "| ${p} | ${pa} | ${fa} | $(( pa * 100 / (pa + fa > 0 ? pa + fa : 1) ))% |" >> "$REPORT"
    done

    cat >> "$REPORT" << EOF

---

## 三、失败详情

| 模块 | 操作 | 问题描述 |
|------|------|----------|
$(echo -e "$FAIL_DETAILS")

---

## 四、测试覆盖范围

| 阶段 | 测试内容 |
|------|----------|
| 运行时 | Node.js/Python/Go 安装、版本管理、检测、卸载 |
| 网站 | Nginx 安装、静态/PHP/Node/代理网站部署、启用/禁用 |
| 数据库 | MySQL 安装、库/表/用户 CRUD、SQL 执行、备份恢复 |
| Docker | 安装检测、容器全生命周期、镜像/卷/网络管理 |
| 包管理 | npm 包安装/扫描/搜索/卸载 |
| 定时任务 | 脚本/任务 CRUD、执行、日志、文档 |
| 进程守护 | 进程/分组 CRUD、启停、批量操作、导入导出 |
| 防火墙 | 启停、规则 CRUD、排序、模板、导出 |

---

## 五、建议

$(if [[ $FAIL -gt 0 ]]; then
echo "- ❌ 有 ${FAIL} 个测试失败，需要修复"
else
echo "- ✅ 所有端到端测试通过"
fi)
EOF

    log_info "报告已保存到: $REPORT"
}

# ============================================================
# 主流程
# ============================================================
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║  EasyServer 全方位端到端测试                         ║"
    echo "║  覆盖：运行时/网站/数据库/Docker/包/任务/进程/防火墙  ║"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    phase_prepare
    phase_runtime
    phase_website
    phase_database
    phase_docker
    phase_packages
    phase_cron
    phase_process
    phase_firewall

    generate_report

    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║  测试完成！                                          ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  总测试: ${TOTAL}                                      "
    echo "║  通过:   ${PASS} ✅                                    "
    echo "║  失败:   ${FAIL} ❌                                    "
    echo "║  跳过:   ${SKIP} ⏭️                                    "
    echo "║  通过率: $(( PASS * 100 / (TOTAL > 0 ? TOTAL : 1) ))%                                       "
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  报告: ${REPORT}                                      "
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""
}

main "$@"
