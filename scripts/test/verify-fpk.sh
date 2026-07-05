#!/bin/bash
# L2 contract verification for a built .fpk artifact.
#
# Validates structural and metadata properties WITHOUT installing/starting the
# app. Designed to run inside the CI build job, between 'build-fpk.sh' and
# 'release create'.
#
# Usage:
#   scripts/test/verify-fpk.sh <fpk-path> [<slug>]
#
# If <slug> is omitted, it is inferred from the fpk filename
# (`<slug>_<version>_<platform>.fpk`).
#
# Exits 0 on pass, 1 on fail.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

require_cmd jq
require_cmd tar
require_cmd file

FPK_PATH="${1:-}"
SLUG_OVERRIDE="${2:-}"

[ -n "$FPK_PATH" ] || error "Usage: $0 <fpk-path> [<slug>]"
[ -f "$FPK_PATH" ] || error "fpk file not found: $FPK_PATH"

FPK_NAME="$(basename "$FPK_PATH")"

# Parse name: <prefix>_<version>_<platform>.fpk — platform is the LAST segment.
SLUG_FROM_NAME=""
PLATFORM_FROM_NAME=""
if [[ "$FPK_NAME" =~ ^(.+)_([^_]+)_(x86|arm)\.fpk$ ]]; then
    SLUG_FROM_NAME="${BASH_REMATCH[1]}"
    PLATFORM_FROM_NAME="${BASH_REMATCH[3]}"
fi

SLUG="${SLUG_OVERRIDE:-$SLUG_FROM_NAME}"
if [ -z "$SLUG" ]; then
    error "cannot infer slug from filename '$FPK_NAME'; pass it explicitly"
fi
info "Verifying $FPK_NAME (slug=$SLUG, platform=$PLATFORM_FROM_NAME)"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

# ---------------------------------------------------------------------------
# Checks
# ---------------------------------------------------------------------------

check_is_gzip_tar() {
    if ! file -b "$FPK_PATH" | grep -qE 'gzip compressed|gzip data'; then
        fail "fpk is not a gzip-compressed file"
        return 1
    fi
    if ! tar -tzf "$FPK_PATH" >/dev/null 2>&1; then
        fail "fpk is not a valid tar.gz archive"
        return 1
    fi
    pass "fpk is a valid tar.gz"
}

check_required_entries() {
    local required=(
        manifest
        app.tgz
        cmd/main
        cmd/common
        cmd/installer
        cmd/install_init
        cmd/install_callback
        cmd/uninstall_init
        cmd/uninstall_callback
        cmd/upgrade_init
        cmd/upgrade_callback
        ICON.PNG
        ICON_256.PNG
    )
    local listing
    listing="$(tar -tzf "$FPK_PATH")"
    local missing=()
    for entry in "${required[@]}"; do
        if ! grep -qxF "$entry" <<<"$listing"; then
            missing+=("$entry")
        fi
    done
    if [ "${#missing[@]}" -gt 0 ]; then
        fail "fpk missing required entries: ${missing[*]}"
        return 1
    fi
    pass "fpk contains all required entries"
}

extract_fpk() {
    mkdir -p "$WORK_DIR/extracted"
    if ! tar -xzf "$FPK_PATH" -C "$WORK_DIR/extracted" 2>/dev/null; then
        fail "fpk extraction failed"
        return 1
    fi
    pass "fpk extracts cleanly"
}

check_manifest_fields() {
    local manifest="$WORK_DIR/extracted/manifest"
    [ -f "$manifest" ] || { fail "manifest missing in extracted fpk"; return 1; }

    local appname version platform port checksum source
    appname="$(manifest_get "$manifest" appname)"
    version="$(manifest_get "$manifest" version)"
    platform="$(manifest_get "$manifest" platform)"
    port="$(manifest_get "$manifest" service_port)"
    checksum="$(manifest_get "$manifest" checksum)"
    source="$(manifest_get "$manifest" source)"

    local ok=1
    if [ -z "$appname" ];  then fail "manifest.appname empty"; ok=0; fi
    if [ -z "$version" ];  then fail "manifest.version empty"; ok=0; fi
    if [ -z "$platform" ]; then fail "manifest.platform empty"; ok=0; fi
    if [ -z "$port" ];     then fail "manifest.service_port empty"; ok=0; fi
    if [ -z "$checksum" ]; then fail "manifest.checksum empty"; ok=0; fi
    if [ -z "$source" ];   then fail "manifest.source empty"; ok=0; fi

    if [ "$appname" != "$SLUG" ] && [ "$appname" != "${SLUG//-/_}" ]; then
        warn "manifest.appname='$appname' differs from slug '$SLUG' (legacy quirk?)"
    fi

    if [ -n "$PLATFORM_FROM_NAME" ] && [ "$platform" != "$PLATFORM_FROM_NAME" ]; then
        fail "manifest.platform='$platform' but filename says '$PLATFORM_FROM_NAME'"
        ok=0
    fi

    [ "$ok" -eq 1 ] && pass "manifest fields consistent (appname=$appname version=$version platform=$platform port=$port)"
    return $((1 - ok))
}

check_app_tgz_checksum() {
    local manifest="$WORK_DIR/extracted/manifest"
    local app_tgz="$WORK_DIR/extracted/app.tgz"
    [ -f "$manifest" ] || return 1
    [ -f "$app_tgz" ] || { fail "app.tgz missing in extracted fpk"; return 1; }

    local declared computed
    declared="$(manifest_get "$manifest" checksum)"
    computed="$(portable_md5 "$app_tgz")"
    if [ "$declared" != "$computed" ]; then
        fail "checksum mismatch: manifest.checksum='$declared' but md5(app.tgz)='$computed'"
        return 1
    fi
    pass "checksum matches md5(app.tgz)"
}

check_app_tgz_arch() {
    local app_tgz="$WORK_DIR/extracted/app.tgz"
    [ -f "$app_tgz" ] || return 1

    mkdir -p "$WORK_DIR/app"
    if ! tar -xzf "$app_tgz" -C "$WORK_DIR/app" 2>/dev/null; then
        fail "app.tgz extraction failed"
        return 1
    fi

    if [ -f "$WORK_DIR/app/docker-compose.yaml" ] || [ -d "$WORK_DIR/app/docker" ]; then
        pass "docker app — skipping native arch check"
        return 0
    fi

    local elf_files
    elf_files="$(find "$WORK_DIR/app" -type f -exec file -b {} \; 2>/dev/null \
                 | grep -c '^ELF ' || true)"
    if [ "${elf_files:-0}" -eq 0 ]; then
        warn "no ELF binaries in app.tgz (interpreter-only app? Python/Java/.NET?)"
        return 0
    fi

    local expected
    case "$PLATFORM_FROM_NAME" in
        x86) expected='x86-64' ;;
        arm) expected='aarch64' ;;
        *)   warn "unknown platform '$PLATFORM_FROM_NAME', skipping arch check"; return 0 ;;
    esac

    local wrong
    wrong="$(find "$WORK_DIR/app" -type f -exec file -b {} \; 2>/dev/null \
             | grep '^ELF ' | grep -v "$expected" || true)"
    if [ -n "$wrong" ]; then
        local sample
        sample="$(echo "$wrong" | head -3)"
        fail "ELF binaries with wrong arch (expected '$expected'):"$'\n'"$sample"
        return 1
    fi
    pass "all ELF binaries match expected arch '$expected'"
}

check_docker_image_exists() {
    local app_tgz_dir="$WORK_DIR/app"
    local compose=""
    if [ -f "$app_tgz_dir/docker-compose.yaml" ]; then
        compose="$app_tgz_dir/docker-compose.yaml"
    elif [ -f "$app_tgz_dir/docker/docker-compose.yaml" ]; then
        compose="$app_tgz_dir/docker/docker-compose.yaml"
    else
        return 0
    fi

    local images
    images="$(grep -E '^[[:space:]]*image:' "$compose" | sed -E 's/^[[:space:]]*image:[[:space:]]*//; s/^"//; s/"$//' || true)"
    if [ -z "$images" ]; then
        warn "docker app but no 'image:' directives found in compose"
        return 0
    fi

    if ! has_cmd docker; then
        debug "no docker CLI; skipping image existence check"
        return 0
    fi

    # Detect timeout wrapper (macOS lacks `timeout` by default; coreutils ships `gtimeout`)
    local timeout_cmd=""
    if command -v timeout >/dev/null 2>&1; then
        timeout_cmd="timeout 30"
    elif command -v gtimeout >/dev/null 2>&1; then
        timeout_cmd="gtimeout 30"
    fi

    while IFS= read -r raw_image; do
        local img="${raw_image//\$\{DOCKER_MIRROR\}/}"
        img="${img//\$\{VERSION\}/latest}"
        img="${img//\$\{TAG\}/latest}"
        if [[ "$img" == *'${'* ]]; then
            debug "image '$raw_image' has unresolved variables; skipping"
            continue
        fi
        if $timeout_cmd docker manifest inspect "$img" >/dev/null 2>&1; then
            pass "docker image accessible: $img"
        else
            fail "docker image NOT found in registry: $img (raw: $raw_image)"
        fi
    done <<<"$images"
}

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------

check_is_gzip_tar || error "fpk is structurally broken; aborting further checks"
check_required_entries
extract_fpk      || error "fpk cannot be extracted; aborting further checks"
check_manifest_fields
check_app_tgz_checksum
check_app_tgz_arch
check_docker_image_exists

report_summary "verify-fpk:$FPK_NAME"
