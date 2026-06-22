#!/bin/bash
BASE="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$BASE/dist"

ARCH="${ARCH:-amd64}"

# 支持指定单个 app: bash build.sh tailscale
if [ $# -ge 1 ]; then
    TARGETS=()
    for arg in "$@"; do
        [ -d "apps/$arg" ] && TARGETS+=("$arg") || echo "WARN: apps/$arg not found"
    done
    [ ${#TARGETS[@]} -eq 0 ] && echo "No valid apps specified" && exit 1
else
    TARGETS=()
    for d in apps/*/; do TARGETS+=("$(basename "$d")"); done
fi

for slug in "${TARGETS[@]}"; do
    slug=$(basename "$slug")
    base="apps/$slug"
    version=$(grep "^version" "$base/manifest" 2>/dev/null | cut -d= -f2- | xargs)
    version=${version:-0.0.0}
    echo "[BUILD] $slug v$version"
    
    BUILD=$(mktemp -d)
    mkdir -p "$BUILD/app"
    
    # 复制 Docker compose
    [ -d "$base/docker" ] && cp -r "$base/docker" "$BUILD/app/"
    
    # 调用 app 专属构建脚本（下载二进制、设置 APP_CGO 等）
    # 使用 source 以便 app/build.sh 中的 export 能在当前 shell 生效
    if [ -x "$base/build.sh" ]; then
        echo "  running app build script..."
        (export BUILD_DIR="$BUILD/app" ARCH="$ARCH" VERSION="$version"; "$base/build.sh")
        # 让 app 可以把 APP_CGO 写入临时文件传回（因为 subshell 的 export 不会传回父 shell）
        if [ -f "$BUILD/app/.app-cgo" ]; then
            source "$BUILD/app/.app-cgo"
        fi
    fi

    # 编译 Go 管理服务（如果存在 www/main.go）
    if [ -f "$base/www/main.go" ]; then
        CGO_FLAG=${APP_CGO:-0}
        # 从 manifest 读取 appname 作为基础名；加 fnos- 前缀避免与系统/其他 app 进程名冲突
        APP_BASE_NAME=$(grep "^appname[[:space:]]*=" "$base/manifest" 2>/dev/null | head -1 | sed -E 's/^[^=]+=[[:space:]]+//; s/[[:space:]]+$//')
        APP_BIN_NAME="${APP_BINARY_NAME:-fnos-${APP_BASE_NAME}}"
        echo "  compiling Go server -> ${APP_BIN_NAME} (CGO=$CGO_FLAG)..."
        (cd "$base/www" && CGO_ENABLED=$CGO_FLAG GOOS=linux GOARCH="$ARCH" \
            go build -o "$BUILD/app/${APP_BIN_NAME}" -ldflags="-s -w" . 2>/dev/null) || \
            echo "  ⚠️  Go build failed"
    fi

    # 复制启动器脚本
    [ -d "$base/bin" ] && cp -r "$base/bin" "$BUILD/app/"

    # 复制 UI
    if [ -d "$base/ui" ]; then
        mkdir -p "$BUILD/app/ui"
        cp -r "$base/ui/"* "$BUILD/app/ui/" 2>/dev/null || true
    fi
    
    (cd "$BUILD" && COPYFILE_DISABLE=1 tar --no-mac-metadata -czf "$BASE/app.tgz" app/ 2>/dev/null)
    
    echo "  packaging fpk..."
    FPK="$BUILD/fpk"
    mkdir -p "$FPK"
    for dir in cmd config wizard; do
        [ -d "$base/$dir" ] && cp -r "$base/$dir" "$FPK/"
    done
    cp "$base/manifest" "$FPK/"
    [ -f "$base/$slug.sc" ] && cp "$base/$slug.sc" "$FPK/"
    [ -f "$base/ICON.PNG" ] && cp "$base/ICON.PNG" "$FPK/"
    [ -f "$base/ICON_256.PNG" ] && cp "$base/ICON_256.PNG" "$FPK/"
    [ -f "$base/health.json" ] && cp "$base/health.json" "$FPK/"
    cp "$BASE/app.tgz" "$FPK/" 2>/dev/null || cp "$BUILD/app.tgz" "$FPK/"
    
    ck=$(md5 -q "$FPK/app.tgz" 2>/dev/null || md5sum "$FPK/app.tgz" | cut -d' ' -f1)
    echo "checksum = $ck" >> "$FPK/manifest"
    
    OUTFILE="$BASE/dist/${slug}-${version}.fpk"
    (cd "$FPK" && COPYFILE_DISABLE=1 tar --no-mac-metadata -czf "$OUTFILE" ./*)
    rm -rf "$BUILD"
    echo "  -> ${slug}-${version}.fpk"
done

echo "Done"
