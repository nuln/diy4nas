#!/bin/bash
# update_tailscale.sh - Build tailscale fpk
[ $# -lt 1 ] && echo "Usage: bash update_tailscale.sh <version>" && exit 1
VERSION="$1"
echo "Building tailscale v$VERSION..."
# TODO: implement download + build logic
