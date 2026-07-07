#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""

PORT="8088"
[ -f /tmp/tailscale-port ] && PORT=$(cat /tmp/tailscale-port 2>/dev/null)

# 从 HTTP_HOST 获取 NAS 地址（去掉端口号）
NAS_HOST="${HTTP_HOST%%:*}"
[ -z "$NAS_HOST" ] && NAS_HOST="127.0.0.1"

# 检查 Go server 是否在运行
if ! /usr/bin/curl -sf "http://127.0.0.1:$PORT/api/healthz" >/dev/null 2>&1; then
    # Go server 没在跑，自动启动
    /var/apps/tailscale/cmd/main start >/dev/null 2>&1 &
    # 等待启动（最多 15 秒）
    i=0
    while [ $i -lt 15 ]; do
        if /usr/bin/curl -sf "http://127.0.0.1:$PORT/api/healthz" >/dev/null 2>&1; then
            break
        fi
        sleep 1
        i=$((i + 1))
    done
fi

if /usr/bin/curl -sf "http://127.0.0.1:$PORT/api/healthz" >/dev/null 2>&1; then
    # Go server 就绪，用 iframe 加载实际页面
    cat << HTM
<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Tailscale</title>
<style>*{margin:0;padding:0;border:0}html,body{width:100%;height:100%;overflow:hidden}iframe{width:100%;height:100%;border:0}</style>
</head><body><iframe src="http://$NAS_HOST:$PORT/"></iframe></body></html>
HTM
else
    # 启动失败，显示错误并自动刷新
    cat << HTM
<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Tailscale</title>
<meta http-equiv="refresh" content="3">
<style>body{font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;background:#1a1a2e;color:#eee;font-size:18px}</style>
</head><body><div>
<p>Tailscale 服务正在启动...</p>
<p style="font-size:14px;color:#888;margin-top:10px">页面将在 3 秒后自动刷新</p>
</div></body></html>
HTM
fi
