#!/bin/bash
# update_cloudflared.sh - Build cloudflared fpk
[ $# -lt 1 ] && echo "Usage: bash update_cloudflared.sh <version>" && exit 1
VERSION="$1"
echo "Building cloudflared v$VERSION..."
# TODO: implement download + build logic
