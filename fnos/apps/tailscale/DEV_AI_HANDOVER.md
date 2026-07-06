# Tailscale FPK 安装失败 — AI 交接文档

## 问题描述

安装了 `dist/tailscale-0.0.0.fpk` 后，NAS 上 `ls /vol1/@appcenter/tailscale/` 只显示 `app/`，缺少 `cmd/`、`config/`、`manifest`、`wizard/` 等元数据文件。

对比：`scheduler` 和 `terminal` FPK 在同一台 NAS 上安装正常。

## 已修复的问题

### 1. Proxy 保存时重启 tailscaled 导致 connection refused
- `apps/tailscale/www/main.go` 中 `handleProxy` POST 不要 `restartTailscaled()`
- 只写入 `proxy.conf` 文件，不重启进程（重启交给用户点"重连"）

### 2. api.cgi / index.cgi 缺失
- 误删了 `apps/tailscale/ui/api.cgi` 和 `apps/tailscale/ui/index.cgi`
- 其他 app（scheduler, terminal）都有这两个文件
- 已恢复

**但即使修复了这两个问题，用户 NAS 上安装后仍然只显示 `app/`。**

## FPK 结构对比（scheduler 正常 vs tailscale 异常）

### FPK 根目录（两者结构相同，都不是问题）
```
scheduler.fpk:                  tailscale.fpk:
  app.tgz                         app.tgz
  cmd/                            cmd/
  config/                         config/
  ICON.PNG                        ICON.PNG
  ICON_256.PNG                    ICON_256.PNG
  manifest                        manifest
  scheduler.sc                    tailscale.sc
  wizard/                         wizard/
```

### app.tgz 内容（这是关键差异！）

| scheduler (正常) | tailscale (异常) |
|---|---|
| `app/fnos-scheduler` | `app/fnos-tailscale` |
| `app/ui/` | `app/ui/` |
| `app/ui/api.cgi` | `app/ui/api.cgi` |
| `app/ui/config` | `app/ui/config` |
| `app/ui/index.cgi` | `app/ui/index.cgi` |
| `app/ui/images/icon_256.png` | `app/ui/images/icon_256.png` |
| `app/ui/images/icon_64.png` | `app/ui/images/icon_64.png` |
| — | `app/ui/images/ICON.PNG`（大写多余） |
| — | **`app/systemd/`（多余！）** |
| — | `app/systemd/tailscaled.service` |
| — | `app/systemd/tailscaled.defaults` |
| — | `app/systemd/tailscale-wait-online.service` |
| — | `app/systemd/tailscale-online.target` |
| — | `app/tailscale`（官方二进制） |
| — | `app/tailscaled`（官方二进制） |

### 其他差异

| scheduler | tailscale |
|---|---|
| `health.json` 在 FPK 根目录 | **没有** `health.json` |
| FPK 大小: 4.1 MB | FPK 大小: **38 MB** |
| `apps/scheduler/ui/` 4 个文件 | `apps/tailscale/ui/` 5 个文件（多 vis-network.min.js） |

## 问题根因（最可能的）

### 1. `app/systemd/` 目录（最可疑）
`apps/tailscale/build.sh` 第 7 行：
```bash
curl -fsSL "$URL" | tar xz -C "$BUILD_DIR" --strip-components=1
```
tailscale 官方 tgz 包含 `systemd/` 目录，不分青红皂白全解压进了 `$BUILD_DIR`。打包 FPK 时所有文件都进了 `app.tgz`，包括 `app/systemd/`。

fnOS 可能在处理 app.tgz 时遇到不认识的目录/文件类型导致静默失败，只解了部分内容。

### 2. `ICON.PNG`（大写）多余
app/ui/images/ 里有三个文件：
- `ICON.PNG`（大写，来自 tailscale 官方包，可能不是 64x64）
- `icon_256.png`（小写，来自 apps/tailscale/ui/images/）
- `icon_64.png`（小写，来自 apps/tailscale/ui/images/）

fnOS 可能被冗余文件搞混。

### 3. 缺少 `health.json`
scheduler 根目录有 `health.json`，tailscale 没有。可能 fnOS 读取这个文件做安装后检查，找不到就回滚。

### 4. FPK 太大（38 MB）
怀疑 fnOS 有 FPK 大小限制。建议测试。

## 建议修复步骤

1. **修改 `apps/tailscale/build.sh`**：解压官方 tgz 后删除 `systemd/` 目录和多余图标
```bash
# 第 7 行后加：
rm -rf "$BUILD_DIR/systemd" "$BUILD_DIR/ui/images/ICON.PNG"
```

2. **添加 `health.json`**：参考 `apps/scheduler/health.json`，把端口改成 8088

3. **清理 `apps/tailscale/ui/` 无用文件**：如果 `vis-network.min.js` 没用就删掉

4. **重新 build**：
```bash
cd /Users/dukangxu/nuln/diy4nas/fnos
rm -rf dist/tailscale-0.0.0  # 清缓存
bash build.sh tailscale
```

5. **验证 FPK**：
```bash
# 检查 app/systemd 已不存在
tar -tf dist/tailscale-0.0.0.fpk
bash -c 'tar xzf dist/tailscale-0.0.0.fpk -O app.tgz | tar -tzf -' | grep systemd
# 应该没输出

# Docker 安装测试
mkdir -p /tmp/tailscale-test
cp dist/tailscale-0.0.0.fpk /tmp/tailscale-test/
docker run --rm --platform linux/amd64 \
  -v /tmp/tailscale-test:/work \
  debian:bookworm-slim bash -c '
    apt update -qq && apt install -y -qq curl procps libcap2-bin kmod
    mkdir -p /vol1/@appcenter/tailscale /vol1/@appdata/tailscale/data
    tar xzf /work/tailscale-0.0.0.fpk -C /vol1/@appcenter/tailscale
    cd /vol1/@appcenter/tailscale && tar xzf app.tgz && rm app.tgz
    ls -la  # 确认有 cmd/ config/ manifest wizard/ app/
  '
```

6. **transfer FPK** → `scp` 到 NAS → App Center → 手动安装

7. **验证安装**：在 NAS SSH 里 `ls /vol1/@appcenter/tailscale/`

## Docker 测试确认

当前 FPK 在 Docker 中安装正常（提取 FPK → 解 app.tgz → 全部文件出现）：

```
/vol1/@appcenter/tailscale/
├── cmd/
├── config/
├── manifest
├── wizard/
├── app/
├── ICON.PNG
├── ICON_256.PNG
├── tailscale.sc
```

但怀疑 `app/systemd/` 文件在 fnOS 真实环境中触发了 bug。

## 代码状态

| 文件 | 状态 |
|---|---|
| `apps/tailscale/www/main.go` | 已修复（handleProxy 不重启） |
| `apps/tailscale/www/ui/index.html` | 已修复（代理输入框） |
| `apps/tailscale/ui/api.cgi` | **已恢复**（之前被误删） |
| `apps/tailscale/ui/index.cgi` | **已恢复**（之前被误删） |
| `apps/tailscale/build.sh` | **待修复**（需清理 systemd/ 和多余图标） |
| `dist/tailscale-0.0.0.fpk` | 38MB，含 systemd/（需重建） |
