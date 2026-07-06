#!/bin/sh
echo "Content-Type: application/json; charset=utf-8"
echo ""
PORT="8088"
[ -f /tmp/tailscale-port ] && PORT=$(cat /tmp/tailscale-port)
API="http://127.0.0.1:$PORT/api"
ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"
case "$ACTION" in
  status) /usr/bin/curl -sf "$API/status" 2>/dev/null || echo '{"online":false}' ;;
  traffic) /usr/bin/curl -sf "$API/traffic" 2>/dev/null || echo '{"rx":0,"tx":0}' ;;
  up) read -r POST; /usr/bin/curl -sf -X POST -d "$POST" "$API/up" 2>/dev/null || echo '{"error":"failed"}' ;;
  down) /usr/bin/curl -sf -X POST "$API/down" 2>/dev/null || echo '{}' ;;
  ping) read -r POST; /usr/bin/curl -sf -X POST -d "$POST" "$API/ping" 2>/dev/null || echo '{"error":"failed"}' ;;
  log) /usr/bin/curl -sf "$API/log/ts" 2>/dev/null || echo '{"log":""}' ;;
  *) echo '{"error":"unknown action"}' ;;
esac
