#!/bin/bash
# update_easytier.sh - Build easytier fpk
[ $# -lt 1 ] && echo "Usage: bash update_easytier.sh <version>" && exit 1
VERSION="$1"
echo "Building easytier v$VERSION..."
# TODO: implement download + build logic
