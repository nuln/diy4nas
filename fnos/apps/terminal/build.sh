#!/bin/bash
# terminal: 需要 CGO 编译（依赖 creack/pty 调用系统 PTY）
# 通过写入 $BUILD_DIR/.app-cgo 文件通知顶层 build.sh 开启 CGO
set -e
cat > "${BUILD_DIR}/.app-cgo" <<EOF
export APP_CGO=1
EOF
echo "  terminal: CGO enabled (creack/pty requires native C)"
