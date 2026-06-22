# Mihomo App 安全审计报告

> 审计日期: 2026-06-16
> 审计目标: `www/main.go` + `www/ui/index.html`
> 审计类型: 协程竞争、并发安全、JS 暂存死区、定时器泄漏、CSS 布局

---

## 🔴 高危

### H1. `startMihomo()` 无防重入，并发重启产生孤儿进程

**位置**: `www/main.go:366-400`

**问题**: `handleRestart` (L651) 和 `handleSettings` (L826) 都在 goroutine 中依次调用 `stopMihomo()` + `startMihomo()`，但两个操作之间没有原子锁保护：

```
Goroutine A: stopMihomo() → [释放锁] → startMihomo() → [mihomoCmd = new]
Goroutine B: stopMihomo() → [mihomoCmd = nil] → startMihomo() → [mihomoCmd = new2]
```

`startMihomo()` 内部从不检查 `mihomoCmd` 是否已在运行，并发场景下同一时间存在多个 mihomo 进程。

**修复**: `startMihomo()` 加锁后先判断：

```go
func startMihomo() error {
    mihomoMu.Lock()
    defer mihomoMu.Unlock()
    if mihomoCmd != nil {
        return fmt.Errorf("already running")
    }
    // ... 启动逻辑
}
```

或由调用侧保证原子性：

```go
func safeRestart() {
    mihomoMu.Lock()
    defer mihomoMu.Unlock()
    stopLocked()
    startLocked()
}
```

---

### H2. `stopMihomo()` 在锁内执行 `Wait()`，阻塞全局 API

**位置**: `www/main.go:402-416`

**问题**: `mihomoCmd.Wait()` 在持有 `mihomoMu` 时执行。mihomo 进程收到 SIGTERM 后有 5 秒宽限 + 实际退出时间，期间：

- `isMihomoRunning()` → 全部挂起
- `/api/status`、`/api/settings`、`handleProxy`、`handleEvents` → 全部阻塞
- 最长可能数十秒无响应

**修复**: 先释放锁再 Wait，避免 `stopMihomo` 阻塞全局：

```go
func stopMihomo() {
    mihomoMu.Lock()
    cmd := mihomoCmd
    mihomoCmd = nil
    mihomoMu.Unlock()

    if cmd == nil || cmd.Process == nil { return }
    cmd.Process.Signal(syscall.SIGTERM)
    go func() { time.Sleep(5 * time.Second); cmd.Process.Kill() }()
    cmd.Wait()
}
```

---

## 🟡 中危

### M1. 前端三个 `setInterval` 永不销毁 (Log 页除外)

**位置**: `www/ui/index.html:138,150,244`

**问题**:

```js
setInterval(dc, 3000);     // 流量图轮询 /api/status（与 st 重复）
setInterval(lp, 10000);    // 代理组轮询 /api/proxy/proxies
setInterval(st, 5000);     // 状态轮询 /api/status
```

- 无论用户在哪一页都持续运行
- `dc` 和 `st` 都调用 `/api/status`，每秒约 0.53 次重复请求
- 只有日志页面实现了 `stopLogRefresh`，其他页面没有 stop 机制

**修复**:
- `dc` 复用 `st` 的数据，去掉独立 `/api/status` 请求
```js
// dc 改为从 td 取最近一次 st 的数据，不单独调 API
const dc = () => { if (td.length > 0) drawChart(); };
setInterval(dc, 3000);
// 在 st() 末尾更新 td 数据
if (d.rx !== undefined) {
    td.push({ rx: d.rx, tx: d.tx });
    if (td.length > 60) td.shift();
}
```
- 或切换页面时 stop/restart 全部定时器

---

## 🟢 低危

### L1. LogBuffer 双层锁（无安全风险）

**位置**: `www/main.go:878-893`

`LogBuffer.Write()` 带 `sync.Mutex`，而 `log.SetOutput(&logBuf)` 后 log 包内部也有全局锁。双层锁不影响正确性，仅轻微性能开销。当前实现已验证安全。

### L2. JS 暂存死区 — 未发现问题

逐行追踪了 `www/ui/index.html:107-245` 的执行顺序。所有 `const` 声明的函数被 onclick 闭包捕获时仅形成引用，求值发生在用户交互之后，此时所有声明已完成初始化。**无 TDZ 风险。**

### L3. CSS 内联样式不干扰显隐路由

`.pt { display:none }` / `.pt.act { display:block }` 的路由机制未被任何 `id` 级的内联 `style="display:flex"` 覆盖。Log 页面 `#pt-lg` 只有 child 元素上有内联 flex，不干扰 `.pt` 的 display 切换。**布局无泄露。**

---

## 修复优先级

| 优先级 | 问题 | 影响 |
|--------|------|------|
| P0 | H1 孤儿进程 | 进程泄漏，端口冲突 |
| P0 | H2 全局阻塞 | 服务无响应数十秒 |
| P1 | M1 定时器浪费 | 非活跃页面持续轮询 |
