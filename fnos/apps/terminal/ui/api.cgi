#!/bin/sh
echo "Content-Type: application/json; charset=utf-8"
echo ""
PORT="7682"
[ -f /tmp/terminal-port ] && PORT=$(cat /tmp/terminal-port)
API="http://127.0.0.1:$PORT/api"
ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"

# 把 fnOS 反代通过 CGI 标准环境变量传的用户信息透传给 Go server
HEADERS=""
[ -n "$REMOTE_USER" ] && HEADERS="$HEADERS -H X-Forwarded-User:$REMOTE_USER"
[ -n "$HTTP_X_FORWARDED_USER" ] && HEADERS="$HEADERS -H X-Forwarded-User:$HTTP_X_FORWARDED_USER"
[ -n "$HTTP_X_REAL_USER" ] && HEADERS="$HEADERS -H X-Real-User:$HTTP_X_REAL_USER"

case "$ACTION" in
  status|healthz) /usr/bin/curl -sf $HEADERS "$API/healthz" 2>/dev/null || echo '{"ok":false}' ;;
  sessions) /usr/bin/curl -sf $HEADERS "$API/sessions" 2>/dev/null || echo '[]' ;;
  settings) /usr/bin/curl -sf $HEADERS "$API/settings" 2>/dev/null || echo '{}' ;;
  *) echo '{"error":"unknown action"}' ;;
esac
