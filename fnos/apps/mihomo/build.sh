#!/bin/bash
# 下载 mihomo + GeoIP/Geosite 数据库
# ARCH 来自 build.sh 顶层（如 arm64 / amd64）
# -compatible 变体仅 amd64 才有，arm64 始终是无后缀版本
set -e

BUILD_DIR="${BUILD_DIR:?BUILD_DIR must be set}"

# 检测并使用本地代理（如 Clash/Mihomo 默认的 7890 端口）
if curl -I -s --connect-timeout 2 http://127.0.0.1:7890 >/dev/null; then
    export http_proxy="http://127.0.0.1:7890"
    export https_proxy="http://127.0.0.1:7890"
    echo "  Using local proxy at 127.0.0.1:7890"
fi

download_file() {
    local url="$1"
    local dest="$2"
    local tmp_dest="${dest}.part"
    if [ -f "$dest" ] && [ -s "$dest" ]; then
        echo "  $(basename "$dest") already cached"
        return 0
    fi
    # 重试 3 次，递增超时
    for attempt in 1 2 3; do
        local timeout=$((180 * attempt))
        if [ $attempt -gt 1 ]; then
            echo "  retry $attempt (timeout ${timeout}s)..."
            sleep 2
        fi
        if curl -fsSL --connect-timeout 15 --max-time $timeout "$url" -o "$tmp_dest" 2>/dev/null; then
            mv "$tmp_dest" "$dest"
            echo "  ✓ downloaded $(basename "$dest")"
            return 0
        fi
    done
    rm -f "$tmp_dest" 2>/dev/null
    echo "  Direct download failed for $(basename "$url"), trying ghproxy.net mirror..."
    for attempt in 1 2 3; do
        local timeout=$((180 * attempt))
        if [ $attempt -gt 1 ]; then
            echo "  mirror retry $attempt..."
            sleep 2
        fi
        if curl -fsSL --connect-timeout 15 --max-time $timeout "https://ghproxy.net/$url" -o "$tmp_dest" 2>/dev/null; then
            mv "$tmp_dest" "$dest"
            echo "  ✓ downloaded $(basename "$dest") via mirror"
            return 0
        fi
    done
    rm -f "$tmp_dest" 2>/dev/null
    echo "  ✗ Failed to download $url"
    return 1
}

# 解析 mihomo 最新版本（如果失败用 hardcoded 1.19.27）
echo "  fetching latest mihomo version..."
VER=$(curl -fsSL --connect-timeout 10 --max-time 30 "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
if [ -z "$VER" ]; then
    VER=$(curl -fsSL --connect-timeout 10 --max-time 30 "https://ghproxy.net/https://api.github.com/repos/MetaCubeX/mihomo/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
fi
[ -z "$VER" ] && VER="1.19.27"
echo "  using mihomo v${VER}"

# mihomo release asset 命名规则（1.19.27 验证）：
#   amd64: mihomo-linux-amd64-compatible-v1.19.27.gz  ← 带 -compatible（兼容老 CPU）
#   arm64: mihomo-linux-arm64-v1.19.27.gz              ← 无 -compatible
# 直接根据 ARCH 拼出正确 URL，避免被动态解析的副作用坑
case "$ARCH" in
    amd64|x86_64)  SUFFIX="amd64-compatible" ;;
    arm64|aarch64) SUFFIX="arm64" ;;
    *) echo "  ✗ unsupported ARCH: $ARCH"; exit 1 ;;
esac
ASSET_URL="https://github.com/MetaCubeX/mihomo/releases/download/v${VER}/mihomo-linux-${SUFFIX}-v${VER}.gz"
echo "  asset: $ASSET_URL"

# 下载
download_file "$ASSET_URL" "$BUILD_DIR/mihomo.gz"

# 解压
if [ -f "$BUILD_DIR/mihomo.gz" ]; then
    if file "$BUILD_DIR/mihomo.gz" 2>/dev/null | grep -q "gzip"; then
        if gunzip -f "$BUILD_DIR/mihomo.gz" 2>/dev/null; then
            echo "  ✓ extracted mihomo.gz"
        else
            echo "  ✗ gunzip failed (corrupted download?)"
            rm -f "$BUILD_DIR/mihomo.gz"
            exit 1
        fi
    else
        mv "$BUILD_DIR/mihomo.gz" "$BUILD_DIR/mihomo"
    fi
fi

if [ ! -f "$BUILD_DIR/mihomo" ]; then
    echo "  ✗ ERROR: mihomo binary not extracted"
    exit 1
fi
chmod +x "$BUILD_DIR/mihomo"
ls -lh "$BUILD_DIR/mihomo"
file "$BUILD_DIR/mihomo" | head -1

# 下载 GeoIP/GeoSite 数据库
echo "  downloading GeoIP/GeoSite databases..."
download_file "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb" "$BUILD_DIR/geoip.metadb" || true
download_file "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat" "$BUILD_DIR/geosite.dat" || true

# 清理
rm -f "$BUILD_DIR/mihomo.gz" 2>/dev/null || true
echo "  mihomo build ready"
