#!/bin/bash
# Homebrew plugin - 一键安装脚本
# 在 unRAID 终端以 root 运行

set -e

NAME="homebrew"
VERSION="2026.06.12"
PLUGIN_DIR="/boot/config/plugins/$NAME"
EMHTTP_DIR="/usr/local/emhttp/plugins/$NAME"
BRANCH="master"
BASE="https://raw.githubusercontent.com/nuln/nas/$BRANCH/unraid/plugins/$NAME"

echo "=== 安装 Homebrew 插件 ==="

echo "下载脚本和WebUI文件..."
mkdir -p "$EMHTTP_DIR"
mkdir -p "$EMHTTP_DIR/event"
mkdir -p "$EMHTTP_DIR/images"
mkdir -p "$PLUGIN_DIR"

wget -q -O /etc/rc.d/rc.$NAME "$BASE/rc.$NAME"
wget -q -O "$PLUGIN_DIR/event.disks_mounted" "$BASE/event.disks_mounted"
wget -q -O "$EMHTTP_DIR/homebrew.page" "$BASE/homebrew.page"
wget -q -O "$EMHTTP_DIR/homebrew-1-Settings.page" "$BASE/homebrew-1-Settings.page"
wget -q -O "$EMHTTP_DIR/homebrew-2-Packages.page" "$BASE/homebrew-2-Packages.page"
wget -q -O "$EMHTTP_DIR/api.php" "$BASE/api.php"
wget -q -O "$EMHTTP_DIR/style.css" "$BASE/style.css"
wget -q -O "$EMHTTP_DIR/homebrew.js" "$BASE/homebrew.js"
wget -q -O "$PLUGIN_DIR/icon.png" "$BASE/logo.png"
wget -q -O "$EMHTTP_DIR/images/$NAME.png" "$BASE/logo.png"

cp "$PLUGIN_DIR/event.disks_mounted" "$EMHTTP_DIR/event/disks_mounted"

chmod +x /etc/rc.d/rc.$NAME
chmod +x "$EMHTTP_DIR/api.php"
chmod +x "$EMHTTP_DIR/event/disks_mounted"

if [ ! -f "$PLUGIN_DIR/homebrew.conf" ]; then
cat > "$PLUGIN_DIR/homebrew.conf" << 'CONF_EOF'
# Homebrew plugin configuration
BREW_STORAGE="/boot/config/plugins/homebrew/linuxbrew"
AUTOSTART="yes"
SHELL_INTEGRATION="bash"
GCC_AUTOINSTALL="yes"
CONF_EOF
fi

echo "正在安装 Homebrew..."
/etc/rc.d/rc.$NAME install

echo ""
echo "=== 安装完成！==="
echo "请前往 unRAID WebUI → User Utilities → Homebrew"
echo ""
