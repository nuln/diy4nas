#!/bin/bash
# release.sh — 构建 fpk 并发布到 GitHub Releases
# 需要: gh (GitHub CLI) 已登录
# Usage: bash release.sh v1.0.0
set -euo pipefail

TAG="${1:-v1.0.0}"
REPO="dukangxu/diy4nas"
echo "=== Release $TAG ==="

# 1. 构建 fpk
echo "[1/4] Building fpk..."
bash build.sh

# 2. 更新 fnpack.json 中的版本号
echo "[2/4] Updating version to $TAG..."
python3 -c "
import json
with open('fnpack.json') as f:
    d = json.load(f)
ver = '$TAG'.lstrip('v')
for a in d['apps']:
    a['version'] = ver
    a['fpk_url'] = 'https://github.com/$REPO/releases/download/$TAG/' + a['slug'] + '-' + ver + '.fpk'
with open('fnpack.json', 'w') as f:
    json.dump(d, f, ensure_ascii=False, indent=2)
"

# 3. 创建 GitHub Release
echo "[3/4] Creating GitHub Release..."
gh release create "$TAG" \
    --title "v$TAG" \
    --notes "diy4nas 应用包发布" \
    dist/*.fpk \
    fnpack.json

# 4. 验证
echo "[4/4] Done!"
echo "Release: https://github.com/$REPO/releases/tag/$TAG"
echo "App Center 源 URL: https://raw.githubusercontent.com/$REPO/main/fnos/fnpack.json"
