#!/bin/bash
# Drive a full install→start→probe→stop→uninstall→assert-clean cycle on an .fpk
# inside the fpk-runner Docker container.
#
# Optionally also exercise the upgrade path: install OLD-fpk, start, stop,
# upgrade to NEW-fpk, start (post-upgrade), probe, then unwind.
#
# Usage:
#   scripts/test/run-fpk-tests.sh <fpk-path> [<slug>]
#   scripts/test/run-fpk-tests.sh --upgrade-from <old-fpk> <new-fpk> [<slug>]
#
# Slug defaults to inferred from the (new) fpk filename.
#
# Requires: docker
#
# Exits 0 on all-pass, 1 on any failure.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "$SCRIPT_DIR/lib.sh"

require_cmd docker "install Docker (CI runners have it; local: brew install docker)"

usage() {
    cat >&2 <<'EOF'
Usage:
  scripts/test/run-fpk-tests.sh <fpk-path> [<slug>]
  scripts/test/run-fpk-tests.sh --upgrade-from <old-fpk> <new-fpk> [<slug>]
EOF
    exit 1
}

OLD_FPK=""
FPK_PATH=""
SLUG=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --upgrade-from)
            [ -n "${2:-}" ] || usage
            OLD_FPK="$2"
            shift 2
            ;;
        --upgrade-from=*)
            OLD_FPK="${1#--upgrade-from=}"
            shift
            ;;
        -h|--help)
            usage
            ;;
        --)
            shift
            break
            ;;
        -*)
            error "unknown flag: $1"
            ;;
        *)
            if [ -z "$FPK_PATH" ]; then
                FPK_PATH="$1"
            elif [ -z "$SLUG" ]; then
                SLUG="$1"
            else
                error "unexpected positional argument: $1"
            fi
            shift
            ;;
    esac
done

[ -n "$FPK_PATH" ] || usage
[ -f "$FPK_PATH" ] || error "fpk file not found: $FPK_PATH"
if [ -n "$OLD_FPK" ]; then
    [ -f "$OLD_FPK" ] || error "old fpk file not found: $OLD_FPK"
fi

FPK_NAME="$(basename "$FPK_PATH")"
if [ -z "$SLUG" ] && [[ "$FPK_NAME" =~ ^(.+)_[^_]+_(x86|arm)\.fpk$ ]]; then
    SLUG="${BASH_REMATCH[1]}"
fi
[ -n "$SLUG" ] || error "cannot infer slug from filename; pass it explicitly"

REPO="$(repo_root)"
HEALTH_FILE="$REPO/apps/$SLUG/fnos/health.json"
HAS_HEALTH=0
if [ -f "$HEALTH_FILE" ]; then
    HAS_HEALTH=1
    info "Using health.json: apps/$SLUG/fnos/health.json"
else
    warn "no apps/$SLUG/fnos/health.json — defaults will apply"
fi

IMAGE_TAG="${FPK_RUNNER_IMAGE:-fnos-fpk-runner:latest}"

# Build the image if it doesn't exist yet.
if ! docker image inspect "$IMAGE_TAG" >/dev/null 2>&1; then
    info "Building fpk-runner image '$IMAGE_TAG'"
    docker build -t "$IMAGE_TAG" "$SCRIPT_DIR/fpk-runner" >&2
fi

abs_path() {
    local p="$1"
    echo "$(cd "$(dirname "$p")" && pwd)/$(basename "$p")"
}

FPK_ABS="$(abs_path "$FPK_PATH")"
MOUNT_ARGS=(-v "$FPK_ABS:/fpk/app.fpk:ro")
RUNNER_ENV=()

if [ -n "$OLD_FPK" ]; then
    OLD_FPK_ABS="$(abs_path "$OLD_FPK")"
    MOUNT_ARGS+=(-v "$OLD_FPK_ABS:/fpk/old.fpk:ro")
fi
if [ "$HAS_HEALTH" -eq 1 ]; then
    HEALTH_ABS="$(abs_path "$HEALTH_FILE")"
    MOUNT_ARGS+=(-v "$HEALTH_ABS:/health.json:ro")
    RUNNER_ENV+=(-e "HEALTH_JSON_SOURCE=/health.json")
fi

if [ -n "$OLD_FPK" ]; then
    info "── Running fpk-runner UPGRADE cycle for $SLUG ($(basename "$OLD_FPK") → $FPK_NAME) ──"
    DOCKER_SCRIPT='
        set -e
        rc_install=0; rc_start1=0; rc_probe1=0; rc_stop1=0
        rc_upgrade=0; rc_start2=0; rc_probe2=0; rc_stop2=0
        rc_uninstall=0; rc_clean=0
        /usr/local/bin/fpk-runner install /fpk/old.fpk || rc_install=$?
        if [ "$rc_install" -eq 0 ]; then
            /usr/local/bin/fpk-runner start    || rc_start1=$?
            /usr/local/bin/fpk-runner probe    || rc_probe1=$?
            /usr/local/bin/fpk-runner logs     || true
            /usr/local/bin/fpk-runner stop     || rc_stop1=$?
            /usr/local/bin/fpk-runner upgrade /fpk/app.fpk || rc_upgrade=$?
            if [ "$rc_upgrade" -eq 0 ]; then
                /usr/local/bin/fpk-runner start    || rc_start2=$?
                /usr/local/bin/fpk-runner probe    || rc_probe2=$?
                /usr/local/bin/fpk-runner logs     || true
                /usr/local/bin/fpk-runner stop     || rc_stop2=$?
            fi
            /usr/local/bin/fpk-runner uninstall || rc_uninstall=$?
            /usr/local/bin/fpk-runner assert-clean || rc_clean=$?
        fi
        echo ""
        echo "FPK_RUNNER_RESULT install=$rc_install start1=$rc_start1 probe1=$rc_probe1 stop1=$rc_stop1 upgrade=$rc_upgrade start2=$rc_start2 probe2=$rc_probe2 stop2=$rc_stop2 uninstall=$rc_uninstall clean=$rc_clean"
        exit_code=$(( rc_install + rc_start1 + rc_probe1 + rc_stop1 + rc_upgrade + rc_start2 + rc_probe2 + rc_stop2 + rc_uninstall + rc_clean ))
        exit $(( exit_code > 0 ? 1 : 0 ))
    '
    LABEL="upgrade-cycle"
else
    info "── Running fpk-runner cycle for $SLUG ($FPK_NAME) ──"
    DOCKER_SCRIPT='
        set -e
        rc_install=0; rc_start=0; rc_probe=0; rc_stop=0; rc_uninstall=0; rc_clean=0
        /usr/local/bin/fpk-runner install /fpk/app.fpk || rc_install=$?
        if [ "$rc_install" -eq 0 ]; then
            /usr/local/bin/fpk-runner start    || rc_start=$?
            /usr/local/bin/fpk-runner probe    || rc_probe=$?
            /usr/local/bin/fpk-runner logs     || true
            /usr/local/bin/fpk-runner stop     || rc_stop=$?
            /usr/local/bin/fpk-runner uninstall || rc_uninstall=$?
            /usr/local/bin/fpk-runner assert-clean || rc_clean=$?
        fi
        echo ""
        echo "FPK_RUNNER_RESULT install=$rc_install start=$rc_start probe=$rc_probe stop=$rc_stop uninstall=$rc_uninstall clean=$rc_clean"
        exit_code=$(( rc_install + rc_start + rc_probe + rc_stop + rc_uninstall + rc_clean ))
        exit $(( exit_code > 0 ? 1 : 0 ))
    '
    LABEL="install-cycle"
fi

set +e
docker run --rm \
    "${MOUNT_ARGS[@]}" \
    ${RUNNER_ENV[@]+"${RUNNER_ENV[@]}"} \
    --entrypoint bash \
    "$IMAGE_TAG" \
    -c "$DOCKER_SCRIPT"
RC=$?
set -e

if [ "$RC" -eq 0 ]; then
    pass "fpk-runner $LABEL PASS for $SLUG"
else
    fail "fpk-runner $LABEL FAIL for $SLUG (exit $RC)"
fi

report_summary "run-fpk-tests:$SLUG"
