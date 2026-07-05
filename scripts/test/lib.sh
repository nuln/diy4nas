#!/bin/bash
# Shared library for fnos-apps test scripts.
#
# Provides:
#   - Coloured logging: info(), warn(), error(), fail()
#   - Pass/fail accumulator: assert_*(), report_summary()
#   - Repo helpers: repo_root(), list_apps(), app_dir(), scripts_app_dir()
#   - Manifest parsing: manifest_get(), manifest_keys()
#   - Cross-platform helpers: portable_stat_size(), portable_md5()
#
# Sourced by every scripts/test/*.sh. Self-contained, no external deps beyond
# coreutils + jq.

# Guard against double-sourcing.
[ -n "${_FNOS_TEST_LIB_LOADED:-}" ] && return 0
_FNOS_TEST_LIB_LOADED=1

# ---------------------------------------------------------------------------
# Colour output
# ---------------------------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    _C_RED=$'\033[0;31m'
    _C_GREEN=$'\033[0;32m'
    _C_YELLOW=$'\033[1;33m'
    _C_BLUE=$'\033[0;34m'
    _C_DIM=$'\033[2m'
    _C_NC=$'\033[0m'
else
    _C_RED=""
    _C_GREEN=""
    _C_YELLOW=""
    _C_BLUE=""
    _C_DIM=""
    _C_NC=""
fi

info()  { printf '%s[INFO]%s %s\n'  "$_C_GREEN"  "$_C_NC" "$*" >&2; }
warn()  { printf '%s[WARN]%s %s\n'  "$_C_YELLOW" "$_C_NC" "$*" >&2; }
debug() { [ -n "${TEST_DEBUG:-}" ] && printf '%s[DEBUG]%s %s\n' "$_C_DIM" "$_C_NC" "$*" >&2 || true; }
note()  { printf '%s[NOTE]%s %s\n'  "$_C_BLUE"   "$_C_NC" "$*" >&2; }

# error() prints + exits. fail() prints + records, but lets the script continue.
error() { printf '%s[ERROR]%s %s\n' "$_C_RED"    "$_C_NC" "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Pass/fail accumulator
# ---------------------------------------------------------------------------
_PASS_COUNT=0
_FAIL_COUNT=0
_FAIL_MESSAGES=()

pass() {
    _PASS_COUNT=$((_PASS_COUNT + 1))
    printf '%s  ✓%s %s\n' "$_C_GREEN" "$_C_NC" "$*" >&2
}

fail() {
    _FAIL_COUNT=$((_FAIL_COUNT + 1))
    _FAIL_MESSAGES+=("$*")
    printf '%s  ✗%s %s\n' "$_C_RED" "$_C_NC" "$*" >&2
}

# Returns 0 if all asserts passed, 1 if any failed.
report_summary() {
    local label="${1:-summary}"
    echo "" >&2
    echo "================================================================" >&2
    if [ "$_FAIL_COUNT" -eq 0 ]; then
        printf '%s[%s] PASS%s — %d checks\n' "$_C_GREEN" "$label" "$_C_NC" "$_PASS_COUNT" >&2
        echo "================================================================" >&2
        return 0
    fi
    printf '%s[%s] FAIL%s — %d passed, %d failed\n' \
        "$_C_RED" "$label" "$_C_NC" "$_PASS_COUNT" "$_FAIL_COUNT" >&2
    echo "" >&2
    echo "Failures:" >&2
    local i=1
    for msg in "${_FAIL_MESSAGES[@]}"; do
        printf '  %d. %s\n' "$i" "$msg" >&2
        i=$((i + 1))
    done
    echo "================================================================" >&2
    return 1
}


reset_accumulator() {
    _PASS_COUNT=0
    _FAIL_COUNT=0
    _FAIL_MESSAGES=()
}

# ---------------------------------------------------------------------------
# Repo layout helpers
# ---------------------------------------------------------------------------

# Find the repo root by walking up from the caller. Caches result.
repo_root() {
    if [ -n "${_FNOS_REPO_ROOT:-}" ]; then
        echo "$_FNOS_REPO_ROOT"
        return 0
    fi
    # Caller-provided override (e.g. for tests in temp dirs).
    if [ -n "${FNOS_REPO_ROOT:-}" ] && [ -d "${FNOS_REPO_ROOT}/apps" ]; then
        _FNOS_REPO_ROOT="$FNOS_REPO_ROOT"
        echo "$_FNOS_REPO_ROOT"
        return 0
    fi

    local d
    d="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    while [ "$d" != "/" ]; do
        if [ -d "$d/apps" ] && [ -d "$d/shared" ] && [ -d "$d/scripts" ]; then
            _FNOS_REPO_ROOT="$d"
            echo "$d"
            return 0
        fi
        d="$(dirname "$d")"
    done
    echo "ERROR: cannot locate fnos-apps repo root" >&2
    return 1
}

# list_apps — print one app slug per line.
list_apps() {
    local root
    root="$(repo_root)" || return 1
    find "$root/apps" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort
}

# app_dir <slug> — print the absolute path to apps/<slug>.
app_dir() {
    local slug="${1:?app_dir requires slug}"
    local root
    root="$(repo_root)" || return 1
    echo "$root/apps/$slug"
}

# scripts_app_dir <slug> — print the absolute path to scripts/apps/<slug>.
scripts_app_dir() {
    local slug="${1:?scripts_app_dir requires slug}"
    local root
    root="$(repo_root)" || return 1
    echo "$root/scripts/apps/$slug"
}

# ---------------------------------------------------------------------------
# Manifest parsing
# ---------------------------------------------------------------------------

# manifest_get <manifest-path> <key> — print value (trimmed) or empty.
manifest_get() {
    local manifest="${1:?manifest_get requires manifest path}"
    local key="${2:?manifest_get requires key}"
    awk -F= -v k="$key" '
        $1 ~ ("^"k"[[:space:]]*$") {
            sub(/^[[:space:]]+/, "", $2)
            sub(/[[:space:]]+$/, "", $2)
            print $2
            exit
        }
    ' "$manifest"
}

# manifest_keys <manifest-path> — print all keys, one per line.
manifest_keys() {
    local manifest="${1:?manifest_keys requires manifest path}"
    awk -F= '
        NF >= 2 && $1 !~ /^[[:space:]]*#/ {
            gsub(/[[:space:]]/, "", $1)
            if (length($1) > 0) print $1
        }
    ' "$manifest"
}

# is_docker_app <slug> — returns 0 if the app uses docker-compose, 1 otherwise.
is_docker_app() {
    local slug="${1:?is_docker_app requires slug}"
    local d
    d="$(app_dir "$slug")" || return 2
    [ -f "$d/fnos/docker/docker-compose.yaml" ]
}

# ---------------------------------------------------------------------------
# Cross-platform helpers (macOS BSD vs Linux GNU)
# ---------------------------------------------------------------------------

# portable_stat_size <path> — print byte size or "0".
portable_stat_size() {
    local f="${1:?path required}"
    stat -f%z "$f" 2>/dev/null || stat -c%s "$f" 2>/dev/null || echo 0
}

# portable_md5 <path> — print md5 hex string.
portable_md5() {
    local f="${1:?path required}"
    if command -v md5sum >/dev/null 2>&1; then
        md5sum "$f" | awk '{print $1}'
    elif command -v md5 >/dev/null 2>&1; then
        md5 -q "$f"
    else
        echo "ERROR: no md5 implementation found" >&2
        return 1
    fi
}

# ---------------------------------------------------------------------------
# Tool presence
# ---------------------------------------------------------------------------

# require_cmd <cmd> [<install-hint>] — error out if cmd is missing.
require_cmd() {
    local cmd="${1:?cmd required}"
    local hint="${2:-}"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        if [ -n "$hint" ]; then
            error "missing required command: $cmd ($hint)"
        else
            error "missing required command: $cmd"
        fi
    fi
}

# has_cmd <cmd> — returns 0 if present, 1 otherwise (no exit).
has_cmd() {
    command -v "${1:?cmd required}" >/dev/null 2>&1
}
