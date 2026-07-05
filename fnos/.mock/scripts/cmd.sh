#!/bin/bash
# 模拟 fnOS 的 cmd/main start 调用
set -e

SLUG="$1"
ACTION="${2:-start}"

if [ -z "$SLUG" ]; then
    echo "Usage: $0 <slug> [start|stop|status|restart]"
    exit 1
fi

APP_DEST="/var/apps/$SLUG"
APP_VAR="/var/apps/$SLUG/data"

export TRIM_APPDEST="$APP_DEST"
export TRIM_PKGVAR="$APP_VAR"
export TRIM_TEMP_LOGFILE="/tmp/${SLUG}-cmd.log"

"$APP_DEST/cmd/main" "$ACTION"
