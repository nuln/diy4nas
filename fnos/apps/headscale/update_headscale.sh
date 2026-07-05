#!/bin/bash
# update_headscale.sh - Build headscale fpk
[ $# -lt 1 ] && echo "Usage: bash update_headscale.sh <version>" && exit 1
VERSION="$1"
echo "Building headscale v$VERSION..."
# TODO: implement download + build logic
