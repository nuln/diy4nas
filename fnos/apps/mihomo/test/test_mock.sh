#!/bin/sh
set -e
PASS=0; FAIL=0
log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== Mihomo Full Integration Test (with Mock) ==="

# The Go server will find and start /app/mihomo (the mock binary)
# which listens on :19090 simulating the mihomo API

# Start Go server - it will auto-start the mihomo mock
echo "[1/8] Starting Go management server (will start mihomo mock)..."
TRIM_PKGVAR="/var/apps/mihomo/data" TRIM_APPDEST="/app" /app/server &
SRV_PID=$!
sleep 3

if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Management server running (pid $SRV_PID)"
else
    log_fail "Management server failed to start"; exit 1
fi

# Verify mock mihomo API is accessible
echo "[2/8] Verifying mihomo API..."
VER=$(curl -sf http://127.0.0.1:19090/version 2>/dev/null || echo "")
if echo "$VER" | grep -q "1.19.27"; then
    log_pass "Mihomo API responding: $VER"
else
    log_fail "Mihomo API not responding: $VER"
fi

# Test /api/status
echo "[3/8] Testing /api/status..."
STATUS=$(curl -sf http://127.0.0.1:9097/api/status)
if echo "$STATUS" | grep -q '"running":true'; then
    log_pass "Status reports running: $STATUS"
else
    log_fail "Status failed: $STATUS"
fi

# Test /api/log
echo "[4/8] Testing /api/log..."
LOG=$(curl -sf http://127.0.0.1:9097/api/log)
if echo "$LOG" | grep -q "log"; then
    log_pass "Log endpoint works"
else
    log_fail "Log failed: $LOG"
fi

# Test /api/config (GET)
echo "[5/8] Testing /api/config GET..."
CONFIG=$(curl -sf http://127.0.0.1:9097/api/config)
if echo "$CONFIG" | grep -q "mixed-port"; then
    log_pass "Config contains mixed-port:7890"
else
    log_fail "Config failed: $CONFIG"
fi

# Test /api/config (POST - save new config)
echo "[6/8] Testing /api/config POST..."
POST_RESULT=$(curl -sf -X POST http://127.0.0.1:9097/api/config \
    -H "Content-Type: application/json" \
    -d '{"config":"mixed-port: 7890\nlog-level: debug\nmode: global\n"}')
if echo "$POST_RESULT" | grep -q "saved"; then
    log_pass "Config save succeeded: $POST_RESULT"
else
    log_fail "Config save failed: $POST_RESULT"
fi

# Verify the updated config
echo "[7/8] Verifying config update..."
CONFIG2=$(curl -sf http://127.0.0.1:9097/api/config)
if echo "$CONFIG2" | grep -q "log-level: debug"; then
    log_pass "Config update persisted correctly"
else
    log_fail "Config update not persisted: $CONFIG2"
fi

# Test UI
echo "[8/8] Testing UI..."
UI=$(curl -sf http://127.0.0.1:9097/)
if echo "$UI" | grep -q "Mihomo"; then
    log_pass "UI serves HTML with Mihomo title"
else
    log_fail "UI failed"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true
pkill -x mihomo 2>/dev/null || true

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
