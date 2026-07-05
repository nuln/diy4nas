#!/bin/bash
# update_redis.sh - Build redis fpk
[ $# -lt 1 ] && echo "Usage: bash update_redis.sh <version>" && exit 1
VERSION="$1"
echo "Building redis v$VERSION..."
# TODO: implement download + build logic
