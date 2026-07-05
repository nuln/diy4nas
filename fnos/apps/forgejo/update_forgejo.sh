#!/bin/bash
# update_forgejo.sh - Build forgejo fpk
[ $# -lt 1 ] && echo "Usage: bash update_forgejo.sh <version>" && exit 1
VERSION="$1"
echo "Building forgejo v$VERSION..."
# TODO: implement download + build logic
