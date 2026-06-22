# tailscale 开发笔记

## 架构
- **Go server**（`fnos-tailscale`）：HTTP API 包装层，提供 Web UI 和 API
- **实际 VPN 客户端**：`tailscaled`（官方二进制）和 `tailscale`（CLI），由 Go server 启动和管理
- **二进制名**：`fnos-tailscale`（独特命名）
- **服务名**：`tailscale`
- **数据目录**：`${TRIM_PKGVAR}/data/`
- **PID 文件**：`${TRIM_PKGVAR}/data/server.pid`
- **日志文件**：`${TRIM_PKGVAR}/data/tailscaled.log`（注意：是 tailscaled.log 不是 server.log）
- **Socket**：`${TRIM_PKGVAR}/data/tailscaled.sock`（tailscaled 的 unix socket）
- **端口**：`8088`（HTTP API）
- **TUN 端口**：`41641`（tailscaled 监听）
- **进程关系**：fnos-tailscale (父) → tailscaled (子)

## 关键文件
- `manifest` — `appname = tailscale`
- `tailscale.sc` — `port_forward="yes"`, ports="8088/tcp"
- `config/privilege` — `run-as: root`
- `cmd/install_callback` — **关键**：setcap tailscaled（创建 TUN 设备需要）
- `cmd/service-setup` — setcap post-install 钩子
- `cmd/main` — 启动/停止脚本（必须 export TAILSCALE_BIN、TAILSCALED_BIN、TAILSCALE_SOCKET）
- `www/main.go` — Go 入口
  - `var tsdBin = findBin("tailscaled", ...)` — 找 tailscaled
  - `startTailscaled()` — exec tailscaled，等待 socket
  - `main()` 调 `startTailscaled()`，失败 `log.Fatalf`
- `www/ui/` — 前端（包含 vis-network.min.js for 网络拓扑）

## Build 命令

### amd64
```bash
# 1. 下载 tailscale amd64 二进制
curl --proxy http://127.0.0.1:7890 -fsSL \
  "https://pkgs.tailscale.com/stable/tailscale_1.98.4_amd64.tgz" -o /tmp/ts.tgz
tar xzf /tmp/ts.tgz -C /tmp/ --strip-components=1

# 2. 用 docker build Go server
docker run --rm --platform linux/amd64 \
  -v /path/to/apps/tailscale:/src golang:1.22-bookworm sh -c '
    apt update && apt install -y gcc libc6-dev
    cd /src/www
    go mod tidy
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /src/fnos-tailscale .
  '

# 3. 组合
mkdir -p /tmp/ts-pack/app
cp /tmp/tailscale /tmp/tailscaled /tmp/fnos-tailscale /tmp/ts-pack/app/
```

### arm64
类似，`_amd64` 改 `_arm64`，`linux/amd64` 改 `linux/arm64`。

## ⚠️ docker 编译注意
- 容器内网络需要代理才能下 tailscale 官方包（macOS 拉 release-assets.githubusercontent.com 不通）
- 或者**外部预下载** + mount 进容器
- 或者直接用 macOS 本地 Go 交叉编译 Go server（不需 docker），二进制下到 /tmp 后再组装

## install_callback 的 setcap（关键！）

```sh
# 必须在 cmd/install_callback 和 cmd/service-setup 的 service_postinst 都执行
setcap 'cap_net_admin,cap_net_raw+eip' "$base/tailscaled"
setcap 'cap_net_admin+eip' "$base/tailscale"
```

**没有 setcap，tailscaled 启动后无法创建 /dev/net/tun，10s 后 Go server 报"socket not ready"。**

如果 fnOS 限制 setcap：
1. SSH 到 NAS 手动跑：`sudo setcap 'cap_net_admin,cap_net_raw+eip' /vol1/@appcenter/tailscale/app/tailscaled`
2. 或检查 `/dev/net/tun` 是否存在：`ls -la /dev/net/tun`
3. 或加载 tun 模块：`sudo modprobe tun`

## 常见坑

### 1. tailscaled 进程名必须是独特的
- ❌ 用 `server` → `pkill -x server` 会杀掉 tailscale/mihomo/scheduler/terminal
- ✅ 用 `fnos-tailscale`

### 2. TRIM_APPDEST 必须传给 Go server
Go server 内部 `findBin("tailscaled", ...)` 读 `os.Getenv("TRIM_APPDEST")`。fnOS 实际路径是 `/vol1/...`（不是 `/var/apps/`）。

**cmd/main start() 必须**：
```sh
export TRIM_APPDEST="$APP_DEST"
export TRIM_PKGVAR="$APP_VAR"
export TAILSCALE_BIN="$APP_DEST/app/tailscale"
export TAILSCALED_BIN="$APP_DEST/app/tailscaled"
export TAILSCALE_SOCKET="$SOCKET"
export TAILSCALE_PORT="8088"
```

### 3. 启动检查必须 ≥ 1s
`startTailscaled()` 内部要 modprobe tun + 等待 socket，最多 10s。
cmd/main 的 `sleep 0.2` 太短，必须 `sleep 1.5`（或更长），否则会误报 failed to start。

### 4. TUN 设备
- tailscaled 创建 TUN 设备需要 CAP_NET_ADMIN
- 真实错误信息在 `${TRIM_PKGVAR}/data/tailscaled.log`（不是 server.log）
- 看这个日志就知道是 setcap 问题还是 TUN 模块问题

## 已知问题 / 待办

- [ ] 启动失败的错误信息更友好（现在只说 "fnos-tailscale failed to start"）
- [ ] 重新安装时清理旧的 tailscaled.sock
- [ ] 跨架构二进制（arm64 NAS 支持）

### 5. manifest 的 platform 字段必须是 `x86_64`
fnOS 新版本校验严格：
- ❌ `platform = x86`（旧格式）→ 报"应用包不符合系统要求"
- ❌ `platform = all`（虽然不报错但会显示不准确）
- ✅ `platform = x86_64`（正确格式，匹配 amd64 NAS）

其他合法值（按需）：
- `platform = arm64`（ARM NAS）
- `platform = all`（通用，但可能在某些 fnOS 版本被警告）
