#!/bin/bash
# update_syncthing.sh - Build syncthing fpk
[ $# -lt 1 ] && echo "Usage: bash update_syncthing.sh <version>" && exit 1
VERSION="$1"
echo "Building syncthing v$VERSION..."
# TODO: implement download + build logic
