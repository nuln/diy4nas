#!/bin/sh
echo "Content-Type: application/json; charset=utf-8"
echo ""

PORT="11210"
[ -f /tmp/easytier-port ] && PORT=$(cat /tmp/easytier-port)
API="http://127.0.0.1:$PORT/api"

ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"

case "$ACTION" in
  status)              /usr/bin/curl -sf "$API/status" 2>/dev/null || echo '{"online":false}' ;;
  traffic)             /usr/bin/curl -sf "$API/traffic" 2>/dev/null || echo '{"rx":0,"tx":0}' ;;
  peers)               /usr/bin/curl -sf "$API/peers" 2>/dev/null || echo '[]' ;;
  routes)              /usr/bin/curl -sf "$API/routes" 2>/dev/null || echo '[]' ;;
  connectors)          /usr/bin/curl -sf "$API/connectors" 2>/dev/null || echo '[]' ;;
  port_forwards)       /usr/bin/curl -sf "$API/port-forward/list" 2>/dev/null || echo '[]' ;;
  credentials)         /usr/bin/curl -sf "$API/credential/list" 2>/dev/null || echo '[]' ;;
  stats)               /usr/bin/curl -sf "$API/stats" 2>/dev/null || echo '{}' ;;
  vpn_portal)          /usr/bin/curl -sf "$API/vpn-portal" 2>/dev/null || echo '{}' ;;
  log_level)           /usr/bin/curl -sf "$API/log/level" 2>/dev/null || echo '{}' ;;
  node_config)         /usr/bin/curl -sf "$API/node/config" 2>/dev/null || echo '{}' ;;
  settings)            /usr/bin/curl -sf "$API/settings" 2>/dev/null || echo '{}' ;;
  service_start)       /usr/bin/curl -sf -X POST "$API/service/start" 2>/dev/null || echo '{"ok":false}' ;;
  service_stop)        /usr/bin/curl -sf -X POST "$API/service/stop" 2>/dev/null || echo '{"ok":false}' ;;
  service_restart)     /usr/bin/curl -sf -X POST "$API/service/restart" 2>/dev/null || echo '{"ok":false}' ;;
  save_settings)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/settings" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  up)                  /usr/bin/curl -sf -X POST "$API/service/start" 2>/dev/null || echo '{"ok":false}' ;;
  down)                /usr/bin/curl -sf -X POST "$API/service/stop" 2>/dev/null || echo '{"ok":false}' ;;
  ping)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/ping" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"output":""}'
    ;;
  connector_add)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/connector/add" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  connector_remove)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/connector/remove" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  port_forward_add)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/port-forward/add" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  port_forward_remove)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/port-forward/remove" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  whitelist_show)      /usr/bin/curl -sf "$API/whitelist/show" 2>/dev/null || echo '{}' ;;
  whitelist_set)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/whitelist/set" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  whitelist_clear)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/whitelist/clear" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  credential_generate)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/credential/generate" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  credential_revoke)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/credential/revoke" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  log_level_set)
    read -r POST
    /usr/bin/curl -sf -X POST "$API/log/level/set" -H "Content-Type: application/json" -d "$POST" 2>/dev/null || echo '{"ok":false}'
    ;;
  log)                 /usr/bin/curl -sf "$API/log" 2>/dev/null || echo '{"log":"no logs"}' ;;
  log_app)             /usr/bin/curl -sf "$API/log/app" 2>/dev/null || echo '{"log":"no logs"}' ;;
  log_core)            /usr/bin/curl -sf "$API/log/core" 2>/dev/null || echo '{"log":"no logs"}' ;;
  log_clear)           /usr/bin/curl -sf -X POST "$API/log/clear" 2>/dev/null || echo '{"ok":true}' ;;
  *)                   echo '{"error":"unknown action"}' ;;
esac