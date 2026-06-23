#!/bin/sh
# eventhook 升级更新脚本
set -e
APP_SLUG="eventhook"
APP_DEST="/vol1/@appcenter/$APP_SLUG"
APP_VAR="/vol1/@appdata/$APP_SLUG"

echo "Stopping $APP_SLUG..."
"$APP_DEST/cmd/main" stop 2>/dev/null || true
sleep 1

echo "Updating binary..."
cp /tmp/fnos-eventhook "$APP_DEST/app/fnos-eventhook"
chmod 0755 "$APP_DEST/app/fnos-eventhook"

echo "Starting $APP_SLUG..."
"$APP_DEST/cmd/main" start
echo "Done"
