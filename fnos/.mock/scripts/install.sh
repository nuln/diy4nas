#!/bin/bash
# 模拟 fnOS 的 FPK 安装流程
set -e

FPK_FILE="$1"
SLUG="$2"

if [ -z "$FPK_FILE" ] || [ -z "$SLUG" ]; then
    echo "Usage: $0 <fpk-file> <slug>"
    exit 1
fi

if [ ! -f "$FPK_FILE" ]; then
    echo "ERROR: FPK file not found: $FPK_FILE"
    exit 1
fi

APPS_ROOT="/var/apps"
APP_DEST="$APPS_ROOT/$SLUG"
APP_VAR="$APPS_ROOT/$SLUG/data"

# Stop existing if running
if [ -x "$APP_DEST/cmd/main" ]; then
    echo "[INSTALL] stopping existing instance of $SLUG..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-install.log" \
        "$APP_DEST/cmd/main" stop 2>/dev/null || true
fi

# Clean previous install
rm -rf "$APP_DEST"
mkdir -p "$APPS_ROOT" "$APP_VAR"

# Extract FPK
echo "[INSTALL] extracting $FPK_FILE to $APP_DEST..."
mkdir -p /tmp/fpk-extract
rm -rf /tmp/fpk-extract/*
tar xzf "$FPK_FILE" -C /tmp/fpk-extract

# Move files to APP_DEST
cp -r /tmp/fpk-extract/cmd "$APP_DEST/"
[ -d /tmp/fpk-extract/config ] && cp -r /tmp/fpk-extract/config "$APP_DEST/"
[ -f /tmp/fpk-extract/manifest ] && cp /tmp/fpk-extract/manifest "$APP_DEST/"
[ -d /tmp/fpk-extract/wizard ] && cp -r /tmp/fpk-extract/wizard "$APP_DEST/" 2>/dev/null || true
[ -f /tmp/fpk-extract/${SLUG}.sc ] && cp /tmp/fpk-extract/${SLUG}.sc "$APP_DEST/"

# Extract app.tgz (contains app/server, app/ui, app/bin, etc.)
mkdir -p "$APP_DEST"
tar xzf /tmp/fpk-extract/app.tgz -C "$APP_DEST" 2>/dev/null || true

# Set permissions
chmod +x "$APP_DEST/cmd/"* 2>/dev/null || true
chmod +x "$APP_DEST/app/server" 2>/dev/null || true

# Setup user/group (if config/privilege specifies one)
if [ -f "$APP_DEST/config/privilege" ]; then
    USERNAME=$(python3 -c "import json; d=json.load(open('$APP_DEST/config/privilege')); print(d.get('username', '$SLUG'))")
    GROUPNAME=$(python3 -c "import json; d=json.load(open('$APP_DEST/config/privilege')); print(d.get('groupname', '$SLUG'))")
    if ! id -u "$USERNAME" >/dev/null 2>&1; then
        useradd -r -s /usr/sbin/nologin "$USERNAME" 2>/dev/null || true
    fi
fi

# Run install_init
if [ -x "$APP_DEST/cmd/install_init" ]; then
    echo "[INSTALL] running install_init..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-install.log" \
        "$APP_DEST/cmd/install_init"
fi

# Run install_callback
if [ -x "$APP_DEST/cmd/install_callback" ]; then
    echo "[INSTALL] running install_callback..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-install.log" \
        "$APP_DEST/cmd/install_callback"
fi

# Run service-setup service_postinst (if file exists)
if [ -x "$APP_DEST/cmd/service-setup" ]; then
    echo "[INSTALL] running service-setup service_postinst..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" \
        bash -c "source $APP_DEST/cmd/service-setup; service_postinst" 2>/dev/null || true
fi

# Set ownership (run-as: root still owns files; but data dir can be per-user)
if id -u "$USERNAME" >/dev/null 2>&1; then
    chown -R "$USERNAME:$GROUPNAME" "$APP_DEST/data" 2>/dev/null || true
fi

echo "[INSTALL] $SLUG installed to $APP_DEST"
echo "---"
ls -la "$APP_DEST"
echo "--- cmd ---"
ls -la "$APP_DEST/cmd/"
