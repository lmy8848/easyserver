#!/bin/bash
# ============================================================
# EasyServer 错误响应格式测试
# 验证各种错误场景的响应格式是否符合规范
# ============================================================

set -uo pipefail

BASE_URL="http://localhost:8080"
TOKEN=""
ADMIN_USER="admin"
ADMIN_PASS="${ADMIN_PASS:-Aan6vRfSvLDBwccF}"

# 计数器
TOTAL=0; PASS=0; FAIL=0

# 颜色输出
log_info()  { echo -e "\033[36m[INFO]\033[0m $*"; }
log_pass()  { echo -e "\033[32m[PASS]\033[0m $*"; }
log_fail()  { echo -e "\033[31m[FAIL]\033[0m $*"; }
log_test()  { echo -e "\033[1;33m[TEST]\033[0m $*"; }

# HTTP 请求
http_request() {
    local method="$1" path="$2" data="${3:-}" token="${4-$TOKEN}"
    local url="${BASE_URL}${path}"
    local curl_opts=(-s -w "\n%{http_code}" -X "$method" -H "Content-Type: application/json")
    [[ -n "$token" ]] && curl_opts+=(-H "Authorization: Bearer $token")
    [[ -n "$data" ]] && curl_opts+=(-d "$data")
    sleep 0.1
    local response
    response=$(curl "${curl_opts[@]}" "$url" 2>/dev/null || echo -e "\n000")
    local body status
    body=$(echo "$response" | sed '$d')
    status=$(echo "$response" | tail -1)
    echo "${status}|${body}"
}

# 验证错误响应格式
verify_error() {
    local desc="$1" resp="$2" expected_code="$3" expected_http="$4"
    local body status json_code message
    body=$(echo "$resp" | cut -d'|' -f2-)
    status=$(echo "$resp" | cut -d'|' -f1)

    ((TOTAL++))

    # 检查是否是有效JSON
    if ! echo "$body" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
        log_fail "$desc - 响应不是有效JSON: ${body:0:100}"
        ((FAIL++))
        return 1
    fi

    # 提取字段
    json_code=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)
    message=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('message',''))" 2>/dev/null)

    # 验证HTTP状态码
    if [[ -n "$expected_http" && "$status" != "$expected_http" ]]; then
        log_fail "$desc - HTTP状态码: 期望$expected_http, 实际$status"
        ((FAIL++))
        return 1
    fi

    # 验证JSON code
    if [[ -n "$expected_code" && "$json_code" != "$expected_code" ]]; then
        log_fail "$desc - JSON code: 期望$expected_code, 实际$json_code"
        ((FAIL++))
        return 1
    fi

    # 验证message字段存在且非空
    if [[ -z "$message" ]]; then
        log_fail "$desc - message字段为空"
        ((FAIL++))
        return 1
    fi

    log_pass "$desc - HTTP=$status, code=$json_code, message=$message"
    ((PASS++))
    return 0
}

# ============================================================
# 登录获取Token
# ============================================================
login() {
    local resp body
    resp=$(http_request "POST" "/api/auth/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" "")
    body=$(echo "$resp" | cut -d'|' -f2-)
    TOKEN=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('token',''))" 2>/dev/null)
    if [[ -n "$TOKEN" ]]; then
        log_info "登录成功"
    else
        log_fail "登录失败"
        exit 1
    fi
}

# ============================================================
# 测试用例
# ============================================================

test_auth_errors() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  认证错误测试"
    echo "════════════════════════════════════════"

    # 1. 无Token访问受保护接口
    log_test "无Token访问受保护接口"
    local resp
    resp=$(http_request "GET" "/api/runtime" "" "")
    verify_error "无Token" "$resp" "40101" "401"

    # 2. 无效Token
    log_test "无效Token"
    resp=$(http_request "GET" "/api/runtime" "" "invalid-token-12345")
    verify_error "无效Token" "$resp" "40101" "401"

    # 3. 错误密码登录
    log_test "错误密码登录"
    resp=$(http_request "POST" "/api/auth/login" '{"username":"admin","password":"wrong-password"}' "")
    verify_error "错误密码" "$resp" "40100" "401"

    # 4. 不存在的用户
    log_test "不存在的用户"
    resp=$(http_request "POST" "/api/auth/login" '{"username":"nonexistent","password":"test123"}' "")
    verify_error "不存在用户" "$resp" "40100" "401"
}

test_not_found_errors() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  资源不存在测试"
    echo "════════════════════════════════════════"

    local resp

    # 1. 不存在的WebServer
    log_test "不存在的WebServer"
    resp=$(http_request "GET" "/api/web-servers/99999")
    verify_error "WebServer不存在" "$resp" "40400" "404"

    # 2. 不存在的数据库
    log_test "不存在的数据库"
    resp=$(http_request "GET" "/api/db-servers/99999/databases")
    verify_error "数据库不存在" "$resp" "40400" "404"

    # 3. 不存在的进程
    log_test "不存在的进程"
    resp=$(http_request "GET" "/api/processes/99999")
    verify_error "进程不存在" "$resp" "40400" "404"

    # 4. 不存在的定时任务
    log_test "不存在的定时任务"
    resp=$(http_request "GET" "/api/cron/tasks/99999")
    verify_error "任务不存在" "$resp" "40400" "404"

    # 5. 不存在的防火墙规则
    log_test "不存在的防火墙规则"
    resp=$(http_request "GET" "/api/firewall/rules/99999")
    verify_error "规则不存在" "$resp" "40400" "404"

    # 6. 不存在的路由
    log_test "不存在的路由"
    resp=$(http_request "GET" "/api/nonexistent/endpoint")
    verify_error "路由不存在" "$resp" "40400" "404"
}

test_bad_request_errors() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  请求参数错误测试"
    echo "════════════════════════════════════════"

    local resp

    # 1. 缺少必填字段
    log_test "创建进程缺少command"
    resp=$(http_request "POST" "/api/processes" '{"name":"test"}')
    verify_error "缺少command" "$resp" "40000" "400"

    # 2. 无效JSON格式
    log_test "无效JSON格式"
    resp=$(http_request "POST" "/api/processes" 'invalid-json')
    verify_error "无效JSON" "$resp" "40000" "400"

    # 3. 类型错误
    log_test "类型错误"
    resp=$(http_request "POST" "/api/processes" '{"name":123,"command":"echo test"}')
    verify_error "类型错误" "$resp" "40000" "400"

    # 4. 无效的数据库ID
    log_test "无效的数据库ID"
    resp=$(http_request "GET" "/api/db-servers/abc/databases")
    verify_error "无效ID" "$resp" "40000" "400"

    # 5. 无效的cron表达式
    log_test "无效的cron表达式"
    resp=$(http_request "GET" "/api/cron/describe?schedule=invalid")
    verify_error "无效cron" "$resp" "40000" "400"
}

test_conflict_errors() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  资源冲突测试"
    echo "════════════════════════════════════════"

    local resp

    # 1. 创建重复进程名
    log_test "创建重复进程名"
    resp=$(http_request "POST" "/api/processes" '{"name":"test-conflict","command":"echo test","auto_start":false}')
    local first_code
    first_code=$(echo "$resp" | cut -d'|' -f2- | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)

    if [[ "$first_code" == "0" ]]; then
        resp=$(http_request "POST" "/api/processes" '{"name":"test-conflict","command":"echo test","auto_start":false}')
        verify_error "重复进程名" "$resp" "40900" "409"

        # 清理
        local proc_id
        proc_id=$(http_request "GET" "/api/processes" | cut -d'|' -f2- | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', [])
for item in items:
    if item.get('name') == 'test-conflict':
        print(item.get('id', ''))
        break
" 2>/dev/null)
        [[ -n "$proc_id" ]] && http_request "DELETE" "/api/processes/$proc_id" > /dev/null 2>&1
    else
        log_info "首次创建失败(code=$first_code)，跳过重复测试"
    fi

    # 2. 创建重复进程组名
    log_test "创建重复进程组名"
    resp=$(http_request "POST" "/api/process-groups" '{"name":"test-group-conflict","description":"test"}')
    first_code=$(echo "$resp" | cut -d'|' -f2- | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code',''))" 2>/dev/null)

    if [[ "$first_code" == "0" ]]; then
        resp=$(http_request "POST" "/api/process-groups" '{"name":"test-group-conflict","description":"test"}')
        verify_error "重复进程组名" "$resp" "40900" "409"

        # 清理
        local pg_id
        pg_id=$(http_request "GET" "/api/process-groups" | cut -d'|' -f2- | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', [])
for item in items:
    if item.get('name') == 'test-group-conflict':
        print(item.get('id', ''))
        break
" 2>/dev/null)
        [[ -n "$pg_id" ]] && http_request "DELETE" "/api/process-groups/$pg_id" > /dev/null 2>&1
    else
        log_info "首次创建失败(code=$first_code)，跳过重复测试"
    fi
}

test_method_not_allowed() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  HTTP方法错误测试"
    echo "════════════════════════════════════════"

    local resp

    # 1. 用GET访问POST接口
    log_test "用GET访问POST接口"
    resp=$(http_request "GET" "/api/auth/login")
    local status
    status=$(echo "$resp" | cut -d'|' -f1)
    if [[ "$status" == "405" || "$status" == "404" ]]; then
        log_pass "方法不允许 - HTTP $status"
        ((PASS++))
    else
        log_fail "方法不允许 - 期望405/404, 实际$status"
        ((FAIL++))
    fi
    ((TOTAL++))

    # 2. 用POST访问GET接口
    log_test "用POST访问GET接口"
    resp=$(http_request "POST" "/api/runtime" '{}')
    status=$(echo "$resp" | cut -d'|' -f1)
    if [[ "$status" == "405" || "$status" == "404" ]]; then
        log_pass "方法不允许 - HTTP $status"
        ((PASS++))
    else
        log_fail "方法不允许 - 期望405/404, 实际$status"
        ((FAIL++))
    fi
    ((TOTAL++))
}

test_guard_errors() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  守卫错误测试"
    echo "════════════════════════════════════════"

    local resp

    # 1. 修改自己的密码（应该成功）
    log_test "修改密码缺少必填字段"
    resp=$(http_request "POST" "/api/auth/change-password" '{"old_password":"test"}')
    verify_error "缺少新密码" "$resp" "40000" "400"

    # 2. 停止不存在的进程
    log_test "停止不存在的进程"
    resp=$(http_request "POST" "/api/processes/99999/stop")
    verify_error "停止不存在进程" "$resp" "40400" "404"

    # 3. 删除正在运行的进程（如果有的话）
    log_test "操作不存在的容器"
    resp=$(http_request "POST" "/api/containers/nonexistent/start")
    verify_error "不存在容器" "$resp" "40400" "404"
}

test_response_structure() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  响应结构测试"
    echo "════════════════════════════════════════"

    local resp body

    # 1. 成功响应结构
    log_test "成功响应结构"
    resp=$(http_request "GET" "/api/runtime")
    body=$(echo "$resp" | cut -d'|' -f2-)

    local has_code has_message has_data
    has_code=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print('code' in d)" 2>/dev/null)
    has_message=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print('message' in d)" 2>/dev/null)
    has_data=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print('data' in d)" 2>/dev/null)

    if [[ "$has_code" == "True" && "$has_message" == "True" && "$has_data" == "True" ]]; then
        log_pass "成功响应包含 code/message/data"
        ((PASS++))
    else
        log_fail "成功响应缺少字段: code=$has_code, message=$has_message, data=$has_data"
        ((FAIL++))
    fi
    ((TOTAL++))

    # 2. 错误响应结构
    log_test "错误响应结构"
    resp=$(http_request "GET" "/api/web-servers/99999")
    body=$(echo "$resp" | cut -d'|' -f2-)

    has_code=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print('code' in d)" 2>/dev/null)
    has_message=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print('message' in d)" 2>/dev/null)

    if [[ "$has_code" == "True" && "$has_message" == "True" ]]; then
        log_pass "错误响应包含 code/message"
        ((PASS++))
    else
        log_fail "错误响应缺少字段: code=$has_code, message=$has_message"
        ((FAIL++))
    fi
    ((TOTAL++))

    # 3. code=0时data不为null
    log_test "成功时data不为null"
    resp=$(http_request "GET" "/api/runtime")
    body=$(echo "$resp" | cut -d'|' -f2-)
    local data_null
    data_null=$(echo "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data') is None)" 2>/dev/null)

    if [[ "$data_null" == "False" ]]; then
        log_pass "成功时data不为null"
        ((PASS++))
    else
        log_fail "成功时data为null"
        ((FAIL++))
    fi
    ((TOTAL++))
}

# ============================================================
# 主流程
# ============================================================
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║  EasyServer 错误响应格式测试                         ║"
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""

    login

    test_auth_errors
    test_not_found_errors
    test_bad_request_errors
    test_conflict_errors
    test_method_not_allowed
    test_guard_errors
    test_response_structure

    echo ""
    echo "╔══════════════════════════════════════════════════════╗"
    echo "║  测试完成！                                          ║"
    echo "╠══════════════════════════════════════════════════════╣"
    echo "║  总测试: ${TOTAL}                                      "
    echo "║  通过:   ${PASS} ✅                                    "
    echo "║  失败:   ${FAIL} ❌                                    "
    echo "║  通过率: $(( PASS * 100 / (TOTAL > 0 ? TOTAL : 1) ))%                                       "
    echo "╚══════════════════════════════════════════════════════╝"
    echo ""
}

main "$@"
