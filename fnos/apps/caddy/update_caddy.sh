#!/bin/bash
# update_caddy.sh - Build caddy fpk
[ $# -lt 1 ] && echo "Usage: bash update_caddy.sh <version>" && exit 1
VERSION="$1"
echo "Building caddy v$VERSION..."
# TODO: implement download + build logic
