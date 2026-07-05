#!/bin/bash
# update_wg-easy.sh - Build wg-easy fpk
[ $# -lt 1 ] && echo "Usage: bash update_wg-easy.sh <version>" && exit 1
VERSION="$1"
echo "Building wg-easy v$VERSION..."
# TODO: implement download + build logic
