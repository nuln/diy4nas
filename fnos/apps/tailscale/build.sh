#!/bin/bash
# 下载 tailscale 官方二进制
set -e
VER="1.98.4"
URL="https://pkgs.tailscale.com/stable/tailscale_${VER}_${ARCH}.tgz"
echo "  downloading tailscale v${VER}..."
curl -fsSL "$URL" | tar xz -C "$BUILD_DIR" --strip-components=1 2>/dev/null
echo "  tailscale binary ready"
