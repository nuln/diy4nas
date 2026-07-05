#!/bin/bash
# update-all.sh - Build all apps and prepare fpk files for App Center
set -euo pipefail

FNOS_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "========================================"
echo "  diy4nas - Build All Apps"
echo "========================================"

# Build tailscale if version specified
if [ $# -ge 1 ]; then
    TS_VERSION="$1"
    echo ""
    echo "[1/2] Building tailscale v${TS_VERSION}..."
    bash "$FNOS_DIR/apps/tailscale/update_tailscale.sh" "$TS_VERSION"
else
    echo ""
    echo "[1/2] Skipping tailscale (no version specified)"
    echo "  Usage: bash fnos/update-all.sh <tailscale-version>"
    echo "  Example: bash fnos/update-all.sh 1.80.0"
fi

echo ""
echo "[2/2] Building App Center..."
bash "$FNOS_DIR/apps/app-center/update.sh"

echo ""
echo "========================================"
echo "  Build complete!"
echo "========================================"
echo ""
echo "FPK files:"
ls -lh "$FNOS_DIR"/apps/*/*.fpk 2>/dev/null || echo "  (none)"
echo ""
echo "Next steps:"
echo "  1. Install App Center fpk on fnOS"
echo "  2. Copy other app fpk files to App Center data/fpk/"
echo "  3. Access App Center at http://<nas-ip>:8011"
