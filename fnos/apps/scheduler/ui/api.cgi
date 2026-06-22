#!/bin/sh
# fnOS 桌面框架 iframe 入口：透明代理 /api/* 到 Go server
# 用法（推荐）：api.cgi?path=/api/jobs&id=123
# HTTP method 用 _method=GET/POST/PUT/DELETE 覆盖（cgi 默认 GET/POST）
# 兼容旧风格：api.cgi?action=healthz → /api/healthz
# POST/PUT 请求 body 通过 stdin 传给 Go server

echo "Content-Type: application/json; charset=utf-8"
echo ""

PORT="7681"
[ -f /tmp/scheduler-port ] && PORT=$(cat /tmp/scheduler-port)
BASE="http://127.0.0.1:$PORT"

QS="$QUERY_STRING"
METHOD="${REQUEST_METHOD:-GET}"
POST_DATA=""
REAL_PATH=""

# 解析 query string：分离 path= 和 _method= ，其余原样转发
NEW_QS=""
PATH_VAL=""
METHOD_VAL=""
IFS='&'
for kv in $QS; do
    case "$kv" in
        path=*)    PATH_VAL="/${kv#path=}";;
        _method=*) METHOD_VAL="${kv#_method=}";;
        *) NEW_QS="${NEW_QS}${NEW_QS:+&}${kv}";;
    esac
done
unset IFS

if [ -n "$PATH_VAL" ]; then
    REAL_PATH="$PATH_VAL"
    [ -n "$NEW_QS" ] && REAL_PATH="${REAL_PATH}?${NEW_QS}"
elif echo "$QS" | grep -q "^action="; then
    # 兼容旧风格
    ACT="${QS#action=}"
    ACT="${ACT%%&*}"
    REAL_PATH="/api/${ACT}"
    REST="${QS#action=${ACT}}"
    REST="${REST#&}"
    [ -n "$REST" ] && REAL_PATH="${REAL_PATH}?${REST}"
else
    REAL_PATH="/api/${QS}"
fi

# 规范化前导斜杠: // -> /
REAL_PATH=$(echo "$REAL_PATH" | sed 's#^//*#/#')

[ -n "$METHOD_VAL" ] && METHOD="$METHOD_VAL"

# POST/PUT body
if [ "$METHOD" = "POST" ] || [ "$METHOD" = "PUT" ]; then
    if [ -n "$CONTENT_LENGTH" ] && [ "$CONTENT_LENGTH" -gt 0 ] 2>/dev/null; then
        POST_DATA=$(head -c "$CONTENT_LENGTH")
    else
        POST_DATA=$(cat)
    fi
fi

# SSE 流式响应
if echo "$REAL_PATH" | grep -qE "/api/runs/[^/]+/log$"; then
    echo "Content-Type: text/event-stream; charset=utf-8"
    echo "Cache-Control: no-cache"
    echo "Connection: keep-alive"
    echo ""
    if [ -n "$POST_DATA" ]; then
        /usr/bin/curl -sNL -X "$METHOD" -H "Content-Type: application/json" -d "$POST_DATA" "$BASE$REAL_PATH" 2>/dev/null
    else
        /usr/bin/curl -sNL -X "$METHOD" "$BASE$REAL_PATH" 2>/dev/null
    fi
    exit 0
fi

# 普通 JSON 请求
HEADERS=""
if [ -n "$CONTENT_TYPE" ]; then
    HEADERS="-H Content-Type:$CONTENT_TYPE"
fi

# Go server 对 //path 自动 301 到 /path，curl -L 跟随重定向
if [ -n "$POST_DATA" ]; then
    /usr/bin/curl -sfL -X "$METHOD" $HEADERS -d "$POST_DATA" "$BASE$REAL_PATH" 2>/dev/null || echo "{\"error\":\"request failed: $METHOD $BASE$REAL_PATH\"}"
else
    /usr/bin/curl -sfL -X "$METHOD" $HEADERS "$BASE$REAL_PATH" 2>/dev/null || echo "{\"error\":\"request failed: $METHOD $BASE$REAL_PATH\"}"
fi
