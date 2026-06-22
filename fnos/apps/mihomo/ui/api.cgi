#!/bin/sh
PORT="9097"
[ -f /tmp/mihomo-port ] && PORT=$(cat /tmp/mihomo-port)
BASE="http://127.0.0.1:$PORT"

ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"

# events need special content-type handling
if [ "$ACTION" = "events" ]; then
  echo "Content-Type: text/event-stream"
  echo ""
  /usr/bin/curl -sN "$BASE/api/events" 2>/dev/null
  exit 0
fi

echo "Content-Type: application/json; charset=utf-8"
echo ""

case "$ACTION" in
  status)
    /usr/bin/curl -sf "$BASE/api/status" 2>/dev/null || echo '{"running":false}'
    ;;
  log)
    /usr/bin/curl -sf "$BASE/api/log" 2>/dev/null || echo '{"log":"service not available"}'
    ;;
  proxies)
    /usr/bin/curl -sf "$BASE/api/proxy/proxies" 2>/dev/null || echo '{"error":"mihomo API not available"}'
    ;;
  traffic)
    /usr/bin/curl -sf "$BASE/api/status" 2>/dev/null || echo '{"rx":0,"tx":0}'
    ;;
  config)
    if [ "$REQUEST_METHOD" = "POST" ]; then
      read -r POST_DATA
      /usr/bin/curl -sf -X POST "$BASE/api/config" -d "$POST_DATA" 2>/dev/null || echo '{"error":"failed"}'
    else
      /usr/bin/curl -sf "$BASE/api/config" 2>/dev/null || echo '{"error":"failed"}'
    fi
    ;;
  restart)
    /usr/bin/curl -sf -X POST "$BASE/api/restart" 2>/dev/null || echo '{"error":"failed"}'
    ;;
  version)
    /usr/bin/curl -sf "$BASE/api/status" 2>/dev/null | /usr/bin/sed 's/.*"version":"\([^"]*\)".*/\1/' || echo "unknown"
    ;;
  *)
    echo '{"error":"unknown action"}'
    ;;
esac
