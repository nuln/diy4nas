#!/bin/sh
# Full integration test with mock easytier-cli and mock easytier-core
set -e
PASS=0; FAIL=0
log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== EasyTier Full Integration Test (with Mocks) ==="

# Create test data directory
mkdir -p /tmp/easytier-mock-test/data
mkdir -p /tmp/easytier-mock-test/app

# Copy mocks
cp /app/mock-cli.sh /tmp/easytier-mock-test/app/easytier-cli
cp /app/mock-core.sh /tmp/easytier-mock-test/app/easytier-core
chmod +x /tmp/easytier-mock-test/app/easytier-cli
chmod +x /tmp/easytier-mock-test/app/easytier-core

# Start the Go server
echo "[1/12] Starting Go server (with mocks)..."
TRIM_PKGVAR="/tmp/easytier-mock-test/data" TRIM_APPDEST="/tmp/easytier-mock-test/app" TRIM_SERVICE_PORT="11216" /app/server > /tmp/easytier-mock-test/server.log 2>&1 &
SRV_PID=$!
sleep 2

if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Server process running (pid $SRV_PID)"
else
    log_fail "Server failed to start"
    cat /tmp/easytier-mock-test/server.log
    exit 1
fi

# Test 2: Initial status
echo "[2/12] Testing initial /api/status..."
STATUS=$(curl -sf http://127.0.0.1:11216/api/status 2>/dev/null || echo "")
if echo "$STATUS" | grep -q '"service_enabled":false'; then
    log_pass "Initial status correct (disabled, no binary running)"
else
    log_fail "Initial status wrong: $STATUS"
fi

# Test 3: Save settings with service_enabled=true and network config
echo "[3/12] Saving settings with full config..."
POST_RESULT=$(curl -sf -X POST http://127.0.0.1:11216/api/settings \
    -H "Content-Type: application/json" \
    -d '{"service_enabled":true,"network_name":"testnet","network_secret":"secret123","dhcp":true,"peer_urls":["tcp://1.2.3.4:11010"],"encryption":true,"mtu":1420}')
if echo "$POST_RESULT" | grep -q '"ok":true'; then
    log_pass "Settings save with full config succeeded"
else
    log_fail "Settings save failed: $POST_RESULT"
fi

# Test 4: Start service
echo "[4/12] Starting service..."
START_RESULT=$(curl -sf -X POST http://127.0.0.1:11216/api/service/start 2>/dev/null || echo "")
if echo "$START_RESULT" | grep -q '"ok":true'; then
    log_pass "Service start succeeded"
    sleep 2
else
    log_fail "Service start failed: $START_RESULT"
fi

# Test 5: Verify service is running
echo "[5/12] Verifying service running status..."
STATUS=$(curl -sf http://127.0.0.1:11216/api/status 2>/dev/null || echo "")
if echo "$STATUS" | grep -q '"running":true\|"daemon":true'; then
    log_pass "Service reports running"
else
    log_fail "Service not reporting running: $STATUS"
fi

# Test 6: Check easytier-core process exists
echo "[6/12] Checking easytier-core process..."
if pgrep -f "mock-core.sh" > /dev/null 2>&1; then
    log_pass "easytier-core mock is running"
else
    log_fail "easytier-core mock is not running"
fi

# Test 7: GET /api/peers (uses cli)
echo "[7/12] Testing /api/peers..."
PEERS=$(curl -sf http://127.0.0.1:11216/api/peers 2>/dev/null || echo "")
if echo "$PEERS" | grep -q '"hostname":"node-1"'; then
    log_pass "/api/peers returns mock peer data"
else
    log_fail "/api/peers failed: $PEERS"
fi

# Test 8: GET /api/routes
echo "[8/12] Testing /api/routes..."
ROUTES=$(curl -sf http://127.0.0.1:11216/api/routes 2>/dev/null || echo "")
if echo "$ROUTES" | grep -q '\['; then
    log_pass "/api/routes returns JSON array"
else
    log_fail "/api/routes failed: $ROUTES"
fi

# Test 9: GET /api/connectors
echo "[9/12] Testing /api/connectors..."
CONN=$(curl -sf http://127.0.0.1:11216/api/connectors 2>/dev/null || echo "")
if echo "$CONN" | grep -q '"protocol":"tcp"'; then
    log_pass "/api/connectors returns mock connector data"
else
    log_fail "/api/connectors failed: $CONN"
fi

# Test 10: POST /api/connector/add
echo "[10/12] Testing POST /api/connector/add..."
ADD_RESULT=$(curl -sf -X POST http://127.0.0.1:11216/api/connector/add \
    -H "Content-Type: application/json" \
    -d '{"url":"tcp://5.6.7.8:11010"}')
if echo "$ADD_RESULT" | grep -q '"ok":true'; then
    log_pass "Connector add succeeded"
else
    log_fail "Connector add failed: $ADD_RESULT"
fi

# Test 11: GET /api/log/app
echo "[11/12] Testing /api/log/app..."
LOG=$(curl -sf http://127.0.0.1:11216/api/log/app 2>/dev/null || echo "")
if echo "$LOG" | grep -q '"log"'; then
    log_pass "/api/log/app returns log data"
else
    log_fail "/api/log/app failed: $LOG"
fi

# Test 12: Stop service
echo "[12/12] Stopping service..."
STOP_RESULT=$(curl -sf -X POST http://127.0.0.1:11216/api/service/stop 2>/dev/null || echo "")
if echo "$STOP_RESULT" | grep -q '"ok":true'; then
    log_pass "Service stop succeeded"
    sleep 2
else
    log_fail "Service stop failed: $STOP_RESULT"
fi

# Verify easytier-core stopped
if ! pgrep -f "mock-core.sh" > /dev/null 2>&1; then
    log_pass "easytier-core mock is no longer running"
else
    log_fail "easytier-core mock is still running"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true
pkill -f mock-core.sh 2>/dev/null || true
sleep 1
rm -rf /tmp/easytier-mock-test

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1