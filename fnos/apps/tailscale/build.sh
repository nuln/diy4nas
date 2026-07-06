#!/bin/bash
# 下载 tailscale 官方二进制（缓存复用，不重复下载）
set -e
VER="1.98.4"
CACHE_DIR="${CACHE_DIR:-$(dirname "$0")/.cache}"
mkdir -p "$CACHE_DIR"
TGZ="$CACHE_DIR/tailscale_${VER}_${ARCH}.tgz"
URL="https://pkgs.tailscale.com/stable/tailscale_${VER}_${ARCH}.tgz"
if [ -f "$TGZ" ]; then
  echo "  using cached tailscale v${VER}..."
else
  echo "  downloading tailscale v${VER}..."
  curl -fsSL -o "$TGZ" "$URL"
fi
tar xzf "$TGZ" -C "$BUILD_DIR" --strip-components=1 2>/dev/null
rm -rf "$BUILD_DIR/systemd"
echo "  tailscale binary ready"
