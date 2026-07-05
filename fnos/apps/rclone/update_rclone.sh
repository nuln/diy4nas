#!/bin/bash
# update_rclone.sh - Build rclone fpk
[ $# -lt 1 ] && echo "Usage: bash update_rclone.sh <version>" && exit 1
VERSION="$1"
echo "Building rclone v$VERSION..."
# TODO: implement download + build logic
