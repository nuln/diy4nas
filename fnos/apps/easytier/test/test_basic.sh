#!/bin/sh
# Basic API test - tests Go server without any easytier binaries
set -e
PASS=0; FAIL=0
log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== EasyTier Basic API Test (No Binaries) ==="

# Start the Go server (no easytier-core or easytier-cli available)
echo "[1/8] Starting Go server..."
TRIM_PKGVAR="/tmp/easytier-test/data" TRIM_APPDEST="/tmp/easytier-test/app" TRIM_SERVICE_PORT="11215" /app/server > /tmp/easytier-test/server.log 2>&1 &
SRV_PID=$!
sleep 2

if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Server process running (pid $SRV_PID)"
else
    log_fail "Server failed to start"
    cat /tmp/easytier-test/server.log
    exit 1
fi

# Test 2: GET /api/status (should return empty status with no core)
echo "[2/8] Testing GET /api/status..."
STATUS=$(curl -sf http://127.0.0.1:11215/api/status 2>/dev/null || echo "")
if echo "$STATUS" | grep -q '"online":false' && echo "$STATUS" | grep -q '"settings"'; then
    log_pass "/api/status returns empty status with settings"
    echo "     Status: $STATUS"
else
    log_fail "/api/status failed: $STATUS"
fi

# Test 3: GET /api/settings (should return empty settings with default service_enabled=false)
echo "[3/8] Testing GET /api/settings..."
SETTINGS=$(curl -sf http://127.0.0.1:11215/api/settings 2>/dev/null || echo "")
if echo "$SETTINGS" | grep -q '"service_enabled":false'; then
    log_pass "/api/settings returns service_enabled=false (default)"
    echo "     Settings: $SETTINGS"
else
    log_fail "/api/settings failed: $SETTINGS"
fi

# Test 4: POST /api/settings (save new settings)
echo "[4/8] Testing POST /api/settings..."
POST_RESULT=$(curl -sf -X POST http://127.0.0.1:11215/api/settings \
    -H "Content-Type: application/json" \
    -d '{"service_enabled":true,"network_name":"testnet","network_secret":"test123","dhcp":true,"encryption":true}')
if echo "$POST_RESULT" | grep -q '"ok":true'; then
    log_pass "Settings save succeeded: $POST_RESULT"
else
    log_fail "Settings save failed: $POST_RESULT"
fi

# Test 5: Verify settings persisted
echo "[5/8] Verifying settings persistence..."
SETTINGS2=$(curl -sf http://127.0.0.1:11215/api/settings 2>/dev/null || echo "")
if echo "$SETTINGS2" | grep -q 'testnet' && echo "$SETTINGS2" | grep -q 'service_enabled":true'; then
    log_pass "Settings persisted to disk"
else
    log_fail "Settings not persisted: $SETTINGS2"
fi

# Test 6: Test service/start (should fail because no easytier-core)
echo "[6/8] Testing POST /api/service/start (no binary)..."
START_RESULT=$(curl -sf -X POST http://127.0.0.1:11215/api/service/start 2>/dev/null || echo "")
if echo "$START_RESULT" | grep -q '"ok":false'; then
    log_pass "Service start correctly fails without binary"
    echo "     Result: $START_RESULT"
else
    log_fail "Service start should have failed: $START_RESULT"
fi

# Test 7: GET /api/log
echo "[7/8] Testing GET /api/log..."
LOG=$(curl -sf http://127.0.0.1:11215/api/log 2>/dev/null || echo "")
if echo "$LOG" | grep -q 'log'; then
    log_pass "/api/log returns log data"
else
    log_fail "/api/log failed: $LOG"
fi

# Test 8: UI endpoint
echo "[8/8] Testing GET / (UI)..."
UI=$(curl -sf http://127.0.0.1:11215/ 2>/dev/null || echo "")
if echo "$UI" | grep -q "EasyTier"; then
    log_pass "UI serves HTML with EasyTier title"
else
    log_fail "UI failed"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true
sleep 1
rm -rf /tmp/easytier-test

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1