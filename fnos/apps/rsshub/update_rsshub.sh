#!/bin/bash
# update_rsshub.sh - Build rsshub fpk
[ $# -lt 1 ] && echo "Usage: bash update_rsshub.sh <version>" && exit 1
VERSION="$1"
echo "Building rsshub v$VERSION..."
# TODO: implement download + build logic
