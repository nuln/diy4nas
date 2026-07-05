#!/bin/bash
# update_jellyfin.sh - Build jellyfin fpk
[ $# -lt 1 ] && echo "Usage: bash update_jellyfin.sh <version>" && exit 1
VERSION="$1"
echo "Building jellyfin v$VERSION..."
# TODO: implement download + build logic
