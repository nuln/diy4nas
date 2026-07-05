#!/bin/bash
# update_garage.sh - Build garage fpk
[ $# -lt 1 ] && echo "Usage: bash update_garage.sh <version>" && exit 1
VERSION="$1"
echo "Building garage v$VERSION..."
# TODO: implement download + build logic
