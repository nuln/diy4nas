#!/bin/sh
# fnOS 桌面框架兼容入口：把 fnOS 框架传过来的 action 代理到 Go server
echo "Content-Type: application/json; charset=utf-8"
echo ""

PORT="7681"
[ -f /tmp/scheduler-port ] && PORT=$(cat /tmp/scheduler-port)
API="http://127.0.0.1:$PORT"

ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"

case "$ACTION" in
  status|healthz)     /usr/bin/curl -sf "$API/api/healthz" 2>/dev/null || echo '{"ok":false}' ;;
  stats)              /usr/bin/curl -sf "$API/api/stats" 2>/dev/null || echo '{}' ;;
  jobs)               /usr/bin/curl -sf "$API/api/jobs" 2>/dev/null || echo '[]' ;;
  runs)               /usr/bin/curl -sf "$API/api/runs?limit=5" 2>/dev/null || echo '[]' ;;
  settings)           /usr/bin/curl -sf "$API/api/settings" 2>/dev/null || echo '{}' ;;
  *)                  echo '{"error":"unknown action"}' ;;
esac
