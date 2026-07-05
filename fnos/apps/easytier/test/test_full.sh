#!/bin/sh
# Full integration test with REAL easytier-core and easytier-cli
set -e
PASS=0; FAIL=0
log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== EasyTier Full Integration Test (with Real Binaries) ==="

# Create test data directory
mkdir -p /tmp/easytier-full-test/data
mkdir -p /tmp/easytier-full-test/app

# Copy real binaries
cp /app/easytier-core /tmp/easytier-full-test/app/easytier-core
cp /app/easytier-cli /tmp/easytier-full-test/app/easytier-cli
chmod +x /tmp/easytier-full-test/app/easytier-core
chmod +x /tmp/easytier-full-test/app/easytier-cli

# Start the Go server
echo "[1/10] Starting Go server (with real easytier-core)..."
TRIM_PKGVAR="/tmp/easytier-full-test/data" TRIM_APPDEST="/tmp/easytier-full-test/app" TRIM_SERVICE_PORT="11217" /app/server > /tmp/easytier-full-test/server.log 2>&1 &
SRV_PID=$!
sleep 2

if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Server process running (pid $SRV_PID)"
else
    log_fail "Server failed to start"
    cat /tmp/easytier-full-test/server.log
    exit 1
fi

# Test 2: Save settings
echo "[2/10] Saving settings with full config..."
POST_RESULT=$(curl -sf -X POST http://127.0.0.1:11217/api/settings \
    -H "Content-Type: application/json" \
    -d '{"network_name":"testnet","network_secret":"testpass","dhcp":true}')
if echo "$POST_RESULT" | grep -q '"ok":true'; then
    log_pass "Settings save succeeded"
else
    log_fail "Settings save failed: $POST_RESULT"
fi

# Test 3: Start service
echo "[3/10] Starting service..."
START_RESULT=$(curl -sf -X POST http://127.0.0.1:11217/api/service/start 2>/dev/null || echo "")
if echo "$START_RESULT" | grep -q '"ok":true'; then
    log_pass "Service start succeeded"
    sleep 3
else
    log_fail "Service start failed: $START_RESULT"
fi

# Test 4: Verify easytier-core is running
echo "[4/10] Checking easytier-core process..."
if pgrep -x "easytier-core" > /dev/null 2>&1; then
    log_pass "easytier-core is running"
else
    log_fail "easytier-core is not running"
fi

# Test 5: Verify status
echo "[5/10] Testing /api/status..."
sleep 2
STATUS=$(curl -sf http://127.0.0.1:11217/api/status 2>/dev/null || echo "")
if echo "$STATUS" | grep -q '"running":true'; then
    log_pass "Service reports running"
else
    log_fail "Service not running: $STATUS"
fi

# Test 6: Test node info via CLI proxy
echo "[6/10] Testing easytier-cli node info..."
sleep 2
NODE=$(/tmp/easytier-full-test/app/easytier-cli node info -o json 2>&1 || echo "")
if echo "$NODE" | grep -q "hostname\|peer_id\|version" 2>/dev/null; then
    log_pass "easytier-cli node info works"
    echo "     Info: $NODE"
else
    log_fail "easytier-cli node info failed: $NODE"
fi

# Test 7: Test peer list via API
echo "[7/10] Testing /api/peers..."
PEERS=$(curl -sf http://127.0.0.1:11217/api/peers 2>/dev/null || echo "")
if [ -n "$PEERS" ]; then
    log_pass "/api/peers returns data: $PEERS"
else
    log_fail "/api/peers failed"
fi

# Test 8: Test route list via API
echo "[8/10] Testing /api/routes..."
ROUTES=$(curl -sf http://127.0.0.1:11217/api/routes 2>/dev/null || echo "")
if [ -n "$ROUTES" ]; then
    log_pass "/api/routes returns data"
else
    log_fail "/api/routes failed"
fi

# Test 9: Test UI endpoint
echo "[9/10] Testing UI..."
UI=$(curl -sf http://127.0.0.1:11217/ 2>/dev/null || echo "")
if echo "$UI" | grep -q "EasyTier"; then
    log_pass "UI serves HTML"
else
    log_fail "UI failed"
fi

# Test 10: Stop service
echo "[10/10] Stopping service..."
STOP_RESULT=$(curl -sf -X POST http://127.0.0.1:11217/api/service/stop 2>/dev/null || echo "")
if echo "$STOP_RESULT" | grep -q '"ok":true'; then
    log_pass "Service stop succeeded"
    sleep 2
else
    log_fail "Service stop failed"
fi

# Verify easytier-core stopped
if ! pgrep -x "easytier-core" > /dev/null 2>&1; then
    log_pass "easytier-core is no longer running"
else
    log_fail "easytier-core is still running"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true
pkill -x easytier-core 2>/dev/null || true
sleep 1
rm -rf /tmp/easytier-full-test

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1