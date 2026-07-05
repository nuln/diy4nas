#!/bin/bash
# update_freshrss.sh - Build freshrss fpk
[ $# -lt 1 ] && echo "Usage: bash update_freshrss.sh <version>" && exit 1
VERSION="$1"
echo "Building freshrss v$VERSION..."
# TODO: implement download + build logic
