#!/bin/bash
# 更新 scheduler: 重新下载 Go 模块并编译
set -e
BASE="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$BASE/apps/scheduler/www"
echo "[UPDATE] scheduler: tidying modules..."
go mod tidy
echo "[UPDATE] scheduler: done (rebuild via build.sh)"
