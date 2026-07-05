#!/bin/bash
set -e
PASS=0; FAIL=0

pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

export TRIM_PKGVAR=/var/apps/easytier/data
export TRIM_APPDEST=/var/apps/easytier
export TRIM_SERVICE_PORT=11210

echo "===== EasyTier Docker Test ====="

echo "--- 1. 目录结构 ---"
[ -d /var/apps/easytier/cmd ] && pass "cmd/ 存在" || fail "cmd/ 缺失"
[ -d /var/apps/easytier/app ] && pass "app/ 存在" || fail "app/ 缺失"
[ -f /var/apps/easytier/manifest ] && pass "manifest 存在" || fail "manifest 缺失"
[ -f /var/apps/easytier/cmd/main ] && pass "cmd/main 存在" || fail "cmd/main 缺失"

echo "--- 2. 二进制文件 ---"
ls -la /var/apps/easytier/app/server 2>/dev/null && pass "Go server 存在" || fail "Go server 缺失"
ls -la /var/apps/easytier/app/easytier-core 2>/dev/null && pass "easytier-core 存在" || fail "easytier-core 缺失"
ls -la /var/apps/easytier/app/easytier-cli 2>/dev/null && pass "easytier-cli 存在" || fail "easytier-cli 缺失"
ls -la /var/apps/easytier/app/easytier-web-embed 2>/dev/null && pass "easytier-web-embed 存在" || fail "easytier-web-embed 缺失"

echo "--- 3. 脚本语法 ---"
bash -n /var/apps/easytier/cmd/main && pass "cmd/main 语法正确" || fail "cmd/main 语法错误"
bash -n /var/apps/easytier/cmd/service-setup 2>/dev/null && pass "service-setup 语法正确" || fail "service-setup 语法错误"
for s in /var/apps/easytier/cmd/*; do
    bash -n "$s" 2>/dev/null || fail "$(basename $s) 语法错误"
done && pass "所有 cmd 脚本语法正确"

echo "--- 4. CGI 脚本 ---"
for c in /var/apps/easytier/app/ui/*.cgi; do
    [ -f "$c" ] && chmod +x "$c" 2>/dev/null || true
done
[ -f /var/apps/easytier/app/ui/index.cgi ] && pass "index.cgi 存在" || fail "index.cgi 缺失"
[ -f /var/apps/easytier/app/ui/api.cgi ] && pass "api.cgi 存在" || fail "api.cgi 缺失"

echo "--- 5. 启动 Go server ---"
mkdir -p /var/apps/easytier/data
/var/apps/easytier/app/server &
SRV_PID=$!
echo "  server PID: $SRV_PID"

# Wait for server
for i in $(seq 1 15); do
    if curl -sf http://127.0.0.1:11210/api/traffic >/dev/null 2>&1; then
        pass "Go server 启动成功 (port 11210)"
        break
    fi
    sleep 1
done
[ $i -eq 15 ] && fail "Go server 启动超时"

echo "--- 6. REST API 测试 ---"
API="http://127.0.0.1:11210/api"

# Status
ST=$(curl -sf "$API/status" 2>/dev/null || echo '{}')
echo "$ST" | jq -e '.online != null' >/dev/null 2>&1 && pass "/api/status 返回正常" || fail "/api/status 返回异常"
echo "$ST" | jq -e '.daemon == true' >/dev/null 2>&1 && pass "  daemon=true" || fail "  daemon!=true"

# Traffic
TR=$(curl -sf "$API/traffic" 2>/dev/null || echo '{}')
echo "$TR" | jq -e '.rx != null' >/dev/null 2>&1 && pass "/api/traffic 返回正常" || fail "/api/traffic 返回异常"

# Peers
PE=$(curl -sf "$API/peers" 2>/dev/null || echo '[]')
echo "$PE" | jq -e 'type == "array"' >/dev/null 2>&1 && pass "/api/peers 返回数组" || fail "/api/peers 返回异常"

# Routes
RO=$(curl -sf "$API/routes" 2>/dev/null || echo '[]')
echo "$RO" | jq -e 'type == "array"' >/dev/null 2>&1 && pass "/api/routes 返回数组" || fail "/api/routes 返回异常"

# Connectors
CO=$(curl -sf "$API/connectors" 2>/dev/null || echo '[]')
echo "$CO" | jq -e 'type == "array"' >/dev/null 2>&1 && pass "/api/connectors 返回数组" || fail "/api/connectors 返回异常"

# Log
LG=$(curl -sf "$API/log" 2>/dev/null || echo '{}')
echo "$LG" | jq -e '.log != null' >/dev/null 2>&1 && pass "/api/log 返回正常" || fail "/api/log 返回异常"

# UI
UI=$(curl -sf "http://127.0.0.1:11210/" 2>/dev/null || echo '')
echo "$UI" | grep -q "EasyTier" && pass "UI 页面返回正常" || fail "UI 页面返回异常"

# vis-network
VN=$(curl -sf "http://127.0.0.1:11210/vis-network.min.js" 2>/dev/null || echo '')
echo "$VN" | grep -q "vis" && pass "vis-network.min.js 可访问" || fail "vis-network.min.js 不可访问"

echo "--- 7. 端口文件 ---"
[ -f /tmp/easytier-port ] && pass "端口文件存在" || fail "端口文件缺失"
PORT=$(cat /tmp/easytier-port)
[ "$PORT" = "11210" ] && pass "端口正确 (11210)" || fail "端口不正确 ($PORT)"

echo "--- 8. 停止服务 ---"
kill $SRV_PID 2>/dev/null && pass "服务停止成功" || fail "服务停止失败"
wait $SRV_PID 2>/dev/null || true
sleep 1
! kill -0 $SRV_PID 2>/dev/null && pass "进程已终止" || fail "进程仍在运行"

echo ""
echo "=========== 测试结果 ==========="
echo "  ✅ 通过: $PASS"
echo "  ❌ 失败: $FAIL"
echo "================================"
[ "$FAIL" -eq 0 ] && echo "  合格" || echo "  不合格"
exit $FAIL
