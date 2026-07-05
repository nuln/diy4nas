#!/bin/bash
# oh-my-zsh plugin - 一键安装脚本
# 在 unRAID 终端以 root 运行

set -e

NAME="ohmyzsh"
PLUGIN_DIR="/boot/config/plugins/$NAME"
EMHTTP_DIR="/usr/local/emhttp/plugins/$NAME"
BRANCH="master"
BASE="https://raw.githubusercontent.com/nuln/nas/$BRANCH/unraid/plugins/$NAME"

echo "=== 安装 oh-my-zsh 插件 ==="

mkdir -p "$EMHTTP_DIR"
mkdir -p "$EMHTTP_DIR/event"
mkdir -p "$EMHTTP_DIR/images"
mkdir -p "$PLUGIN_DIR"

echo "下载脚本文件..."
wget -q -O /etc/rc.d/rc.$NAME "$BASE/rc.$NAME"
wget -q -O "$PLUGIN_DIR/event.disks_mounted" "$BASE/event.disks_mounted"
cp "$PLUGIN_DIR/event.disks_mounted" "$EMHTTP_DIR/event/disks_mounted"

wget -q -O "$EMHTTP_DIR/ohmyzsh.page" "$BASE/ohmyzsh.page"
wget -q -O "$EMHTTP_DIR/api.php" "$BASE/api.php"
wget -q -O "$EMHTTP_DIR/style.css" "$BASE/style.css"
wget -q -O "$EMHTTP_DIR/ohmyzsh.js" "$BASE/ohmyzsh.js"
wget -q -O "$PLUGIN_DIR/icon.png" "$BASE/logo.png"
wget -q -O "$EMHTTP_DIR/images/$NAME.png" "$BASE/logo.png"

chmod +x /etc/rc.d/rc.$NAME
chmod +x "$EMHTTP_DIR/api.php"
chmod +x "$EMHTTP_DIR/event/disks_mounted"

if [ ! -f "$PLUGIN_DIR/ohmyzsh.conf" ]; then
cat > "$PLUGIN_DIR/ohmyzsh.conf" << 'CONF'
# oh-my-zsh plugin configuration
AUTOSTART="yes"
ZSH_THEME="robbyrussell"
ZSH_PLUGINS="git"
CUSTOM_ALIASES=""
CONF
fi

echo "安装 oh-my-zsh..."
/etc/rc.d/rc.$NAME install

echo ""
echo "=== 安装完成！==="
echo "请前往 unRAID WebUI → Settings → oh-my-zsh"
echo "配置主题和插件，或直接运行 'zsh' 开始使用。"
echo ""
