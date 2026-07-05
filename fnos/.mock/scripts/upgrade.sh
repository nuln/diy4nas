#!/bin/bash
# 升级流程模拟
set -e
FPK_FILE="$1"
SLUG="$2"

if [ -z "$FPK_FILE" ] || [ -z "$SLUG" ]; then
    echo "Usage: $0 <fpk-file> <slug>"
    exit 1
fi

APP_DEST="/var/apps/$SLUG"
APP_VAR="/var/apps/$SLUG/data"

# Backup data
if [ -d "$APP_VAR" ]; then
    cp -r "$APP_VAR" /tmp/${SLUG}-data-backup
fi

# Run upgrade hooks BEFORE replacing
if [ -x "$APP_DEST/cmd/upgrade_init" ]; then
    echo "[UPGRADE] running upgrade_init..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-upgrade.log" \
        "$APP_DEST/cmd/upgrade_init"
fi

if [ -x "$APP_DEST/cmd/upgrade_callback" ]; then
    echo "[UPGRADE] running upgrade_callback (this stops the old service)..."
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-upgrade.log" \
        "$APP_DEST/cmd/upgrade_callback"
fi

# Re-extract
rm -rf /tmp/fpk-extract
mkdir -p /tmp/fpk-extract
tar xzf "$FPK_FILE" -C /tmp/fpk-extract

# Preserve data
rm -rf "$APP_VAR"
mv /tmp/${SLUG}-data-backup "$APP_VAR"

# Remove old app dir
rm -rf "$APP_DEST"

# Re-install
mkdir -p "$APP_DEST"
cp -r /tmp/fpk-extract/cmd "$APP_DEST/"
[ -d /tmp/fpk-extract/config ] && cp -r /tmp/fpk-extract/config "$APP_DEST/"
[ -f /tmp/fpk-extract/manifest ] && cp /tmp/fpk-extract/manifest "$APP_DEST/"
[ -f /tmp/fpk-extract/${SLUG}.sc ] && cp /tmp/fpk-extract/${SLUG}.sc "$APP_DEST/"
tar xzf /tmp/fpk-extract/app.tgz -C "$APP_DEST" 2>/dev/null || true

chmod +x "$APP_DEST/cmd/"* 2>/dev/null || true
chmod +x "$APP_DEST/app/server" 2>/dev/null || true

echo "[UPGRADE] $SLUG upgraded"
