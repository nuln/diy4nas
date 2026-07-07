# Tailscale App: 黑屏 / Connection Refused

## 症状
- App Center 启动后页面黑屏 或 ERR_CONNECTION_REFUSED
- SSH `sudo cmd/main start` 手动启动正常

## 已知工作版本
- `0e15101` commit — 用户确认从 App Center 启动工作
- 之后所有修改版本均不工作

## 0e15101 vs HEAD (534e46b) 差异
```
git diff 0e15101..534e46b -- apps/tailscale/www/main.go
```

### 改动清单（按可疑程度排序）

1. **`retryTailscaled()` goroutine** ← 最可疑
   - 后台 goroutine 10s 后执行 startTailscaled()
   - goroutine panic → 杀死整个进程 → connection refused
   - 同时启动多个 tailscaled 实例 → state 文件损坏

2. **`handleProxy` 移除 restartTailscaled()**
   - 0e15101: 保存代理后自动重启 tailscaled
   - HEAD: 只保存到文件，不重启

3. **`handleUp` 增加 proxy 重连逻辑**
   - `restartTailscaled()` + bare `tailscale up`

4. **`lastUpFlags` + 持久化**（写 JSON 文件）

5. **`startTailscaled()` socket 超时从 10s→60s**（0.0.6 改的）

6. **`findBin` 增加 `/target/app` 搜索路径**

### 当前版本 (HEAD) 的修复尝试
- `retryTailscaled()` 已删除（0.0.6）
- `stopTailscaled()` 有 nil check
- 启动失败不会 `log.Fatalf`（继续启动 HTTP）

## 当前代码状态

### startTailscaled() (main.go:186)
- 始终设置 tsdCmd.Env = filtered os.Environ()（过滤掉 HTTP_PROXY 家族）
- 代理从 proxyFile（appVar + "/proxy.conf"）读取

### main() (main.go:260)
- 写 debug 日志到 /tmp/fnos-tailscale-debug.log
- startTailscaled() 失败后继续启动 HTTP（非 fatal）
- `defer stopTailscaled()` 始终执行

### cmd/main
- `nohup "$SRV" >> "$LOG_FILE" 2>&1 &` — 防止 App Center SIGHUP
- `kill_by_pid()` — PID 文件 + /proc 校验
- 启动后做 curl health check（不 fatal）

## 调试手段
- 日志: `/tmp/fnos-tailscale-debug.log`（Go server 启动日志）
- 日志: `${TRIM_PKGVAR}/data/tailscaled.log`（tailscaled 日志）
- 日志: `${TRIM_TEMP_LOGFILE}`（cmd/main 错误报告）
```

