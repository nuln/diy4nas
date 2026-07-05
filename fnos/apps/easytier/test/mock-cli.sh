#!/bin/sh
# Mock easytier-cli for testing
# Responds to easytier-cli subcommands with fake JSON data

case "$1" in
  node)
    case "$2" in
      info)
        cat <<'EOF'
{"hostname":"test-node","peer_id":"abcdef1234567890","version":"v2.6.4","ipv4_addr":"10.144.144.1/24","rx_bytes":1024,"tx_bytes":512}
EOF
        ;;
      config)
        echo 'network-name = "test"'
        ;;
    esac
    ;;
  peer)
    case "$2" in
      list)
        cat <<'EOF'
[{"id":"peer-1","hostname":"node-1","ip":"10.144.144.2","connected":true,"online":true,"rx_bytes":2048,"tx_bytes":1024,"lat_ms":5},{"id":"peer-2","hostname":"node-2","ip":"10.144.144.3","connected":false,"online":false,"rx_bytes":0,"tx_bytes":0,"lat_ms":0}]
EOF
        ;;
    esac
    ;;
  route)
    echo '[]'
    ;;
  connector)
    case "$2" in
      list)
        cat <<'EOF'
[{"protocol":"tcp","address":"tcp://0.0.0.0:11010","status":"Listening"}]
EOF
        ;;
      add|remove)
        echo "ok"
        ;;
    esac
    ;;
  port-forward)
    case "$2" in
      list)
        echo '[]'
        ;;
      *)
        echo "ok"
        ;;
    esac
    ;;
  whitelist)
    case "$2" in
      show)
        echo '{"tcp":"80,443","udp":""}'
        ;;
      *)
        echo "ok"
        ;;
    esac
    ;;
  credential)
    case "$2" in
      list)
        echo '[]'
        ;;
      generate|revoke)
        echo "mock-credential-output"
        ;;
    esac
    ;;
  stats)
    case "$2" in
      show)
        echo '{"cpu_usage":1.5,"mem_usage":1024}'
        ;;
      prometheus)
        echo "# HELP easytier_up Node is up"
        echo "easytier_up 1"
        ;;
    esac
    ;;
  vpn-portal)
    echo '{"connector_name":"wg","ip":"10.14.14.1","port":11013}'
    ;;
  logger)
    case "$2" in
      get)
        echo '"info"'
        ;;
      set)
        echo "log level set to $3"
        ;;
    esac
    ;;
  *)
    echo "unknown command: $*" >&2
    exit 1
    ;;
esac