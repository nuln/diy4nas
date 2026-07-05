#!/bin/bash
# EasyTier plugin - 一键安装脚本
# 在 unRAID 终端以 root 运行

set -e

NAME="easytier"
VERSION="2026.06.07"
PLUGIN_DIR="/boot/config/plugins/$NAME"
EMHTTP_DIR="/usr/local/emhttp/plugins/$NAME"
BRANCH="master"
BASE="https://raw.githubusercontent.com/nuln/nas/$BRANCH/unraid/plugins/$NAME"

echo "=== 安装 EasyTier 插件 ==="

echo "下载二进制文件..."
wget -q -O "$PLUGIN_DIR/$NAME-$VERSION.tgz" "$BASE/$NAME-$VERSION.tgz"
installpkg "$PLUGIN_DIR/$NAME-$VERSION.tgz"

echo "下载脚本文件..."
mkdir -p "$EMHTTP_DIR"
mkdir -p "$PLUGIN_DIR"

wget -q -O /etc/rc.d/rc.$NAME "$BASE/rc.$NAME"
wget -q -O "$PLUGIN_DIR/event.disks_mounted" "$BASE/event.disks_mounted"
wget -q -O "$EMHTTP_DIR/easytier.page" "$BASE/easytier.page"
wget -q -O "$EMHTTP_DIR/api.php" "$BASE/api.php"
wget -q -O "$EMHTTP_DIR/style.css" "$BASE/style.css"
wget -q -O "$PLUGIN_DIR/icon.png" "$BASE/logo.png"
wget -q -O "$EMHTTP_DIR/images/$NAME.png" "$BASE/logo.png"

cp "$PLUGIN_DIR/event.disks_mounted" "$EMHTTP_DIR/event/disks_mounted"

chmod +x /etc/rc.d/rc.$NAME
chmod +x "$EMHTTP_DIR/api.php"
chmod +x "$EMHTTP_DIR/event/disks_mounted"

if [ ! -f "$PLUGIN_DIR/easytier.conf" ]; then
cat > "$PLUGIN_DIR/easytier.conf" << 'CONF_EOF'
# EasyTier plugin configuration
NETWORK_NAME="easytier"
NETWORK_SECRET=""
DHCP="yes"
VIRTUAL_IPV4=""
HOSTNAME=""
PEER_URLS=""
LISTENER_URLS="tcp://0.0.0.0:11010, udp://0.0.0.0:11010"
PROXY_CIDRS=""
AUTOSTART="yes"
CONF_EOF
fi

/etc/rc.d/rc.$NAME start

echo ""
echo "=== 安装完成！==="
echo "请前往 unRAID WebUI → Settings → EasyTier"
echo ""
