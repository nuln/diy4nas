# scheduler 开发笔记

## 架构
- **后端**：Go 1.22+ 纯 Go（无 CGO），用 `robfig/cron/v3` + `modernc.org/sqlite`
- **前端**：单 HTML，go:embed 嵌入到 Go 二进制
- **二进制名**：`fnos-scheduler`（独特命名，防止 pkill 误杀）
- **服务名**：`scheduler`（用户可见）
- **数据目录**：`${TRIM_PKGVAR}/data/`（fnOS 实际为 `/vol1/@appdata/scheduler/data/`）
- **PID 文件**：`${TRIM_PKGVAR}/data/server.pid`
- **日志文件**：`${TRIM_PKGVAR}/data/scheduler.log`
- **端口**：`7681`（HTTP，127.0.0.1 loopback）

## 关键文件
- `manifest` — `appname = scheduler`, `platform = x86_64`
- `scheduler.sc` — `port_forward="yes"`, `src.ports="7681/tcp"`, `dst.ports="7681/tcp"`
- `config/privilege` — `run-as: root`（cron 子进程需要执行任意命令）
- `config/resource` — `port-config` 引用 `scheduler.sc` + `data-share`
- `wizard/install` — 安装提示
- `wizard/uninstall` — `delete_data: true`（默认删除，与 mihomo 一致）
- `cmd/main` — 启动/停止脚本，**必须 `export TRIM_APPDEST/PKGVAR/SERVICE_PORT` 给 Go server**
- `cmd/install_callback` — 准备数据目录
- `cmd/service-setup` — `service_postinst`（启动）+ `service_preuninst`（杀进程）
- `cmd/upgrade_init` — 备份 db/settings 为 `.bak`
- `cmd/upgrade_callback` — 恢复 `.bak` + 启动
- `cmd/uninstall_callback` — 兼容 `wizard_delete_data` / `delete_data`（默认 true）
- `health.json` — fnOS 健康检查（端口 + /api/healthz）
- `www/main.go` — Go 入口，监听 127.0.0.1:7681
- `www/db.go` — SQLite + 自动迁移 (last_status / last_run_at 列)
- `www/scheduler.go` — cron 集成
- `www/executor.go` — 进程执行 + 日志收集 + last_status 维护
- `www/api.go` — REST API + SSE（/api/log, /api/cleanup, /api/settings 立即生效）
- `www/ui/index.html` — 前端 SPA

## Build 命令

### amd64（NAS 是 x86_64）
```bash
cd apps/scheduler/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-scheduler -ldflags="-s -w" .
```

### arm64（NAS 是 ARM）
```bash
cd apps/scheduler/www
GOOS=linux GOARCH=arm64 go build -o /tmp/fnos-scheduler -ldflags="-s -w" .
```

**注意**：本项目可以直接 macOS 交叉编译到 linux/amd64 或 linux/arm64，不需要 docker。

### 完整 FPK 打包
用 `bash build.sh scheduler`（顶层 build.sh 已经处理二进制名 `fnos-scheduler`）

## 常见坑

### 1. 进程名必须是独特的
- ❌ 用 `server` 作二进制名 → `pkill -x server` 会杀掉所有 app
- ✅ 用 `fnos-scheduler` 作二进制名（`build.sh` 顶层自动从 manifest 的 `appname` 加 `fnos-` 前缀）

### 2. cmd/main 必须 export 环境变量
fnOS 实际安装路径是 `/vol1/...`，不是 `/var/apps/`。Go server 的 `os.Getenv("TRIM_APPDEST")` 拿不到就会用默认 `/var/apps/scheduler` 找不到二进制。

```sh
# cmd/main start() 必须有：
export TRIM_APPDEST="$APP_DEST"
export TRIM_PKGVAR="$APP_VAR"
export TRIM_SERVICE_PORT="7681"
```

### 3. 杀进程用 PID 优先 + pkill 兜底
不能只用 `pkill -x fnos-scheduler`，要：
1. 读 `server.pid` → 验证 `/proc/$pid/comm` 和 `/proc/$pid/exe` 是本 app
2. 验证通过才 kill
3. PID 文件无效/丢失时才 `pkill -x` 兜底

详见 `cmd/main` 的 `verify_pid` 和 `kill_by_pid` 函数。

### 4. /var/apps vs /vol1 路径问题
开发时测试用 `/var/apps/`，fnOS 实际用 `/vol1/@appcenter/<slug>/`。
**永远不要 hardcode `/var/apps/`**。

### 5. manifest 的 platform 字段必须是 `x86`
fnOS 桌面安装应用时校验 `platform` 字段，**实际接受的值是 `x86`（短名）**，不是 `x86_64`：
- ✅ `platform = x86`（amd64/x86_64 NAS 都用这个）
- ❌ `platform = x86_64` → 报"当前设备无法安装该应用"
- ❌ `platform = all`（虽不报错但显示不准确，部分 fnOS 版本不识别）

**经验**：参考 fnOS 上能正常安装的 app（mihomo/tailscale 都是 `x86`）。

### 6. schema 自动迁移
在 `db.go` 中使用 `migrateSchema()` 自动添加新列（如 `last_status`、`last_run_at`），避免老 db 升级后 SQL 错误。

```go
func migrateSchema(conn *sql.DB) error {
    for _, col := range []string{"last_status TEXT DEFAULT ''", ...} {
        // 检查列是否存在，不存在则 ALTER TABLE 添加
    }
}
```

### 7. 监听 127.0.0.1 而非 :port
Go server 监听 `127.0.0.1:port` 而不是 `:port`，避免暴露到外网（fnOS 用 `port_forward="yes"` 转发到 fnOS 网关）。

### 8. Settings 立即生效
`handleSettings` 在时区变更时调用 `serverCmd.Stop()` + 重建 cron + `Start()`，使设置变更立即生效而不是需要重启。

## fnOS 桌面入口（FPK 在桌面显示和打开）

必须有 `ui/` 目录：
- `ui/config` — JSON 告诉 fnOS 怎么打开（iframe 模式、端口、URL）
- `ui/api.cgi` — CGI 代理到后端 API（兼容 fnOS 桌面框架的旧 action 风格）
- `ui/index.cgi` — 加载页面（meta refresh 跳到 `/`）
- `ui/images/icon_64.png` + `icon_256.png` — 图标
- manifest 里 `desktop_applaunchname` 必须和 `ui/config` 的 `.url` 键一致

## 健康检查 (health.json)

fnOS 桌面右下角的"健康"指示灯：
```json
{
  "checks": [
    {"name": "service_listening", "type": "port", "port": 7681, "interval": 30, "fail_threshold": 3},
    {"name": "api_healthz", "type": "http", "path": "/api/healthz", "expected_status": 200, "interval": 30}
  ]
}
```

## API 端点

| Method | Path | 说明 |
|---|---|---|
| GET | `/api/healthz` | 健康检查 |
| GET | `/api/stats` | 仪表盘统计 |
| GET | `/api/jobs` | 任务列表 |
| POST | `/api/jobs` | 创建任务 |
| GET | `/api/jobs/<id>` | 单个任务 |
| PUT | `/api/jobs/<id>` | 更新任务 |
| DELETE | `/api/jobs/<id>` | 删除任务 |
| POST | `/api/jobs/<id>/run` | 立即执行 |
| POST | `/api/jobs/<id>/toggle` | 启用/禁用 |
| GET | `/api/jobs/<id>/runs` | 任务历史 |
| GET | `/api/runs?limit=50` | 全局历史 |
| GET | `/api/runs/<id>` | 单次执行详情 |
| GET | `/api/runs/<id>/log` | SSE 实时日志流 |
| GET | `/api/settings` | 获取设置 |
| PUT | `/api/settings` | 更新设置（时区变更立即生效） |
| GET | `/api/log?lines=200` | 服务端日志尾部 |
| POST | `/api/cleanup?days=30` | 清理历史 |

## 测试

### 本地 macOS
```bash
mkdir -p /tmp/scheduler-test/data
TRIM_APPDEST=/tmp/scheduler-test \
TRIM_PKGVAR=/tmp/scheduler-test/data \
/tmp/fnos-scheduler &
sleep 1
curl http://127.0.0.1:7681/api/healthz
# → {"ok":true,"time":"..."}
```

### Docker
```bash
docker run --rm --platform linux/amd64 \
  -v /path/to/dist:/work debian:bookworm sh -c '
    apt update && apt install -y curl procps
    mkdir -p /var/apps/scheduler/data
    tar xzf /work/scheduler-0.0.0.fpk
    TRIM_APPDEST=/var/apps/scheduler TRIM_PKGVAR=/var/apps/scheduler/data \
      sh /var/apps/scheduler/cmd/main start
    curl http://127.0.0.1:7681/api/healthz
  '
```

## 已知问题 / 待办
- [ ] 国际化（目前只有中文）
- [ ] 通知渠道（失败时只有日志，没有 webhook/邮件）
- [ ] 多用户隔离（当前所有任务以单一身份执行）
