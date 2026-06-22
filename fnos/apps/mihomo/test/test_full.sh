#!/bin/sh
# Full integration test - downloads mihomo binary and tests everything
set -e
PASS=0
FAIL=0

log_pass() { PASS=$((PASS+1)); echo "  ✅ $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }

echo "=== Mihomo Full Docker Integration Test ==="

# Download mihomo binary if not present
MIHOMO_BIN="/tmp/mihomo"
if [ ! -f "$MIHOMO_BIN" ]; then
    echo "[DOWNLOAD] Downloading mihomo..."
    ARCH="amd64"
    VER="1.19.27"
    curl -fsSL "https://github.com/MetaCubeX/mihomo/releases/download/v${VER}/mihomo-linux-${ARCH}-v${VER}.gz" \
        -o /tmp/mihomo.gz && gunzip -f /tmp/mihomo.gz && chmod +x /tmp/mihomo
    echo "  mihomo v${VER} downloaded"
fi

# Create config directory
mkdir -p /var/apps/mihomo/data/config

# Start the Go server with mihomo binary
echo "[1/7] Starting server with mihomo..."
TRIM_PKGVAR="/var/apps/mihomo/data" \
TRIM_APPDEST="/app" \
/app/server &
SRV_PID=$!
sleep 3

# Check server running
echo "[2/7] Checking server..."
if kill -0 $SRV_PID 2>/dev/null; then
    log_pass "Server running (pid $SRV_PID)"
else
    log_fail "Server not running"; exit 1
fi

# Test status
echo "[3/7] Testing /api/status..."
STATUS=$(curl -sf http://127.0.0.1:9097/api/status)
if echo "$STATUS" | grep -q '"running":true'; then
    log_pass "Mihomo is running: $STATUS"
else
    log_fail "Mihomo not running: $STATUS"
fi

# Test version
echo "[4/7] Testing version..."
if echo "$STATUS" | grep -q '"version"'; then
    VER=$(echo "$STATUS" | sed 's/.*"version":"\([^"]*\)".*/\1/')
    log_pass "Mihomo version: $VER"
else
    log_fail "No version info"
fi

# Test log
echo "[5/7] Testing /api/log..."
LOG=$(curl -sf http://127.0.0.1:9097/api/log)
if echo "$LOG" | grep -q "log"; then
    log_pass "Log endpoint works"
else
    log_fail "Log endpoint failed: $LOG"
fi

# Test UI
echo "[6/7] Testing UI..."
UI=$(curl -sf http://127.0.0.1:9097/)
if echo "$UI" | grep -q "Mihomo"; then
    log_pass "UI serves HTML"
else
    log_fail "UI failed"
fi

# Test config
echo "[7/7] Testing /api/config..."
CONFIG=$(curl -sf http://127.0.0.1:9097/api/config)
if echo "$CONFIG" | grep -q "mixed-port"; then
    log_pass "Config contains mixed-port:7890"
else
    log_fail "Config failed: $CONFIG"
fi

# Cleanup
kill $SRV_PID 2>/dev/null || true
sleep 1
pkill -x mihomo 2>/dev/null || true

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
