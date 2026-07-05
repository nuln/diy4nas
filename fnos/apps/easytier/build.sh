#!/bin/bash
set -e
VER="2.6.4"

case "${ARCH}" in
  amd64|x86_64) ET_ARCH="x86_64" ;;
  arm64|aarch64) ET_ARCH="aarch64" ;;
  arm|armhf) ET_ARCH="armhf" ;;
  armv7) ET_ARCH="armv7" ;;
  riscv64) ET_ARCH="riscv64" ;;
  loongarch64) ET_ARCH="loongarch64" ;;
  *) ET_ARCH="${ARCH}" ;;
esac

echo "  downloading easytier v${VER} (${ET_ARCH})..."

URL="https://github.com/EasyTier/EasyTier/releases/download/v${VER}/easytier-linux-${ET_ARCH}-v${VER}.zip"
TMP="$BUILD_DIR/easytier.zip"

# Try to download from GitHub
download_ok=0
if curl -fsSL --connect-timeout 10 -o "$TMP" "$URL" 2>/dev/null; then
    download_ok=1
    echo "    downloaded from GitHub"
fi

# If download failed, look for local binaries in the workspace
if [ "$download_ok" = "0" ]; then
    echo "    download failed, looking for local binaries..."
    LOCAL_BIN_DIRS="/Users/dukangxu/nuln/diy4nas/fnos/apps/easytier/test /tmp/easytier-extract/usr/bin /Users/dukangxu/nuln/diy4nas"
    found=0
    for dir in $LOCAL_BIN_DIRS; do
        if [ -f "$dir/easytier-core" ] && [ -f "$dir/easytier-cli" ]; then
            echo "    found local binaries in: $dir"
            cp "$dir/easytier-core" "$BUILD_DIR/easytier-core"
            cp "$dir/easytier-cli" "$BUILD_DIR/easytier-cli"
            found=1
            break
        fi
    done
    if [ "$found" = "0" ]; then
        echo "    ⚠️  easytier-core and easytier-cli not found locally"
        echo "    please place them in $BUILD_DIR or run on a system with internet access"
    fi
else
    if [ -f "$TMP" ]; then
        unzip -o "$TMP" -d "$BUILD_DIR" >/dev/null 2>&1
        mv "$BUILD_DIR/easytier-linux-"*/* "$BUILD_DIR/" 2>/dev/null || true
        rm -f "$TMP"
    fi
fi

# Always ensure binaries are executable
chmod +x "$BUILD_DIR/easytier-core" 2>/dev/null || echo "  ⚠️  easytier-core not found in download"
chmod +x "$BUILD_DIR/easytier-cli" 2>/dev/null || echo "  ⚠️  easytier-cli not found in download"

echo "  easytier binaries ready"
ls -la "$BUILD_DIR/" 2>/dev/null | grep -v "^total"