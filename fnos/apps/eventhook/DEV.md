# eventhook — 系统事件监听与钩子服务

## 功能

监听 fnOS 系统 eventlogger 数据库（`/usr/trim/var/eventlogger_service/logger_data.db3`），识别登录、SSH、硬盘、UPS、应用异常等事件，通过配置的钩子（Webhook/Bark/PushPlus/钉钉/企业微信/本地脚本）执行通知或响应操作。

## 编译

```bash
cd apps/eventhook/www
GOOS=linux GOARCH=amd64 go build -o /tmp/fnos-eventhook -ldflags="-s -w" .
```

## 开发说明

- 纯 Go，无 CGO 依赖
- 使用 `modernc.org/sqlite`（纯 Go SQLite，无需 CGO）
- 本地交叉编译即可（macOS → linux/amd64）

## 架构

```
eventhook server (Go, port 7683)
  ├── Watcher goroutine — 定时轮询 eventlogger DB
  │   ├── 读取新事件（基于 cursor > last_id）
  │   ├── 去重检测（configurable dedup window）
  │   └── 勿扰模式判定
  ├── Hook Executor — 匹配规则并执行
  │   ├── Webhook POST
  │   ├── Bark 推送
  │   ├── PushPlus 推送
  │   ├── 钉钉机器人
  │   ├── 企业微信机器人
  │   └── 本地脚本执行（脚本可读 EVENT_TYPE/EVENT_DETAIL/EVENT_TIME 环境变量）
  └── HTTP API
      ├── /api/healthz — 健康检查
      ├── /api/hooks — CRUD 钩子规则
      ├── /api/events — 事件日志查询
      ├── /api/settings — 全局设置（轮询间隔、去重窗口、勿扰模式、DB 路径）
      ├── /api/stats — 统计信息
      └── /api/log — 服务端日志
```

## 事件类型

| 类别 | 事件 |
|------|------|
| 登录 | LoginSucc, LoginSucc2FA1, LoginFail, Logout |
| SSH | SSH_INVALID_USER, SSH_AUTH_FAILED, SSH_LOGIN_SUCCESS, SSH_DISCONNECTED |
| 硬盘 | FoundDisk, DiskWakeup, DiskSpindown, DISK_IO_ERR |
| UPS | UPS_ONBATT, UPS_ONBATT_LOWBATT, UPS_ONLINE, UPS_ENABLE, UPS_DISABLE |
| 应用 | APP_CRASH, APP_STARTED, APP_STOPPED, APP_UPDATED, APP_INSTALLED, APP_UNINSTALLED, APP_UPDATE_FAILED, APP_START_FAILED, APP_AUTO_START_FAILED |
| 硬件 | CPU_USAGE_ALARM, CPU_TEMPERATURE_ALARM |
| 存储 | STORAGE_DEGRADED, STORAGE_DAMAGED |
| 虚拟机 | VM_START, VM_STOP, VM_CRASH, VM_PAUSE, VM_RESUME |

## 测试

```bash
# 本地测试（无需 fnOS 环境）
mkdir -p /tmp/eventhook-test/data
TRIM_APPDEST=/tmp/eventhook-test \
TRIM_PKGVAR=/tmp/eventhook-test/data \
TRIM_SERVICE_PORT=7683 \
/tmp/fnos-eventhook &
sleep 1
curl http://127.0.0.1:7683/api/healthz
```

## 钩子脚本示例

事件钩子执行脚本时传入以下环境变量：
- `EVENT_TYPE` — 事件类型（如 LoginSucc）
- `EVENT_DETAIL` — 事件详情
- `EVENT_TIME` — 事件发生时间
- `APP_DEST` — 应用安装目录
- `APP_VAR` — 应用数据目录

示例脚本 `/path/to/notify.sh`:
```bash
#!/bin/sh
curl -s "https://api.day.app/YOUR_BARK_KEY/$EVENT_TYPE?body=$(echo $EVENT_DETAIL | jq -sRr @uri)"
```
