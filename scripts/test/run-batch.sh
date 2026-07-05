#!/bin/bash
# Batch driver: for each app, locally build .fpk, run L2 verify, run L3
# fpk-runner cycle. Logs each step per-app; prints a summary table.
#
# Usage:
#   scripts/test/run-batch.sh                          # all non-docker apps
#   scripts/test/run-batch.sh <slug> [<slug>...]       # specific apps
#
# Env:
#   BATCH_LOG_DIR    where to keep per-app logs (default: /tmp/fnos-batch-<pid>)
#   BATCH_KEEP_FPK   if set, don't delete dist/<slug>_*.fpk after pass
#   BATCH_ARCH       x86 (default) or arm
#
# Exits 0 if every app passed end-to-end, 1 if any fail.

set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

require_cmd docker
require_cmd jq

ARCH="${BATCH_ARCH:-x86}"
LOG_DIR="${BATCH_LOG_DIR:-/tmp/fnos-batch-$$}"
mkdir -p "$LOG_DIR"
info "Logs: $LOG_DIR"

REPO="$(repo_root)"
APPS=("$@")

if [ "${#APPS[@]}" -eq 0 ]; then
    if [ ! -f "$REPO/apps.json" ]; then
        error "no apps.json in repo root; pass app slugs explicitly"
    fi
    while IFS= read -r slug; do
        APPS+=("$slug")
    done < <(jq -r '.apps[] | select(.app_type != "docker") | .slug' "$REPO/apps.json")
    info "Defaulting to ${#APPS[@]} non-docker apps from apps.json"
fi

PASS=()
FAIL_BUILD=()
FAIL_L2=()
FAIL_L3=()
SKIP=()

T_START=$(date +%s)

run_one() {
    local slug="$1"
    local app_log="$LOG_DIR/$slug"
    mkdir -p "$app_log"

    note "═══════════ $slug (arch=$ARCH) ═══════════"

    local update_script="$REPO/apps/$slug/update_${slug}.sh"
    if [ ! -x "$update_script" ]; then
        warn "$slug: no $update_script — SKIP"
        SKIP+=("$slug:no-update-script")
        return
    fi

    if is_docker_app "$slug"; then
        warn "$slug: is a docker app, fpk-runner does not support Docker apps yet — SKIP"
        SKIP+=("$slug:docker")
        return
    fi

    info "$slug → building .fpk"
    if ! (cd "$REPO/apps/$slug" && bash "update_${slug}.sh" --arch "$ARCH") > "$app_log/build.log" 2>&1; then
        warn "$slug: BUILD failed (see $app_log/build.log)"
        tail -5 "$app_log/build.log" | sed 's/^/    /'
        FAIL_BUILD+=("$slug")
        return
    fi

    local fpk file_prefix
    file_prefix="$(grep -E '^FILE_PREFIX=' "$REPO/scripts/apps/$slug/meta.env" 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"' || true)"
    [ -z "$file_prefix" ] && file_prefix="$slug"
    fpk="$(ls "$REPO/dist/${file_prefix}"_*_"$ARCH".fpk 2>/dev/null | tail -1)"
    if [ -z "$fpk" ] || [ ! -f "$fpk" ]; then
        warn "$slug: BUILD finished but no .fpk artifact in dist/"
        FAIL_BUILD+=("$slug:no-artifact")
        return
    fi
    info "$slug → built $(basename "$fpk") ($(du -h "$fpk" | cut -f1))"

    info "$slug → L2 verify-fpk"
    if ! bash "$SCRIPT_DIR/verify-fpk.sh" "$fpk" "$slug" > "$app_log/l2.log" 2>&1; then
        warn "$slug: L2 verify-fpk FAILED"
        tail -10 "$app_log/l2.log" | sed 's/^/    /'
        FAIL_L2+=("$slug")
        return
    fi

    info "$slug → L3 fpk-runner cycle"
    if ! bash "$SCRIPT_DIR/run-fpk-tests.sh" "$fpk" "$slug" > "$app_log/l3.log" 2>&1; then
        warn "$slug: L3 cycle FAILED"
        tail -15 "$app_log/l3.log" | sed 's/^/    /'
        FAIL_L3+=("$slug")
        return
    fi

    pass "$slug: ALL GREEN"
    PASS+=("$slug")

    if [ -z "${BATCH_KEEP_FPK:-}" ]; then
        rm -f "$fpk"
    fi
}

for slug in "${APPS[@]}"; do
    run_one "$slug"
done

T_END=$(date +%s)
ELAPSED=$((T_END - T_START))

echo "" >&2
echo "════════════════════════════════════════════════════════════════" >&2
echo "  BATCH SUMMARY ($((ELAPSED / 60))m$((ELAPSED % 60))s)" >&2
echo "════════════════════════════════════════════════════════════════" >&2
printf '  %s%d PASS%s          %s\n' "$_C_GREEN" "${#PASS[@]}" "$_C_NC" "${PASS[*]}" >&2
printf '  %s%d FAIL build%s    %s\n' "$_C_RED"   "${#FAIL_BUILD[@]}" "$_C_NC" "${FAIL_BUILD[*]}" >&2
printf '  %s%d FAIL L2%s       %s\n' "$_C_RED"   "${#FAIL_L2[@]}" "$_C_NC" "${FAIL_L2[*]}" >&2
printf '  %s%d FAIL L3%s       %s\n' "$_C_RED"   "${#FAIL_L3[@]}" "$_C_NC" "${FAIL_L3[*]}" >&2
printf '  %s%d SKIP%s          %s\n' "$_C_YELLOW" "${#SKIP[@]}" "$_C_NC" "${SKIP[*]}" >&2
echo "" >&2
echo "  Per-app logs: $LOG_DIR" >&2
echo "════════════════════════════════════════════════════════════════" >&2

TOTAL_FAIL=$(( ${#FAIL_BUILD[@]} + ${#FAIL_L2[@]} + ${#FAIL_L3[@]} ))
[ "$TOTAL_FAIL" -eq 0 ]
