#!/bin/sh
echo "Content-Type: application/json; charset=utf-8"
echo ""

# 读取 socket 路径（与 cmd/main 一致）
APP_VAR="${TRIM_PKGVAR:-/var/apps/tailscale/data}"
SOCKET="${APP_VAR}/tailscaled.sock"
TS="/usr/bin/tailscale --socket=$SOCKET"
SUDO="/usr/bin/sudo"

ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"

case "$ACTION" in
  status)
    $TS status --json 2>/dev/null || echo '{"online":false}'
    ;;
  traffic)
    OUT=$($TS status --json 2>/dev/null)
    RX=$(echo "$OUT" | grep -o '"RxBytes":[0-9]*' | sed 's/.*://' | awk '{s+=$1}END{print s+0}')
    TX=$(echo "$OUT" | grep -o '"TxBytes":[0-9]*' | sed 's/.*://' | awk '{s+=$1}END{print s+0}')
    echo "{\"rx\":${RX:-0},\"tx\":${TX:-0}}"
    ;;
  up)
    read -r POST
    AUTH=$(echo "$POST" | grep -o 'authKey=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    HOST=$(echo "$POST" | grep -o 'hostname=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    ROUTE=$(echo "$POST" | grep -o 'routes=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    LOGIN=$(echo "$POST" | grep -o 'loginServer=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    ARGS="up --accept-risk=all"
    [ -n "$AUTH" ] && ARGS="$ARGS --authkey=$AUTH"
    [ -n "$HOST" ] && ARGS="$ARGS --hostname=$HOST"
    [ -n "$ROUTE" ] && ARGS="$ARGS --advertise-routes=$ROUTE"
    [ -n "$LOGIN" ] && ARGS="$ARGS --login-server=$LOGIN"
    OUT=$($SUDO $TS $ARGS 2>&1)
    echo "{\"output\":$(echo "$OUT" | jq -Rs .)}"
    ;;
  down)
    OUT=$($SUDO $TS down 2>&1)
    echo "{\"output\":$(echo "$OUT" | jq -Rs .)}"
    ;;
  logout)
    OUT=$($SUDO $TS logout 2>&1)
    echo "{\"output\":$(echo "$OUT" | jq -Rs .)}"
    ;;
  ping)
    read -r POST
    TARGET=$(echo "$POST" | grep -o 'target=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    COUNT=$(echo "$POST" | grep -o 'count=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    COUNT="${COUNT:-10}"
    OUT=$($TS ping --c "$COUNT" "$TARGET" 2>&1)
    echo "{\"output\":$(echo "$OUT" | jq -Rs .)}"
    ;;
  log)
    OUT=$(journalctl -u tailscaled --no-pager -n 200 --no-hostname 2>/dev/null || echo "no logs")
    echo "{\"log\":$(echo "$OUT" | jq -Rs .)}"
    ;;
  *)
    echo '{"error":"unknown action"}'
    ;;
esac
