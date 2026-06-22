# terminal 开发笔记

## 架构
- **后端**：Go + **CGO**（依赖 `github.com/creack/pty` 调用系统 PTY）
- **前端**：单 HTML + xterm.js（CDN 引入）+ go:embed
- **二进制名**：`fnos-terminal`（独特命名）
- **服务名**：`terminal`
- **数据目录**：`${TRIM_PKGVAR}/data/`
- **PID 文件**：`${TRIM_PKGVAR}/data/server.pid`
- **日志文件**：`${TRIM_PKGVAR}/data/terminal.log`
- **端口**：`7682`（HTTP，loopback）
- **协议**：WebSocket（前端 xterm.js ↔ Go server ↔ PTY shell）
- **Shell**：默认 `/bin/bash -l`（login shell）

## 关键文件
- `manifest` — `appname = terminal`
- `terminal.sc` — `port_forward="yes"`, ports="7682/tcp"
- `config/privilege` — `run-as: root`（PTY 创建需要 root 或 setcap）
- `cmd/main` — 启动/停止脚本
- `www/main.go` — Go 入口，监听 127.0.0.1:7682
- `www/session.go` — **关键**：PTY 创建、ring buffer、session 管理
- `www/api.go` — WebSocket 处理（`/api/ws`）
- `www/manager.go` — session 池
- `www/ringbuffer.go` — 滚动缓冲（断线重连可回放）
- `www/ui/index.html` — xterm.js 终端

## Build 命令

### amd64
```bash
cd apps/terminal/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-terminal -ldflags="-s -w" .
```

### arm64
```bash
cd apps/terminal/www
GOOS=linux GOARCH=arm64 go build -o /tmp/fnos-terminal -ldflags="-s -w" .
```

### ⚠️ 不需要 CGO 跨平台
**关键**：不需要 `CGO_ENABLED=1`，本项目用 `github.com/creack/pty` 必须在 linux/amd64 或 linux/arm64 上编译（**必须**用 docker 编译，因为 creack/pty 是 CGO，macOS 交叉编译 Linux 时缺 Linux C 库头文件）。

```bash
# 正确：macOS 上用 docker 编译 amd64
docker run --rm --platform linux/amd64 \
  -v /path/to/apps/terminal:/src golang:1.22-bookworm sh -c '
    apt update && apt install -y gcc libc6-dev
    cd /src/www
    go mod tidy
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /src/fnos-terminal .
  '
```

**最容易踩的坑**：在 macOS 上 `CGO_ENABLED=1 GOOS=linux` 会报 cgo 找不到头文件。**必须用 docker**。

## 常见坑

### 1. CGO 跨平台编译
- macOS 上不能 `CGO_ENABLED=1 GOOS=linux` 交叉编译（缺 Linux 头文件）
- 必须用 linux docker 镜像 + apt 装 gcc/libc6-dev + `CGO_ENABLED=1`
- 不要尝试在 macOS 本地交叉编译 fnos-terminal

### 2. cmd/main 必须 export TRIM_APPDEST
同 scheduler，详见 scheduler/DEV.md 第 2 节。

### 3. PTY session 资源管理
- PTY 进程死了后要清理 subscriber channel（close 防止 goroutine 泄漏）
- `appVar` 用于 shell 的 `HOME` 和 `cmd.Dir`（默认工作目录）

### 4. WebSocket 路径前缀
前端 xterm.js 在 `/app/terminal/` 路径下访问 WebSocket。**WebSocket URL 必须带 `/app/terminal/` 前缀**。前端通过 `location.pathname` 推算（`API_BASE` 变量），不能写死。

### 5. 浏览器必须支持 xterm.js
xterm.js 从 CDN 加载（jsdelivr）。如果 fnOS 桌面无法访问 CDN，终端不能用。**未来需要把 xterm.js 打包到 ui/ 里**。

## 已知问题 / 待办

- [ ] xterm.js 打包到本地（不依赖 CDN）
- [ ] session 持久化（重启服务会丢失所有 session）
- [ ] 多用户隔离 / 鉴权（任何能访问 socket 的人都能开 root shell）
- [ ] 操作日志（用户在终端执行了什么命令）
- [ ] 真实图标

### 5. manifest 的 platform 字段必须是 `x86_64`
fnOS 新版本校验严格：
- ❌ `platform = x86`（旧格式）→ 报"应用包不符合系统要求"
- ❌ `platform = all`（虽然不报错但会显示不准确）
- ✅ `platform = x86_64`（正确格式，匹配 amd64 NAS）

其他合法值（按需）：
- `platform = arm64`（ARM NAS）
- `platform = all`（通用，但可能在某些 fnOS 版本被警告）
