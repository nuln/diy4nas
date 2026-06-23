#!/bin/sh
# fnOS 桌面框架 iframe 入口：透明代理 /api/* 到 Go server
echo "Content-Type: application/json; charset=utf-8"
echo ""

PORT="7683"
[ -f /tmp/eventhook-port ] && PORT=$(cat /tmp/eventhook-port)
BASE="http://127.0.0.1:$PORT"

QS="$QUERY_STRING"
METHOD="${REQUEST_METHOD:-GET}"
POST_DATA=""
REAL_PATH=""

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
    ACT="${QS#action=}"
    ACT="${ACT%%&*}"
    REAL_PATH="/api/${ACT}"
    REST="${QS#action=${ACT}}"
    REST="${REST#&}"
    [ -n "$REST" ] && REAL_PATH="${REAL_PATH}?${REST}"
else
    REAL_PATH="/api/${QS}"
fi

REAL_PATH=$(echo "$REAL_PATH" | sed 's#^//*#/#' | sed 's/%2[fF]/\//g; s/%3[fF]/?/g; s/%3[dD]/=/g; s/%26/\&/g; s/+/ /g')

[ -n "$METHOD_VAL" ] && METHOD="$METHOD_VAL"

if [ "$METHOD" = "POST" ] || [ "$METHOD" = "PUT" ]; then
    if [ -n "$CONTENT_LENGTH" ] && [ "$CONTENT_LENGTH" -gt 0 ] 2>/dev/null; then
        POST_DATA=$(head -c "$CONTENT_LENGTH")
    else
        POST_DATA=$(cat)
    fi
fi

# SSE stream for event log
if echo "$REAL_PATH" | grep -qE "/api/events/stream$"; then
    echo "Content-Type: text/event-stream; charset=utf-8"
    echo "Cache-Control: no-cache"
    echo "Connection: keep-alive"
    echo ""
    /usr/bin/curl -sNL -X "$METHOD" "$BASE$REAL_PATH" 2>/dev/null
    exit 0
fi

HEADERS=""
if [ -n "$CONTENT_TYPE" ]; then
    HEADERS="-H Content-Type:$CONTENT_TYPE"
fi

if [ -n "$POST_DATA" ]; then
    /usr/bin/curl -sfL -X "$METHOD" $HEADERS -d "$POST_DATA" "$BASE$REAL_PATH" 2>/dev/null || echo "{\"error\":\"request failed: $METHOD $BASE$REAL_PATH\"}"
else
    /usr/bin/curl -sfL -X "$METHOD" $HEADERS "$BASE$REAL_PATH" 2>/dev/null || echo "{\"error\":\"request failed: $METHOD $BASE$REAL_PATH\"}"
fi
