# mihomo 开发笔记

## 架构
- **Go server**（`fnos-mihomo`）：HTTP API 包装层，管理 mihomo 进程 + 提供 Web UI
- **实际代理**：`mihomo` 二进制（MetaCubeX 的 Clash.Meta 分支），由 Go server 启动
- **二进制名**：`fnos-mihomo`（独特命名）
- **服务名**：`mihomo`
- **数据目录**：`${TRIM_PKGVAR}/data/`
- **PID 文件**：`${TRIM_PKGVAR}/data/server.pid`
- **日志文件**：`${TRIM_PKGVAR}/data/mihomo.log`
- **Socket**：`${TRIM_PKGVAR}/data/mihomo.sock`
- **端口**：`9097`（HTTP API）、`7890`/`7891`（代理端口，外部）
- **进程关系**：fnos-mihomo (父) → mihomo (子)
- **数据库**：`geoip.metadb` + `geosite.dat`（路由规则）

## 关键文件
- `manifest` — `appname = mihomo`
- `mihomo.sc` — `port_forward="yes"`, ports="9097/tcp,7890/tcp"
- `config/privilege` — `run-as: root`
- `cmd/install_callback` — setcap mihomo（需要 NET_ADMIN、NET_RAW、NET_BIND_SERVICE）
- `cmd/service-setup` — setcap post-install
- `cmd/uninstall_callback` — 清理 setcap、清理系统代理文件 /etc/profile.d/mihomo-proxy.sh
- `cmd/main` — 启动/停止脚本（必须 export MIHOMO_SERVICE_PORT）
- `www/main.go` — Go 入口
  - `var mihomoBin = findBin("mihomo", "")` — 找 mihomo 二进制
  - `var appDest = getEnv("TRIM_APPDEST", "/var/apps/mihomo")` — ⚠️ 内部默认值不可信，必须用 cmd/main export 覆盖
- `www/ui/` — 前端

## Build 命令

### amd64
```bash
# 1. 下载 mihomo amd64 二进制（注意 URL 格式）
curl --proxy http://127.0.0.1:7890 -fsSL \
  "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.27/mihomo-linux-amd64-compatible-v1.19.27.gz" \
  -o /tmp/mihomo.gz
gunzip /tmp/mihomo.gz && chmod +x /tmp/mihomo

# 2. 下载数据库
curl --proxy http://127.0.0.1:7890 -fsSL \
  "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb" -o /tmp/geoip.metadb
curl --proxy http://127.0.0.1:7890 -fsSL \
  "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat" -o /tmp/geosite.dat

# 3. 用 macOS 本地 Go 编译 Go server（不需要 docker）
cd apps/mihomo/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-mihomo -ldflags="-s -w" .

# 4. 组合 FPK
mkdir -p /tmp/mihomo-pack/app
cp /tmp/mihomo /tmp/fnos-mihomo /tmp/geoip.metadb /tmp/geosite.dat /tmp/mihomo-pack/app/
cp -r /Users/dukangxu/nuln/diy4nas/fnos/apps/mihomo/ui /tmp/mihomo-pack/app/
cd /tmp/mihomo-pack && tar czf app.tgz app
# 然后按标准 FPK 流程打包
```

### arm64
URL 改：
- `mihomo-linux-arm64-v1.19.27.gz`（**没有 -compatible 后缀**）
- arm64 永远没 -compatible

```bash
curl ... "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.27/mihomo-linux-arm64-v1.19.27.gz" ...
```

## install_callback 的 setcap（关键！）

```sh
setcap 'cap_net_admin,cap_net_raw,cap_net_bind_service+eip' "$base/mihomo"
```

**cap_net_bind_service** 是必须的（mihomo 要 bind 7890/7891 等端口）。
**cap_net_admin + cap_net_raw** 是 TUN 模式需要（如果用 TUN 模式）。

如果 mihomo 只用 system proxy 而不开 TUN，理论上不需要 NET_ADMIN/NET_RAW。
但保险起见都加上。

## 常见坑

### 1. mihomo 二进制 URL 命名规则混乱
- **amd64**: `mihomo-linux-amd64-compatible-v1.19.27.gz`（带 -compatible）
- **arm64**: `mihomo-linux-arm64-v1.19.27.gz`（**不带 -compatible**）
- 这是 MetaCubeX 的历史遗留，1.19.27 是最后一个有这种分离的版本
- 新版本（v2+）统一命名，build.sh 里 API 动态解析更稳

### 2. TRIM_APPDEST 必须传给 Go server
同 tailscale，cmd/main 必须 export：
```sh
export TRIM_APPDEST="$APP_DEST"
export TRIM_PKGVAR="$APP_VAR"
export MIHOMO_SERVICE_PORT="9097"
```

### 3. unzip 要兼容 .gz 和 .zip
build.sh 要用 `file` 命令判断 .gz/.zip 格式

### 4. download 网络问题
- `release-assets.githubusercontent.com` 在国内网络**不通**
- 必须用 `--proxy http://127.0.0.1:7890` 走本地代理
- 或用 ghproxy.net 镜像（慢但稳定）

## docker build 的坑（**重要**）

`bash build.sh mihomo` 在 docker 里**会卡**：
- docker 容器内**没有**用户的代理配置
- `http_proxy` env 必须显式传：`docker run -e http_proxy=... -e https_proxy=...`
- 或者 macOS 外部预下载后挂载：`docker run -v /tmp:/host:ro` 然后 build.sh 里改路径

**最稳妥方案**：不用 docker build mihomo，直接 macOS 上：
1. `curl --proxy ...` 下载
2. `GOOS=linux/amd64 go build` 编译 Go server
3. 手动组装 FPK

## 已知问题 / 待办

- [ ] mihomo 版本写死 1.19.27（最新是 1.19.x 系列，更新后命名变了）
- [ ] 升级 mihomo 时不中断当前代理连接
- [ ] 显示当前代理状态/连接数
- [ ] UI 更友好（v2.x 的 mihomo dashboard 风格）

### 5. manifest 的 platform 字段必须是 `x86_64`
fnOS 新版本校验严格：
- ❌ `platform = x86`（旧格式）→ 报"应用包不符合系统要求"
- ❌ `platform = all`（虽然不报错但会显示不准确）
- ✅ `platform = x86_64`（正确格式，匹配 amd64 NAS）

其他合法值（按需）：
- `platform = arm64`（ARM NAS）
- `platform = all`（通用，但可能在某些 fnOS 版本被警告）
