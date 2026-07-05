#!/bin/bash
# WebSocket via nginx (模拟 fnOS 反代)
set -e

mkdir -p /tmp/nginx-logs
cat > /tmp/nginx-test.conf <<EOF
worker_processes 1;
pid /tmp/nginx.pid;
error_log /tmp/nginx-logs/error.log info;
events { worker_connections 64; }
http {
    access_log /tmp/nginx-logs/access.log;
    map \$http_upgrade \$connection_upgrade {
        default upgrade;
        "" close;
    }
    server {
        listen 8080;
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

nginx -t -c /tmp/nginx-test.conf 2>&1
nginx -c /tmp/nginx-test.conf
sleep 1

SESSION=$(curl -s http://127.0.0.1:8080/app/terminal/api/sessions \
    -X POST -H "Content-Type: application/json" \
    -d '{"title":"ws-test","cols":80,"rows":24}')
SID=$(echo "$SESSION" | python3 -c "import sys, json; print(json.load(sys.stdin)['id'])")
echo "Session: $SID"

python3 <<EOF
import socket, base64, os, struct, time
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.connect(("127.0.0.1", 8080))
key = base64.b64encode(os.urandom(16)).decode()
req = (
    f"GET /app/terminal/api/ws?session=$SID&cols=80&rows=24 HTTP/1.1\r\n"
    f"Host: 127.0.0.1:8080\r\n"
    f"Upgrade: websocket\r\n"
    f"Connection: Upgrade\r\n"
    f"Sec-WebSocket-Key: {key}\r\n"
    f"Sec-WebSocket-Version: 13\r\n\r\n"
)
sock.send(req.encode())
resp = b""
while b"\r\n\r\n" not in resp:
    resp += sock.recv(4096)
print("HTTP upgrade:", resp.split(b"\r\n")[0].decode())

def encode_frame(payload, opcode=0x2):
    header = bytearray([0x80 | opcode])
    L = len(payload)
    header.append(0x80 | (126 if L >= 126 else L))
    if L >= 126: header += struct.pack(">H", L)
    mask = os.urandom(4)
    header += mask
    masked = bytes(b ^ mask[i%4] for i, b in enumerate(payload))
    return bytes(header) + masked

def decode_frame(sock):
    h = sock.recv(2)
    if len(h) < 2: return None, None
    opcode = h[0] & 0x0F
    L = h[1] & 0x7F
    if L == 126: L = struct.unpack(">H", sock.recv(2))[0]
    data = b""
    while len(data) < L: data += sock.recv(L - len(data))
    return opcode, data

sock.settimeout(2)
buf = b""
try:
    while True:
        op, data = decode_frame(sock)
        if op in (0x1, 0x2): buf += data
        if len(buf) > 30: break
except: pass
print("Initial:", repr(buf[:60]))

sock.send(encode_frame(b"echo WS_VIA_NGINX; whoami\n"))
time.sleep(2)
buf = b""
try:
    while True:
        op, data = decode_frame(sock)
        if op in (0x1, 0x2): buf += data
        if len(buf) > 300: break
except: pass
print("Output:", repr(buf[:300]))
sock.close()
EOF

nginx -c /tmp/nginx-test.conf -s stop 2>/dev/null || true
