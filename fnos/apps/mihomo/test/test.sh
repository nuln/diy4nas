#!/bin/sh
set -e
PASS=0
FAIL=0

log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== Mihomo Docker Test ==="

# 1. Start the Go server
echo "[1/6] Starting server..."
TRIM_PKGVAR="/var/apps/mihomo/data" TRIM_APPDEST="/app" /app/server &
SRV_PID=$!
sleep 2

# 2. Check server process is running
echo "[2/6] Checking server process..."
if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Server process running (pid $SRV_PID)"
else
    log_fail "Server process not running"
fi

# 3. Test API /api/status
echo "[3/6] Testing /api/status..."
STATUS=$(curl -sf http://127.0.0.1:9097/api/status 2>/dev/null || echo "")
if echo "$STATUS" | grep -q "running"; then
    log_pass "/api/status returns status"
    echo "     Status: $STATUS"
else
    log_fail "/api/status failed: $STATUS"
fi

# 4. Test API /api/log
echo "[4/6] Testing /api/log..."
LOG=$(curl -sf http://127.0.0.1:9097/api/log 2>/dev/null || echo "")
if echo "$LOG" | grep -q "log"; then
    log_pass "/api/log returns log data"
else
    log_fail "/api/log failed: $LOG"
fi

# 5. Test embedded UI
echo "[5/6] Testing UI..."
UI=$(curl -sf http://127.0.0.1:9097/ 2>/dev/null || echo "")
if echo "$UI" | grep -q "Mihomo"; then
    log_pass "UI returns HTML with Mihomo title"
else
    log_fail "UI failed"
fi

# 6. Test config API
echo "[6/6] Testing /api/config..."
CONFIG=$(curl -sf http://127.0.0.1:9097/api/config 2>/dev/null || echo "")
if echo "$CONFIG" | grep -q "mixed-port"; then
    log_pass "/api/config returns config with mixed-port"
else
    log_fail "/api/config failed: $CONFIG"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
