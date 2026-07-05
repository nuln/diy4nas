#!/bin/bash
# update_transmission.sh - Build transmission fpk
[ $# -lt 1 ] && echo "Usage: bash update_transmission.sh <version>" && exit 1
VERSION="$1"
echo "Building transmission v$VERSION..."
# TODO: implement download + build logic
