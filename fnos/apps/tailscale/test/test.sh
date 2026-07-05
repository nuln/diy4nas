#!/bin/sh
set -e
PASS=0
FAIL=0

log_pass() { PASS=$((PASS+1)); echo "  [PASS] $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  [FAIL] $1"; }

echo "=== Tailscale Proxy Feature Test ==="
echo ""

# ── 1. Start the Go server ──────────────────────────────────
echo "[1] Starting server..."
TRIM_PKGVAR="/data" TRIM_APPDEST="/app" \
TAILSCALE_BIN="/app/tailscale" TAILSCALED_BIN="/app/tailscaled" \
TAILSCALE_SOCKET="/data/tailscaled.sock" TAILSCALE_PORT="8088" \
/app/server &
SRV_PID=$!
sleep 2

if kill -0 "$SRV_PID" 2>/dev/null; then
	log_pass "Server running (pid $SRV_PID)"
else
	log_fail "Server not running"
	echo "--- server log ---"
	cat /data/tailscaled.log 2>/dev/null || true
	exit 1
fi

# ── 2. GET proxy (should be empty) ──────────────────────────
echo ""
echo "[2] GET /api/proxy (initial, should be empty)..."
RESP=$(curl -sf http://127.0.0.1:8088/api/proxy 2>/dev/null || echo "")
PROXY=$(echo "$RESP" | sed 's/.*"proxy":"\([^"]*\)".*/\1/')
if [ "$PROXY" = "" ] && echo "$RESP" | grep -q "proxy"; then
	log_pass "Initial proxy is empty: proxy=\"$PROXY\""
else
	log_fail "Expected empty proxy, got: $RESP"
fi

# ── 3. POST proxy ───────────────────────────────────────────
echo ""
echo "[3] POST /api/proxy {\"proxy\":\"http://myproxy:7890\"}..."
RESP=$(curl -sf -X POST -H 'Content-Type: application/json' \
	-d '{"proxy":"http://myproxy:7890"}' \
	http://127.0.0.1:8088/api/proxy 2>/dev/null || echo "")
if echo "$RESP" | grep -q "代理配置已保存"; then
	log_pass "Proxy saved successfully"
else
	log_fail "POST proxy failed: $RESP"
fi

sleep 2

# ── 4. GET proxy (should be http://myproxy:7890) ────────────
echo ""
echo "[4] GET /api/proxy (after save)..."
RESP=$(curl -sf http://127.0.0.1:8088/api/proxy 2>/dev/null || echo "")
PROXY=$(echo "$RESP" | sed 's/.*"proxy":"\([^"]*\)".*/\1/')
if [ "$PROXY" = "http://myproxy:7890" ]; then
	log_pass "Proxy correctly saved: \"$PROXY\""
else
	log_fail "Expected 'http://myproxy:7890', got: $RESP"
fi

# ── 5. Check tailscaled env for proxy ───────────────────────
echo ""
echo "[5] Checking tailscaled env for HTTP_PROXY..."
sleep 1
ENV_FILE="/data/tailscaled.sock.env"
if [ -f "$ENV_FILE" ]; then
	if grep -q "HTTP_PROXY=http://myproxy:7890" "$ENV_FILE" && \
	   grep -q "HTTPS_PROXY=http://myproxy:7890" "$ENV_FILE"; then
		log_pass "HTTP_PROXY/HTTPS_PROXY found in tailscaled env"
		grep "HTTP_PROXY=" "$ENV_FILE"
		grep "NO_PROXY=" "$ENV_FILE"
	else
		log_fail "PROXY env not found in $ENV_FILE"
		echo "--- env file ---"
		cat "$ENV_FILE"
	fi
	if grep -q "NO_PROXY=100.64.0.0/10,fd7a:115c:a1e0::/48,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,localhost,127.0.0.1" "$ENV_FILE"; then
		log_pass "NO_PROXY correctly set"
	else
		log_fail "NO_PROXY not found or wrong"
		echo "--- relevant lines ---"
		grep "NO_PROXY=" "$ENV_FILE" || true
	fi
else
	log_fail "tailscaled env file not found at $ENV_FILE"
	echo "--- /data/ contents ---"
	ls -la /data/
fi

# ── 6. Clear proxy ──────────────────────────────────────────
echo ""
echo "[6] POST /api/proxy {\"proxy\":\"\"} (clear)..."
RESP=$(curl -sf -X POST -H 'Content-Type: application/json' \
	-d '{"proxy":""}' \
	http://127.0.0.1:8088/api/proxy 2>/dev/null || echo "")
if echo "$RESP" | grep -q "代理配置已保存"; then
	log_pass "Proxy cleared successfully"
else
	log_fail "Clear proxy failed: $RESP"
fi

sleep 2

# ── 7. GET proxy (should be empty) ──────────────────────────
echo ""
echo "[7] GET /api/proxy (after clear)..."
RESP=$(curl -sf http://127.0.0.1:8088/api/proxy 2>/dev/null || echo "")
PROXY=$(echo "$RESP" | sed 's/.*"proxy":"\([^"]*\)".*/\1/')
if [ "$PROXY" = "" ]; then
	log_pass "Proxy correctly cleared"
else
	log_fail "Expected empty proxy, got: $RESP"
fi

# ── 8. Check tailscaled env after clear (no HTTP_PROXY) ────
echo ""
echo "[8] Checking tailscaled env after proxy cleared..."
sleep 1
if [ -f "$ENV_FILE" ]; then
	if grep -q "HTTP_PROXY=" "$ENV_FILE"; then
		log_fail "HTTP_PROXY still present after clearing proxy"
		grep "HTTP_PROXY=" "$ENV_FILE"
	else
		log_pass "HTTP_PROXY not present after clearing"
	fi
else
	log_fail "tailscaled env file not found"
fi

# ── 9. Set proxy again (for consistency) ────────────────────
echo ""
echo "[9] Setting proxy back for consistency..."
curl -sf -X POST -H 'Content-Type: application/json' \
	-d '{"proxy":"http://127.0.0.1:7890"}' \
	http://127.0.0.1:8088/api/proxy >/dev/null 2>&1 || true

# ── 10. Check proxy.conf file exists ────────────────────────
echo ""
echo "[10] Checking proxy.conf file..."
if [ -f "/data/proxy.conf" ]; then
	CONTENT=$(cat /data/proxy.conf | tr -d '\n')
	if [ "$CONTENT" = "http://127.0.0.1:7890" ]; then
		log_pass "proxy.conf content matches"
	else
		log_fail "proxy.conf content wrong: '$CONTENT'"
	fi
else
	log_fail "proxy.conf not found in /data/"
	ls -la /data/
fi

# ── Cleanup ─────────────────────────────────────────────────
echo ""
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
