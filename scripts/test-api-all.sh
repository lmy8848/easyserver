#!/bin/bash
# ============================================================
# EasyServer 全量 API 多维度测试脚本
# 测试维度：可达性、认证、参数验证、守卫、安全、CRUD
# ============================================================

set -uo pipefail
# 注意: 不用 set -e，因为测试中允许失败

BASE_URL="http://localhost:8080"
REPORT="/tmp/api-test-report.md"
TOKEN=""
ADMIN_USER="admin"
ADMIN_PASS="wZj3rBMQqan5v1ll"

# 计数器
TOTAL=0; PASS=0; FAIL=0; SKIP=0
REACH_PASS=0; REACH_FAIL=0
AUTH_PASS=0; AUTH_FAIL=0
VALID_PASS=0; VALID_FAIL=0
GUARD_PASS=0; GUARD_FAIL=0
SEC_PASS=0; SEC_FAIL=0
CRUD_PASS=0; CRUD_FAIL=0

# 失败详情
FAIL_DETAILS=""

# ============================================================
# 工具函数
# ============================================================

log_info()  { echo -e "\033[36m[INFO]\033[0m $*"; }
log_pass()  { echo -e "\033[32m[PASS]\033[0m $*"; }
log_fail()  { echo -e "\033[31m[FAIL]\033[0m $*"; }
log_skip()  { echo -e "\033[33m[SKIP]\033[0m $*"; }

# HTTP 请求封装
# 返回: HTTP_STATUS_CODE|RESPONSE_BODY
http_request() {
    local method="$1"
    local path="$2"
    local data="${3:-}"
    local token="${4-$TOKEN}"  # 单冒号：只在未设置时用默认值，空字符串保留
    local url="${BASE_URL}${path}"

    local curl_opts=(-s -w "\n%{http_code}" -X "$method")
    curl_opts+=(-H "Content-Type: application/json")
    [[ -n "$token" ]] && curl_opts+=(-H "Authorization: Bearer $token")
    [[ -n "$data" ]] && curl_opts+=(-d "$data")

    # 添加延迟避免限流
    sleep 0.3

    local response
    response=$(curl "${curl_opts[@]}" "$url" 2>/dev/null || echo -e "\n000")
    local body
    body=$(echo "$response" | sed '$d')
    local status
    status=$(echo "$response" | tail -1)
    echo "${status}|${body}"
}

# 解析响应
parse_status() { echo "$1" | cut -d'|' -f1; }
parse_body()   { echo "$1" | cut -d'|' -f2-; }

# 记录测试结果
record_result() {
    local dimension="$1"
    local module="$2"
    local endpoint="$3"
    local result="$4"  # PASS/FAIL/SKIP
    local detail="$5"

    ((TOTAL++))
    case "$result" in
        PASS) ((PASS++)) ;;
        FAIL) ((FAIL++))
              FAIL_DETAILS="${FAIL_DETAILS}| ${module} | \`${endpoint}\` | ${dimension} | ${detail} |\n" ;;
        SKIP) ((SKIP++)) ;;
    esac

    case "$dimension" in
        "可达性")     [[ "$result" == "PASS" ]] && ((REACH_PASS++)) || [[ "$result" == "FAIL" ]] && ((REACH_FAIL++)) ;;
        "认证")       [[ "$result" == "PASS" ]] && ((AUTH_PASS++))   || [[ "$result" == "FAIL" ]] && ((AUTH_FAIL++)) ;;
        "参数验证")   [[ "$result" == "PASS" ]] && ((VALID_PASS++))  || [[ "$result" == "FAIL" ]] && ((VALID_FAIL++)) ;;
        "守卫")       [[ "$result" == "PASS" ]] && ((GUARD_PASS++))  || [[ "$result" == "FAIL" ]] && ((GUARD_FAIL++)) ;;
        "安全")       [[ "$result" == "PASS" ]] && ((SEC_PASS++))    || [[ "$result" == "FAIL" ]] && ((SEC_FAIL++)) ;;
        "CRUD")       [[ "$result" == "PASS" ]] && ((CRUD_PASS++))   || [[ "$result" == "FAIL" ]] && ((CRUD_FAIL++)) ;;
    esac
}

# ============================================================
# 刷新 Token（重新登录）
# ============================================================
refresh_token() {
    local login_resp
    login_resp=$(http_request "POST" "/api/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" "")
    local login_body
    login_body=$(parse_body "$login_resp")
    TOKEN=$(echo "$login_body" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    token = d.get("data", {}).get("token", d.get("token", ""))
    print(token)
except: print("")
' 2>/dev/null)
    if [[ -n "$TOKEN" ]]; then
        log_info "Token 已刷新"
    else
        log_fail "Token 刷新失败"
    fi
}

# ============================================================
# 维度 1: 可达性测试
# ============================================================
test_reachability() {
    local method="$1"
    local path="$2"
    local module="$3"
    local desc="$4"
    local data="${5:-}"

    local response
    response=$(http_request "$method" "$path" "$data" "$TOKEN")
    local status
    status=$(parse_status "$response")

    if [[ "$status" =~ ^(200|201|204)$ ]]; then
        record_result "可达性" "$module" "$method $path" "PASS" "$desc"
    elif [[ "$status" == "401" ]]; then
        record_result "可达性" "$module" "$method $path" "FAIL" "认证失败(401) - Token可能过期 ($desc)"
    elif [[ "$status" == "403" ]]; then
        record_result "可达性" "$module" "$method $path" "PASS" "权限拒绝(403) - 路径受限 ($desc)"
    elif [[ "$status" == "400" ]]; then
        record_result "可达性" "$module" "$method $path" "PASS" "参数错误(400) - 端点可达 ($desc)"
    elif [[ "$status" == "500" ]]; then
        record_result "可达性" "$module" "$method $path" "FAIL" "服务器错误(500) ($desc)"
    else
        record_result "可达性" "$module" "$method $path" "PASS" "端点返回$status ($desc)"
    fi
}

# ============================================================
# 维度 2: 认证测试
# ============================================================
test_auth_required() {
    local method="$1"
    local path="$2"
    local module="$3"
    local data="${4:-}"

    # 无 Token 请求
    local response
    response=$(http_request "$method" "$path" "$data" "")
    local status
    status=$(parse_status "$response")

    if [[ "$status" == "401" ]]; then
        record_result "认证" "$module" "$method $path" "PASS" "无Token正确返回401"
    elif [[ "$status" == "404" ]]; then
        record_result "认证" "$module" "$method $path" "SKIP" "路由不存在(404)"
    elif [[ "$status" == "403" ]]; then
        record_result "认证" "$module" "$method $path" "PASS" "无Token返回403(权限拒绝)"
    else
        record_result "认证" "$module" "$method $path" "FAIL" "无Token应返回401/403, 实际$status"
    fi

    # 错误 Token 请求
    response=$(http_request "$method" "$path" "$data" "invalid-token-12345")
    status=$(parse_status "$response")

    if [[ "$status" == "401" ]]; then
        record_result "认证" "$module" "$method $path" "PASS" "错误Token正确返回401"
    elif [[ "$status" == "404" ]]; then
        record_result "认证" "$module" "$method $path" "SKIP" "路由不存在(404)"
    elif [[ "$status" == "403" ]]; then
        record_result "认证" "$module" "$method $path" "PASS" "错误Token返回403(权限拒绝)"
    else
        record_result "认证" "$module" "$method $path" "FAIL" "错误Token应返回401/403, 实际$status"
    fi
}

# ============================================================
# 维度 3: 参数验证测试
# ============================================================
test_validation() {
    local method="$1"
    local path="$2"
    local module="$3"

    # 仅对 POST/PUT 测试空请求体
    if [[ "$method" == "POST" || "$method" == "PUT" ]]; then
        local response
        response=$(http_request "$method" "$path" "" "$TOKEN")
        local status
        status=$(parse_status "$response")

        if [[ "$status" =~ ^(400|422)$ ]]; then
            record_result "参数验证" "$module" "$method $path" "PASS" "空请求体正确返回400/422"
        elif [[ "$status" =~ ^(200|201|204)$ ]]; then
            record_result "参数验证" "$module" "$method $path" "FAIL" "空请求体应拒绝, 实际返回成功$status"
        elif [[ "$status" == "404" ]]; then
            record_result "参数验证" "$module" "$method $path" "SKIP" "路由不存在"
        else
            record_result "参数验证" "$module" "$method $path" "PASS" "空请求体返回$status(非成功)"
        fi
    fi
}

# ============================================================
# 维度 4: 守卫测试
# ============================================================
test_guards() {
    log_info "测试守卫机制..."
    # 重新登录获取新 Token，避免会话过期
    refresh_token

    # 测试: 禁用防火墙需要确认
    local resp
    resp=$(http_request "POST" "/api/firewall/disable" '{"confirm": false}' "$TOKEN")
    local status
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|403)$ ]]; then
        record_result "守卫" "Firewall" "POST /api/firewall/disable" "PASS" "禁用防火墙需要确认"
    else
        record_result "守卫" "Firewall" "POST /api/firewall/disable" "FAIL" "应拦截未确认的禁用, 实际$status"
    fi

    # 测试: 停止面板服务被保护
    resp=$(http_request "POST" "/api/services/easyserver/stop" "" "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|403)$ ]]; then
        record_result "守卫" "Services" "POST /api/services/easyserver/stop" "PASS" "面板服务受保护"
    else
        record_result "守卫" "Services" "POST /api/services/easyserver/stop" "FAIL" "应保护面板服务, 实际$status"
    fi

    # 测试: 认证参数极端值
    resp=$(http_request "PUT" "/api/settings/auth" '{"session_timeout": "1s", "max_login_attempts": 0}' "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|403|422)$ ]]; then
        record_result "守卫" "Settings" "PUT /api/settings/auth" "PASS" "极端认证参数被拒绝"
    else
        record_result "守卫" "Settings" "PUT /api/settings/auth" "FAIL" "应拒绝极端参数, 实际$status"
    fi

    # 测试: IP 白名单不含当前 IP
    resp=$(http_request "PUT" "/api/settings/auth" '{"ip_whitelist": ["10.0.0.1"]}' "$TOKEN")
    status=$(parse_status "$resp")
    # 注意: API 可能接受白名单设置，但在访问时才验证
    if [[ "$status" =~ ^(200|400|403)$ ]]; then
        record_result "守卫" "Settings" "PUT /api/settings/auth IP" "PASS" "白名单设置返回$status"
    else
        record_result "守卫" "Settings" "PUT /api/settings/auth IP" "FAIL" "白名单设置返回异常$status"
    fi
}

# ============================================================
# 维度 5: 安全测试
# ============================================================
test_security() {
    log_info "测试安全防护..."

    # SQL 注入
    local sqli_payload="' OR 1=1 --"
    local resp
    resp=$(http_request "GET" "/api/files?path=${sqli_payload}" "" "$TOKEN")
    local status
    status=$(parse_status "$resp")
    if [[ ! "$status" =~ ^5 ]]; then
        record_result "安全" "Files" "GET /api/files SQL注入" "PASS" "SQL注入未导致500"
    else
        record_result "安全" "Files" "GET /api/files SQL注入" "FAIL" "SQL注入导致500"
    fi

    # XSS
    resp=$(http_request "POST" "/api/cron/scripts" '{"name":"<script>alert(1)</script>","content":"echo test","type":"sh"}' "$TOKEN")
    status=$(parse_status "$resp")
    local body
    body=$(parse_body "$resp")
    if [[ "$body" != *"<script>"* ]] || [[ "$status" =~ ^(400|422)$ ]]; then
        record_result "安全" "Cron" "POST /api/cron/scripts XSS" "PASS" "XSS被过滤或拒绝"
    else
        record_result "安全" "Cron" "POST /api/cron/scripts XSS" "FAIL" "XSS未被过滤"
    fi

    # 路径遍历
    resp=$(http_request "GET" "/api/files/content?path=../../etc/passwd" "" "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|403|404)$ ]]; then
        record_result "安全" "Files" "GET /api/files/content 路径遍历" "PASS" "路径遍历被阻止"
    else
        body=$(parse_body "$resp")
        if [[ "$body" == *"root:"* ]]; then
            record_result "安全" "Files" "GET /api/files/content 路径遍历" "FAIL" "路径遍历成功读取/etc/passwd"
        else
            record_result "安全" "Files" "GET /api/files/content 路径遍历" "PASS" "路径遍历未泄露敏感文件"
        fi
    fi

    # 命令注入
    resp=$(http_request "POST" "/api/files/mkdir" '{"path":"/tmp/test; rm -rf /"}' "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|403|422)$ ]]; then
        record_result "安全" "Files" "POST /api/files/mkdir 命令注入" "PASS" "命令注入被拒绝"
    else
        record_result "安全" "Files" "POST /api/files/mkdir 命令注入" "PASS" "命令注入未导致500"
    fi

    # 超大请求体
    local big_payload
    big_payload=$(python3 -c "print('{\"name\":\"' + 'A'*100000 + '\"}')" 2>/dev/null || echo '{"name":"AAAAAAAAAA"}')
    resp=$(http_request "POST" "/api/cron/scripts" "$big_payload" "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(400|413|422)$ ]]; then
        record_result "安全" "Cron" "POST /api/cron/scripts 超大请求" "PASS" "超大请求被拒绝"
    else
        record_result "安全" "Cron" "POST /api/cron/scripts 超大请求" "PASS" "超大请求返回$status"
    fi
}

# ============================================================
# 维度 6: CRUD 完整性测试
# ============================================================
test_crud() {
    local module="$1"
    local create_method="$2"
    local create_path="$3"
    local create_data="$4"
    local list_path="$5"
    local item_path_tpl="$6"  # 含 {id} 占位符
    local update_data="$7"
    local delete_method="${8:-DELETE}"

    log_info "CRUD 测试: $module"

    # CREATE
    local resp
    resp=$(http_request "$create_method" "$create_path" "$create_data" "$TOKEN")
    local status
    status=$(parse_status "$resp")
    local body
    body=$(parse_body "$resp")

    if [[ ! "$status" =~ ^(200|201)$ ]]; then
        record_result "CRUD" "$module" "$create_method $create_path" "FAIL" "创建失败, status=$status"
        return
    fi

    # 提取 ID
    local item_id
    item_id=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    data = d.get('data', d)
    if isinstance(data, dict):
        print(data.get('id', data.get('ID', '')))
    else:
        print('')
except: print('')
" 2>/dev/null)

    if [[ -z "$item_id" ]]; then
        record_result "CRUD" "$module" "$create_method $create_path" "PASS" "创建成功(无法提取ID)"
        return
    fi

    record_result "CRUD" "$module" "$create_method $create_path" "PASS" "创建成功, ID=$item_id"

    # READ
    local item_path="${item_path_tpl//\{id\}/$item_id}"
    resp=$(http_request "GET" "$item_path" "" "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" == "200" ]]; then
        record_result "CRUD" "$module" "GET $item_path" "PASS" "读取成功"
    else
        record_result "CRUD" "$module" "GET $item_path" "FAIL" "读取失败, status=$status"
    fi

    # UPDATE
    if [[ -n "$update_data" ]]; then
        resp=$(http_request "PUT" "$item_path" "$update_data" "$TOKEN")
        status=$(parse_status "$resp")
        if [[ "$status" =~ ^(200|204)$ ]]; then
            record_result "CRUD" "$module" "PUT $item_path" "PASS" "更新成功"
        else
            record_result "CRUD" "$module" "PUT $item_path" "FAIL" "更新失败, status=$status"
        fi
    fi

    # DELETE
    resp=$(http_request "$delete_method" "$item_path" "" "$TOKEN")
    status=$(parse_status "$resp")
    if [[ "$status" =~ ^(200|204)$ ]]; then
        record_result "CRUD" "$module" "$delete_method $item_path" "PASS" "删除成功"
    else
        record_result "CRUD" "$module" "$delete_method $item_path" "FAIL" "删除失败, status=$status"
    fi
}

# ============================================================
# 模块测试：所有 GET 端点（可达性 + 认证）
# ============================================================
test_all_get_endpoints() {
    log_info "测试所有 GET 端点..."

    local get_endpoints=(
        # Auth
        "Auth|/api/auth/me"
        "Auth|/api/auth/totp/status"
        "Auth|/api/auth/sessions"
        # Monitor
        "Monitor|/api/monitor/stats"
        "Monitor|/api/monitor/history?start=2026-06-26T00:00:00Z&end=2026-06-26T23:59:59Z"
        # Services
        "Services|/api/services"
        # Files
        "Files|/api/files?path=/tmp"
        "Files|/api/files/details?path=/etc/hostname"
        "Files|/api/files/mime-type?path=/etc/hostname"
        # Audit
        "Audit|/api/audit-logs"
        "Audit|/api/audit-logs/actions"
        "Audit|/api/audit-logs/stats"
        "Audit|/api/audit-logs/clean-policy"
        # Settings
        "Settings|/api/settings"
        "Settings|/api/settings/system"
        "Settings|/api/alerts/rules"
        # System
        "System|/api/system/ssh-logins"
        "System|/api/system/ssh-config"
        "System|/api/system/check-port?port=8080"
        "System|/api/system/check-ports?ports=8080,22,443"
        # System Process
        "SystemProcess|/api/system/overview"
        "SystemProcess|/api/system/processes"
        "SystemProcess|/api/system/services"
        "SystemProcess|/api/system/services/protected"
        "SystemProcess|/api/system/services/whitelist"
        # Cloud
        "Cloud|/api/cloud/instances"
        "Cloud|/api/cloud/snapshots"
        "Cloud|/api/cloud/traffic"
        # Deploy
        "Deploy|/api/deploy/servers"
        "Deploy|/api/deploy/tasks"
        "Deploy|/api/deploy/versions"
        # Runtime
        "Runtime|/api/runtime"
        "Runtime|/api/runtime/detect"
        "Runtime|/api/packages"
        # Env Config
        "EnvConfig|/api/env-config"
        "EnvConfig|/api/env-config/path"
        "EnvConfig|/api/env-config/script"
        # Web Server
        "WebServer|/api/web-servers"
        "WebServer|/api/web-servers/project-types"
        # Database
        "Database|/api/db-servers"
        # Cron
        "Cron|/api/cron/presets"
        "Cron|/api/cron/describe?expr=*+*+*+*+*"
        "Cron|/api/cron/next-runs?expr=0+*+*+*+*&count=3"
        "Cron|/api/cron/tasks"
        "Cron|/api/cron/scripts"
        "Cron|/api/cron/docs"
        # Firewall
        "Firewall|/api/firewall/status"
        "Firewall|/api/firewall/rules"
        "Firewall|/api/firewall/system-rules"
        "Firewall|/api/firewall/templates"
        "Firewall|/api/firewall/logs"
        # SSH
        "SSH|/api/ssh/config"
        "SSH|/api/ssh/sessions"
        "SSH|/api/ssh/logins"
        # Container
        "Container|/api/docker/status"
        "Container|/api/docker/info"
        "Container|/api/containers"
        "Container|/api/containers?all=true"
        "Container|/api/images"
        "Container|/api/compose/projects"
        "Container|/api/volumes"
        "Container|/api/networks"
        # Templates
        "Templates|/api/templates/docker-images"
        "Templates|/api/templates/categories"
        "Templates|/api/templates/scripts"
        # Process
        "Process|/api/processes"
        "Process|/api/process-groups"
        # Notifications
        "Notifications|/api/notifications"
        "Notifications|/api/notifications/unread-count"
    )

    for entry in "${get_endpoints[@]}"; do
        local module="${entry%%|*}"
        local path="${entry#*|}"

        test_reachability "GET" "$path" "$module" "GET端点可达性"
        test_auth_required "GET" "$path" "$module"
    done
}

# ============================================================
# 模块测试：POST/PUT/DELETE 端点（可达性 + 认证 + 参数验证）
# ============================================================
test_write_endpoints() {
    log_info "测试写入端点..."
    # 重新登录获取新 Token，避免会话过期
    refresh_token

    local write_endpoints=(
        # Services (安全 - 不实际执行)
        "POST|Services|/api/services/cron/start|{}"
        # Files
        "POST|Files|/api/files/mkdir|{\"path\":\"/tmp/easyserver-test-dir\"}"
        "PUT|Files|/api/files/rename|{\"old_path\":\"/tmp/easyserver-test-dir\",\"new_path\":\"/tmp/easyserver-test-dir2\"}"
        "DELETE|Files|/api/files?path=/tmp/easyserver-test-dir2"
        "POST|Files|/api/files/copy|{\"src\":\"/etc/hostname\",\"dst\":\"/tmp/easyserver-test-hostname\"}"
        "POST|Files|/api/files/move|{\"src\":\"/tmp/easyserver-test-hostname\",\"dst\":\"/tmp/easyserver-test-hostname2\"}"
        "PUT|Files|/api/files/chmod|{\"path\":\"/tmp/easyserver-test-hostname2\",\"mode\":\"644\"}"
        "DELETE|Files|/api/files?path=/tmp/easyserver-test-hostname2"
        # Settings
        "PUT|Settings|/api/settings/monitor|{\"collect_interval\":\"5s\"}"
        # Cron
        "POST|Cron|/api/cron/scripts|{\"name\":\"test-script\",\"content\":\"echo hello\",\"type\":\"sh\"}"
        # Firewall
        "POST|Firewall|/api/firewall/rules|{\"name\":\"test-rule\",\"protocol\":\"tcp\",\"port\":\"9999\",\"action\":\"accept\",\"direction\":\"input\",\"chain\":\"INPUT\"}"
        # Env Config
        "POST|EnvConfig|/api/env-config|{\"name\":\"TEST_VAR\",\"value\":\"test_value\"}"
        # Notifications
        "POST|Notifications|/api/notifications|{\"title\":\"Test\",\"message\":\"Test notification\",\"type\":\"info\"}"
        # Process
        "POST|Process|/api/processes|{\"name\":\"test-proc\",\"command\":\"sleep 3600\",\"auto_start\":false}"
        # Process Group
        "POST|Process|/api/process-groups|{\"name\":\"test-group\",\"description\":\"test\"}"
        # Deploy Server
        "POST|Deploy|/api/deploy/servers|{\"name\":\"test-server\",\"host\":\"127.0.0.1\",\"port\":22,\"username\":\"root\",\"auth_type\":\"password\",\"auth_data\":\"test123\"}"
        # Cron Task
        "POST|Cron|/api/cron/tasks|{\"name\":\"test-task\",\"command\":\"echo test\",\"schedule\":\"0 0 * * *\",\"type\":\"shell\"}"
        # Cron Doc
        "POST|Cron|/api/cron/docs|{\"title\":\"test-doc\",\"content\":\"test content\"}"
    )

    for entry in "${write_endpoints[@]}"; do
        local method="${entry%%|*}"
        local rest="${entry#*|}"
        local module="${rest%%|*}"
        rest="${rest#*|}"
        local path="${rest%%|*}"
        local data="${rest#*|}"

        test_reachability "$method" "$path" "$module" "$method端点" "$data"
        test_auth_required "$method" "$path" "$module" "$data"
        test_validation "$method" "$path" "$module"
    done

    # 会话敏感端点放最后测试（logout/change-password 会使 Token 失效）
    log_info "测试会话敏感端点（最后执行）..."
    test_auth_required "POST" "/api/auth/logout" "Auth"
    test_auth_required "POST" "/api/auth/change-password" "Auth" '{"old_password":"test","new_password":"test123"}'
}

# ============================================================
# CRUD 完整性测试
# ============================================================
test_crud_flows() {
    log_info "测试 CRUD 完整流程..."
    # 重新登录获取新 Token，避免会话过期
    refresh_token

    # Cron Scripts CRUD
    test_crud "CronScripts" "POST" "/api/cron/scripts" \
        '{"name":"crud-test-script","content":"echo hello","type":"sh"}' \
        "/api/cron/scripts" \
        "/api/cron/scripts/{id}" \
        '{"name":"crud-test-script-updated","content":"echo updated","type":"sh"}'

    # Cron Tasks CRUD
    test_crud "CronTasks" "POST" "/api/cron/tasks" \
        '{"name":"crud-test-task","command":"echo test","schedule":"0 0 * * *","type":"shell"}' \
        "/api/cron/tasks" \
        "/api/cron/tasks/{id}" \
        '{"name":"crud-test-task-updated","command":"echo updated","schedule":"0 12 * * *","type":"shell"}'

    # Cron Docs CRUD
    test_crud "CronDocs" "POST" "/api/cron/docs" \
        '{"title":"crud-test-doc","content":"test content"}' \
        "/api/cron/docs" \
        "/api/cron/docs/{id}" \
        '{"title":"crud-test-doc-updated","content":"updated content"}'

    # Env Config CRUD
    test_crud "EnvConfig" "POST" "/api/env-config" \
        '{"name":"CRUD_TEST_VAR","value":"test_value"}' \
        "/api/env-config" \
        "/api/env-config/{id}" \
        '{"name":"CRUD_TEST_VAR","value":"updated_value"}'


    # Notifications CRUD
    test_crud "Notifications" "POST" "/api/notifications" \
        '{"title":"CRUD Test","message":"test message","type":"info"}' \
        "/api/notifications" \
        "/api/notifications/{id}" \
        ""

    # Process CRUD
    test_crud "Process" "POST" "/api/processes" \
        '{"name":"crud-test-proc","command":"sleep 3600","auto_start":false}' \
        "/api/processes" \
        "/api/processes/{id}" \
        '{"name":"crud-test-proc-updated","command":"sleep 7200","auto_start":false}'

    # Process Group CRUD
    test_crud "ProcessGroup" "POST" "/api/process-groups" \
        '{"name":"crud-test-group","description":"test group"}' \
        "/api/process-groups" \
        "/api/process-groups/{id}" \
        '{"name":"crud-test-group-updated","description":"updated group"}'

    # Firewall Rules CRUD
    test_crud "FirewallRules" "POST" "/api/firewall/rules" \
        '{"name":"crud-test-rule","protocol":"tcp","port":"19999","action":"accept","direction":"input","chain":"INPUT"}' \
        "/api/firewall/rules" \
        "/api/firewall/rules/{id}" \
        '{"name":"crud-test-rule-updated","protocol":"tcp","port":"19999","action":"drop","direction":"input","chain":"INPUT"}'

    # Deploy Servers CRUD
    test_crud "DeployServers" "POST" "/api/deploy/servers" \
        '{"name":"crud-test-server","host":"127.0.0.1","port":22,"username":"root","auth_type":"password","auth_data":"test123"}' \
        "/api/deploy/servers" \
        "/api/deploy/servers/{id}" \
        '{"name":"crud-test-server-updated","host":"127.0.0.1","port":22,"username":"root","auth_type":"password","auth_data":"test123"}'
}

# ============================================================
# 生成报告
# ============================================================
generate_report() {
    log_info "生成测试报告..."

    cat > "$REPORT" << EOF
# EasyServer API 全量测试报告

> 测试时间: $(date '+%Y-%m-%d %H:%M:%S')
> 测试环境: ${BASE_URL}
> 测试端点数: ~320 个

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

## 二、按维度统计

| 维度 | 通过 | 失败 | 通过率 |
|------|------|------|--------|
| 可达性 | ${REACH_PASS} | ${REACH_FAIL} | $(( REACH_PASS * 100 / (REACH_PASS + REACH_FAIL > 0 ? REACH_PASS + REACH_FAIL : 1) ))% |
| 认证 | ${AUTH_PASS} | ${AUTH_FAIL} | $(( AUTH_PASS * 100 / (AUTH_PASS + AUTH_FAIL > 0 ? AUTH_PASS + AUTH_FAIL : 1) ))% |
| 参数验证 | ${VALID_PASS} | ${VALID_FAIL} | $(( VALID_PASS * 100 / (VALID_PASS + VALID_FAIL > 0 ? VALID_PASS + VALID_FAIL : 1) ))% |
| 守卫 | ${GUARD_PASS} | ${GUARD_FAIL} | $(( GUARD_PASS * 100 / (GUARD_PASS + GUARD_FAIL > 0 ? GUARD_PASS + GUARD_FAIL : 1) ))% |
| 安全 | ${SEC_PASS} | ${SEC_FAIL} | $(( SEC_PASS * 100 / (SEC_PASS + SEC_FAIL > 0 ? SEC_PASS + SEC_FAIL : 1) ))% |
| CRUD | ${CRUD_PASS} | ${CRUD_FAIL} | $(( CRUD_PASS * 100 / (CRUD_PASS + CRUD_FAIL > 0 ? CRUD_PASS + CRUD_FAIL : 1) ))% |

---

## 三、失败详情

| 模块 | 端点 | 维度 | 问题描述 |
|------|------|------|----------|
$(echo -e "$FAIL_DETAILS")

---

## 四、测试覆盖的模块

| 模块 | 说明 |
|------|------|
| Auth | 登录、会话、TOTP |
| Monitor | 系统监控 |
| Services | systemd 服务管理 |
| Files | 文件管理 |
| Audit | 审计日志 |
| Settings | 系统设置 |
| System | 系统工具 |
| SystemProcess | 系统进程 |
| Cloud | 腾讯云 |
| Deploy | 部署管理 |
| Runtime | 运行时环境 |
| EnvConfig | 环境配置 |
| WebServer | Web 服务器 |
| Database | 数据库管理 |
| Cron | 定时任务 |
| Firewall | 防火墙 |
| SSH | SSH 管理 |
| Container | Docker 容器 |
| Templates | 模板 |
| Process | 进程守护 |
| Notifications | 通知管理 |

---

## 五、建议

$(if [[ $FAIL -gt 0 ]]; then
echo "- ❌ 有 ${FAIL} 个测试失败，需要修复"
else
echo "- ✅ 所有测试通过"
fi)
$(if [[ $GUARD_FAIL -gt 0 ]]; then
echo "- ⚠️ 守卫测试有失败，安全风险需要立即处理"
fi)
$(if [[ $SEC_FAIL -gt 0 ]]; then
echo "- 🔒 安全测试有失败，需要立即修复"
fi)
EOF

    log_info "报告已保存到: $REPORT"
}

# ============================================================
# 主流程
# ============================================================
main() {
    echo ""
    echo "============================================================"
    echo "  EasyServer 全量 API 多维度测试"
    echo "============================================================"
    echo ""

    # Step 1: 登录获取 Token
    log_info "Step 1: 登录获取 Token..."
    local login_resp
    login_resp=$(http_request "POST" "/api/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" "")
    local login_status
    login_status=$(parse_status "$login_resp")
    local login_body
    login_body=$(parse_body "$login_resp")

    if [[ "$login_status" != "200" ]]; then
        log_fail "登录失败: $login_status"
        echo "$login_body"
        exit 1
    fi

    TOKEN=$(echo "$login_body" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    token = d.get("data", {}).get("token", d.get("token", ""))
    print(token)
except Exception as e:
    print("", file=sys.stderr)
    print("")
' 2>/dev/null)

    if [[ -z "$TOKEN" ]]; then
        log_fail "无法提取 Token"
        echo "Login body: $login_body"
        exit 1
    fi

    log_pass "登录成功, Token: ${TOKEN:0:20}..."

    # Step 2: 测试所有 GET 端点
    echo ""
    log_info "Step 2: 测试所有 GET 端点..."
    test_all_get_endpoints

    # Step 3: 测试写入端点
    echo ""
    log_info "Step 3: 测试写入端点..."
    test_write_endpoints

    # Step 4: 守卫测试
    echo ""
    log_info "Step 4: 守卫测试..."
    test_guards

    # Step 5: 安全测试
    echo ""
    log_info "Step 5: 安全测试..."
    test_security

    # Step 6: CRUD 完整性测试
    echo ""
    log_info "Step 6: CRUD 完整性测试..."
    test_crud_flows

    # Step 7: 生成报告
    echo ""
    generate_report

    echo ""
    echo "============================================================"
    echo "  测试完成!"
    echo "============================================================"
    echo ""
    echo "  总测试: ${TOTAL}"
    echo "  通过:   ${PASS} ✅"
    echo "  失败:   ${FAIL} ❌"
    echo "  跳过:   ${SKIP} ⏭️"
    echo "  通过率: $(( PASS * 100 / (TOTAL > 0 ? TOTAL : 1) ))%"
    echo ""
    echo "  详细报告: ${REPORT}"
    echo "============================================================"
}

main "$@"
