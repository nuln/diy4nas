#!/bin/bash
# 卸载流程模拟
set -e
SLUG="$1"

if [ -z "$SLUG" ]; then
    echo "Usage: $0 <slug>"
    exit 1
fi

APP_DEST="/var/apps/$SLUG"
APP_VAR="/var/apps/$SLUG/data"

# Stop first
if [ -x "$APP_DEST/cmd/main" ]; then
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-uninstall.log" \
        "$APP_DEST/cmd/main" stop 2>/dev/null || true
fi

# Run service-setup service_preuninst
if [ -x "$APP_DEST/cmd/service-setup" ]; then
    TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" \
        bash -c "source $APP_DEST/cmd/service-setup; service_preuninst" 2>/dev/null || true
fi

# Run uninstall hooks
for hook in uninstall_init uninstall_callback; do
    if [ -x "$APP_DEST/cmd/$hook" ]; then
        echo "[UNINSTALL] running $hook..."
        TRIM_APPDEST="$APP_DEST" TRIM_PKGVAR="$APP_VAR" TRIM_TEMP_LOGFILE="/tmp/${SLUG}-uninstall.log" \
            "$APP_DEST/cmd/$hook"
    fi
done

# Optional: remove data (we keep it for re-install test)
# rm -rf "$APP_VAR"

# Remove app
rm -rf "$APP_DEST"
echo "[UNINSTALL] $SLUG removed"
