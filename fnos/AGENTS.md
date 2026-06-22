# diy4nas/fnos — AI Agent Notes

## 项目说明
fnOS（飞牛 NAS）应用的 DIY 仓库。每个 `apps/<slug>/` 是一个独立 app，打包成 `.fpk` 在 fnOS 桌面上安装运行。

**重要参考**：
- `apps/tailscale/` 和 `apps/mihomo/` 是 **真实可用** 的参考实现，结构完整
- `apps/easytier/` 是另一个参考
- `apps/scheduler/` `apps/terminal/` 是本项目新开发的 2 个 app

## 用户环境
- **用户 NAS**：x86_64 架构，fnOS 系统
- **用户本地开发机**：macOS（Apple Silicon，arm64）
- **用户本地代理端口**：`127.0.0.1:7890`（用于下载 GitHub release 资源）
- **网络限制**：
  - `github.com` 主域可通
  - `release-assets.githubusercontent.com` 直连**不通**（被网络层封）
  - 必须用 `curl --proxy http://127.0.0.1:7890` 或 `ghproxy.net` 镜像

## 4 个新 app 的开发注意事项

每个 app 目录下都有 `DEV.md` 详细记录。**修改/编译前必读！**

| App | DEV.md | 关键特点 |
|---|---|---|
| `apps/scheduler/` | ✓ | 纯 Go，无 CGO，可直接 macOS 交叉编译 |
| `apps/terminal/` | ✓ | **CGO 依赖**（`creack/pty`），**必须用 docker 编译** |
| `apps/tailscale/` | ✓ | 需要外部二进制（tailscale + tailscaled）+ setcap |
| `apps/mihomo/` | ✓ | 需要外部二进制（mihomo）+ setcap + 数据库 |

## 通用 build 流程

### 编译方法

**纯 Go app（scheduler）：**
```bash
cd apps/<slug>/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-<slug> -ldflags="-s -w" .
# 不需要 docker
```

**CGO app（terminal）：**
```bash
# 必须用 docker，因为 creack/pty 是 CGO
docker run --rm --platform linux/amd64 \
  -v /path/to/apps/terminal:/src golang:1.22-bookworm sh -c '
    apt update && apt install -y gcc libc6-dev
    cd /src/www
    go mod tidy
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /src/fnos-terminal .
  '
```

**需要外部二进制的 app（tailscale/mihomo）：**
```bash
# 1. macOS 上下载（用代理）
curl --proxy http://127.0.0.1:7890 -fsSL <URL> -o /tmp/binary
chmod +x /tmp/binary

# 2. macOS 上编译 Go server（不需要 docker）
cd apps/<slug>/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-<slug> -ldflags="-s -w" .

# 3. 手动组装 FPK（build.sh 顶层也行，但 docker build 容易卡）
mkdir -p /tmp/pack/app
cp /tmp/binary /tmp/fnos-<slug> /tmp/pack/app/
# 打包 app.tgz 然后装到 FPK
```

### FPK 结构（必须包含）

```
dist/<slug>-0.0.0.fpk
├── manifest             # appname / display_name / desktop_uidir / service_port
├── <slug>.sc            # port_forward="yes" + ports
├── cmd/
│   ├── main             # 启动/停止/状态（必须 export TRIM_APPDEST）
│   ├── install_callback # setcap 等初始化
│   ├── service-setup    # service_postinst + service_preuninst 函数
│   ├── upgrade_callback # bak 还原 + 自动 start
│   └── uninstall_callback # 读 wizard_delete_data 清理
├── config/
│   ├── privilege        # run-as: root
│   └── resource         # port-config, data-share
├── wizard/
│   ├── install          # 安装步骤
│   └── uninstall        # 含 delete_data switch
├── ui/                  # fnOS 桌面入口（必须有）
│   ├── config           # JSON 告诉 fnOS 怎么打开
│   ├── api.cgi          # CGI 代理
│   ├── index.cgi        # 加载页
│   └── images/icon_64.png + icon_256.png
└── app.tgz              # 实际二进制（包含 app/<slug-server>）
```

## 踩过的坑（重要！）

### 1. 二进制名必须独特
- ❌ 用 `server` 作 Go server 名 → `pkill -x server` 误杀所有 app
- ✅ 用 `fnos-<slug>`（build.sh 顶层自动从 `appname` 加 `fnos-` 前缀）

### 2. cmd/main 必须 export 环境变量
fnOS 实际安装路径是 `/vol1/@appcenter/<slug>/`，不是 `/var/apps/`。Go server 内部 `findBin` 读 `os.Getenv("TRIM_APPDEST")`，**拿不到默认 fallback 到 `/var/apps/` 找不到二进制**。

**每个 cmd/main start() 必须**：
```sh
export TRIM_APPDEST="$APP_DEST"
export TRIM_PKGVAR="$APP_VAR"
# app 专属：
export TRIM_LISTEN="127.0.0.1:7681"          # scheduler/terminal
export TAILSCALE_BIN="$APP_DEST/app/tailscale"
export TAILSCALED_BIN="$APP_DEST/app/tailscaled"
export TAILSCALE_SOCKET="$SOCKET"
export MIHOMO_SERVICE_PORT="9097"
```

### 3. 杀进程用 PID 优先 + 独特名兜底
```sh
verify_pid() {
    # /proc/$pid/comm 必须 == $NAME
    # /proc/$pid/exe 必须指向 $APP_DEST/app/$NAME
}
kill_by_pid() {  # 主杀进程
    # read PID file → verify_pid → kill
    # PID 无效时返回 1
}
stop() {
    if kill_by_pid; then
        exit 0   # 成功处理
    fi
    pkill -x "$NAME"  # 兜底（独特名）
}
```

### 4. 启动检查 ≥ 1.5s
不要用 `sleep 0.2`，Go server 启动可能慢（特别是 tailscale 要 modprobe tun）。用 `sleep 1.5`。

### 5. setcap 是关键
tailscale 和 mihomo 需要 setcap 给二进制：
- tailscale: `cap_net_admin,cap_net_raw` 给 tailscaled
- mihomo: `cap_net_admin,cap_net_raw,cap_net_bind_service` 给 mihomo

放在 `cmd/install_callback` 和 `cmd/service-setup` 的 `service_postinst` 里都做一次。

### 6. 架构必须匹配
- 在 Apple Silicon Mac 上 build 默认是 arm64
- 用户 NAS 是 x86_64 → 必须 `GOARCH=amd64` 或 `docker --platform linux/amd64`
- **Exec format error** = 架构不匹配

### 7. build.sh 顶层的二进制名机制
build.sh 从 manifest 的 `appname` 读二进制名，加 `fnos-` 前缀：
- `appname = scheduler` → `app/fnos-scheduler`
- `appname = mihomo` → `app/fnos-mihomo`

cmd/main 用同样的 `find_bin "$NAME"` 找这个二进制（NAME="fnos-<appname>"）。

## 4 个 FPK 当前状态

```
dist/scheduler-0.0.0.fpk   4.1M  amd64
dist/terminal-0.0.0.fpk    2.6M  amd64  (CGO 编译需 docker)
dist/tailscale-0.0.0.fpk   38M   amd64  (含 tailscale + tailscaled)
dist/mihomo-0.0.0.fpk      25M   amd64  (含 mihomo + 数据库)
```

## 测试

### 单元 smoke test
```bash
mkdir -p /tmp/scheduler-test/data
TRIM_APPDEST=/tmp/scheduler-test \
TRIM_PKGVAR=/tmp/scheduler-test/data \
TRIM_TEMP_LOGFILE=/tmp/test.log \
/tmp/fnos-scheduler &
sleep 1
curl http://127.0.0.1:7681/api/healthz
```

### Docker 模拟 fnOS 实际路径
```bash
docker run --rm --platform linux/amd64 \
  --cap-add=NET_ADMIN --device=/dev/net/tun \
  -v /path/to/dist:/work debian:bookworm sh -c '
    apt update && apt install -y curl procps libcap2-bin
    mkdir -p /vol1/@appcenter/tailscale /vol1/@appdata/tailscale
    tar xzf /work/tailscale-0.0.0.fpk
    ...
  '
```

### 完整 E2E（用 .mock 脚本）
```bash
# .mock/ 目录有完整 fnOS 部署模拟
# install / start / stop / upgrade / uninstall 都有脚本
```

## 重要提示给 AI

1. **不要自动 build**：build.sh 在 docker 容器内容易卡（网络、代理问题）。**手动分步 build**：
   - 下载用 `curl --proxy http://127.0.0.1:7890`
   - Go 编译用 macOS 本地交叉编译
   - 手动组装 FPK

2. **不要 hardcode `/var/apps/`**：fnOS 实际是 `/vol1/@appcenter/`

3. **不要破坏现有 FPK**：用户装好后只重装需要修改的 app，**先告诉用户会重 build 哪些**

4. **架构必须匹配**：x86_64（amd64）for user's NAS

5. **修复后必须实际 build + verify**：不能只改代码就声称"已修"
