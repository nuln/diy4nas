#!/bin/bash
# 更新 terminal: 重新下载 Go 模块
set -e
BASE="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$BASE/apps/terminal/www"
echo "[UPDATE] terminal: tidying modules..."
go mod tidy
echo "[UPDATE] terminal: done (rebuild via build.sh, CGO=1)"
