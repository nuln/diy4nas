#!/bin/bash
# nginx 反向代理测试：scheduler 和 terminal 通过 HTTP 端口暴露
set -e

NGINX_PORT=8080
NGINX_CONF=/tmp/nginx-test.conf
NGINX_PID=/tmp/nginx.pid

# 确保 app 都在跑
for slug in scheduler terminal; do
    if ! pgrep -f "/var/apps/$slug/app/server" >/dev/null 2>&1; then
        echo "[PROXY] starting $slug..."
        bash $(dirname "$0")/scripts/cmd.sh $slug start 2>&1 | tail -3
    fi
done
sleep 2

mkdir -p /tmp/nginx-logs
cat > $NGINX_CONF <<EOF
worker_processes 1;
pid $NGINX_PID;
error_log /tmp/nginx-logs/error.log info;
events { worker_connections 64; }
http {
    access_log /tmp/nginx-logs/access.log;
    map \$http_upgrade \$connection_upgrade {
        default upgrade;
        "" close;
    }
    server {
        listen $NGINX_PORT;
        # 模拟 fnOS 反代：/app/scheduler/* → 127.0.0.1:7681
        location /app/scheduler/ {
            proxy_pass http://127.0.0.1:7681/;
            proxy_http_version 1.1;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_buffering off;
        }
        location /app/terminal/ {
            proxy_pass http://127.0.0.1:7682/;
            proxy_http_version 1.1;
            proxy_set_header Host \$host;
            proxy_set_header Upgrade \$http_upgrade;
            proxy_set_header Connection \$connection_upgrade;
            proxy_read_timeout 86400;
            proxy_buffering off;
        }
    }
}
EOF

nginx -t -c $NGINX_CONF 2>&1
nginx -c $NGINX_CONF
sleep 1

echo "=== GET /app/scheduler/api/healthz ==="
RESP=$(curl -s http://127.0.0.1:$NGINX_PORT/app/scheduler/api/healthz)
echo "  $RESP"
if echo "$RESP" | grep -q '"ok":true'; then
    echo "  ✓ nginx → scheduler works"
else
    echo "  ✗ nginx → scheduler FAILED"
fi

echo "=== GET /app/scheduler/ (UI) ==="
UI=$(curl -s http://127.0.0.1:$NGINX_PORT/app/scheduler/ | head -c 200)
echo "  ${UI:0:120}..."
if echo "$UI" | grep -q '<title>计划任务</title>'; then
    echo "  ✓ nginx → scheduler UI works"
else
    echo "  ✗ nginx → scheduler UI FAILED"
fi

echo "=== GET /app/terminal/api/healthz ==="
RESP=$(curl -s http://127.0.0.1:$NGINX_PORT/app/terminal/api/healthz)
echo "  $RESP"
if echo "$RESP" | grep -q '"ok":true'; then
    echo "  ✓ nginx → terminal works"
else
    echo "  ✗ nginx → terminal FAILED"
fi

echo "=== GET /app/terminal/api/sessions ==="
RESP=$(curl -s http://127.0.0.1:$NGINX_PORT/app/terminal/api/sessions)
echo "  $RESP"
if echo "$RESP" | grep -q '\['; then
    echo "  ✓ nginx → terminal sessions API works"
else
    echo "  ✗ nginx → terminal sessions FAILED"
fi

nginx -c $NGINX_CONF -s stop 2>/dev/null || true
sleep 1
rm -f $NGINX_PID $NGINX_CONF /tmp/nginx-logs/error.log /tmp/nginx-logs/access.log
