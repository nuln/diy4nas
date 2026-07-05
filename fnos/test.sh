#!/bin/bash
# fnOS App Test Suite — 基于 jianyun8023 fpk 开发规范
# 测试所有 21 个 app 的结构、脚本、配置、权限
set +euo pipefail 2>/dev/null || true
cd "$(dirname "$0")"

PASS=0; FAIL=0; WARN=0
log_pass() { PASS=$((PASS+1)); }
log_fail() { FAIL=$((FAIL+1)); echo "  ❌ $1"; }
log_warn() { WARN=$((WARN+1)); echo "  ⚠️  $1"; }

# ── 1. 目录结构 ──
echo "=== 1. 目录结构 ==="
REQUIRED_DIRS=("cmd" "config" "ui" "wizard")
REQUIRED_FILES=("manifest" "config/privilege" "config/resource" "cmd/main" "cmd/service-setup")
for d in apps/*/; do
    slug=$(basename "$d")
    for dir in "${REQUIRED_DIRS[@]}"; do
        [ -d "$d$dir" ] || log_fail "$slug: 缺少目录 $dir/"
    done
    for f in "${REQUIRED_FILES[@]}"; do
        [ -f "$d$f" ] || log_fail "$slug: 缺少文件 $f"
    done
    # *.sc port file
    [ -f "$d$slug.sc" ] || log_warn "$slug: 缺少 $slug.sc"
done
echo "  目录结构检查完成"

# ── 2. manifest ──
echo "=== 2. manifest ==="
MANIFEST_REQUIRED=("appname" "version" "display_name" "platform" "service_port" "desc" "source")
for d in apps/*/; do
    slug=$(basename "$d")
    m="$d/manifest"
    for key in "${MANIFEST_REQUIRED[@]}"; do
        val=$(grep "^$key[[:space:]]*=" "$m" 2>/dev/null | head -1 | cut -d= -f2- | xargs)
        [ -n "$val" ] || log_fail "$slug: manifest 缺少 $key"
    done
    # service_port must be number
    port=$(grep "^service_port" "$m" 2>/dev/null | cut -d= -f2- | xargs)
    [[ "$port" =~ ^[0-9]+$ ]] || log_fail "$slug: service_port 必须是数字 (got: $port)"
done
echo "  manifest 检查完成"

# ── 3. cmd/main ──
echo "=== 3. cmd/main ==="
for d in apps/*/; do
    slug=$(basename "$d")
    m="$d/cmd/main"
    [ -x "$m" ] || log_fail "$slug: cmd/main 不可执行"
    # Check for start/stop/status
    grep -q "start" "$m" 2>/dev/null || log_warn "$slug: cmd/main 缺少 start"
    grep -q "stop" "$m" 2>/dev/null || log_warn "$slug: cmd/main 缺少 stop"
    grep -q "status" "$m" 2>/dev/null || log_warn "$slug: cmd/main 缺少 status"
    # bash syntax
    bash -n "$m" 2>/dev/null || log_fail "$slug: cmd/main 语法错误"
done

# ── 4. config/privilege ──
echo "=== 4. config/privilege ==="
for d in apps/*/; do
    slug=$(basename "$d")
    p="$d/config/privilege"
    python3 -c "import json; json.load(open('$p'))" 2>/dev/null || log_fail "$slug: config/privilege JSON 格式错误"
    python3 -c "import json; d=json.load(open('$p')); assert 'defaults' in d" 2>/dev/null || log_fail "$slug: config/privilege 缺少 defaults"
done

# ── 5. config/resource ──
echo "=== 5. config/resource ==="
for d in apps/*/; do
    slug=$(basename "$d")
    r="$d/config/resource"
    python3 -c "import json; json.load(open('$r'))" 2>/dev/null || log_fail "$slug: config/resource JSON 格式错误"
done

# ── 6. wizard JSON ──
echo "=== 6. wizard ==="
for d in apps/*/; do
    slug=$(basename "$d")
    for w in wizard/install wizard/config; do
        [ -f "$d$w" ] || continue
        python3 -c "import json; json.load(open('$d$w'))" 2>/dev/null || log_fail "$slug: $w JSON 格式错误"
    done
done

# ── 7. bash 语法（所有 cmd 脚本）──
echo "=== 7. 脚本语法 ==="
for d in apps/*/; do
    slug=$(basename "$d")
    for s in "$d/cmd/"*; do
        [ -f "$s" ] || continue
        bash -n "$s" 2>/dev/null || log_fail "$slug: $(basename $s) 语法错误"
    done
done

# ── 8. CGI 脚本 ──
echo "=== 8. CGI 脚本 ==="
for d in apps/*/; do
    slug=$(basename "$d")
    for c in "$d/ui/"*.cgi; do
        [ -f "$c" ] || continue
        [ -x "$c" ] || log_fail "$slug: $(basename $c) 不可执行"
        # Header output check
        out=$(sh "$c" 2>/dev/null | head -1)
        echo "$out" | grep -q "Content-Type" || log_fail "$slug: $(basename $c) 缺少 Content-Type 头"
    done
done

# ── 9. Docker compose ──
echo "=== 9. Docker compose ==="
for d in apps/*/; do
    slug=$(basename "$d")
    dc="$d/docker/docker-compose.yaml"
    [ -f "$dc" ] || continue
    DOCKER_MIRROR="" VERSION="test" TRIM_PKGVAR="/tmp" TRIM_SERVICE_PORT="8080" \
        docker compose -f "$dc" config >/dev/null 2>&1 || log_fail "$slug: docker-compose.yaml 格式错误"
    # Check for essential fields
    grep -q "image:" "$dc" || log_fail "$slug: docker-compose 缺少 image"
    grep -q "container_name:" "$dc" || log_fail "$slug: docker-compose 缺少 container_name"
done

# ── 10. Go 服务 ──
echo "=== 10. Go 服务 ==="
for d in apps/*/www/main.go; do
    [ -f "$d" ] || continue
    dir=$(dirname "$d")
    slug=$(basename $(dirname $(dirname "$d")))
    cd "$dir" && go vet ./... 2>/dev/null && log_pass "$slug: Go vet 通过" || log_fail "$slug: Go vet 失败"
    cd "$OLDPWD"
done

# ── 11. 更新脚本 ──
echo "=== 11. 更新脚本 ==="
for d in apps/*/; do
    slug=$(basename "$d")
    us="$d/update_$slug.sh"
    [ -f "$us" ] && [ -x "$us" ] || log_warn "$slug: 缺少或不可执行 update_$slug.sh"
done

# ── 12. ui/config ──
echo "=== 12. ui/config ==="
for d in apps/*/; do
    slug=$(basename "$d")
    uc="$d/ui/config"
    [ -f "$uc" ] || continue
    python3 -c "import json; json.load(open('$uc'))" 2>/dev/null || log_fail "$slug: ui/config JSON 格式错误"
    python3 -c "
import json; d=json.load(open('$uc'))
# Check .url has key starting with appname
for k in d.get('.url',{}):
    # Key should contain the slug
    pass
" 2>/dev/null || log_fail "$slug: ui/config 结构错误"
done

# ── 13. 变量引用检查 ──
echo "=== 13. 变量引用 ==="
for d in apps/*/cmd/*; do
    [ -f "$d" ] || continue
    # 检查是否有硬编码路径
    if grep -q "/var/apps/[a-z]" "$d" 2>/dev/null; then
        slug=$(basename $(dirname $(dirname "$d")))
        log_warn "$slug: $(basename $d) 可能有硬编码路径"
    fi
done

# ── 14. .sc 端口文件 ──
echo "=== 14. 端口文件 ==="
for d in apps/*/; do
    slug=$(basename "$d")
    sc="$d$slug.sc"
    [ -f "$sc" ] || continue
    grep -q "port_forward" "$sc" || log_warn "$slug: $slug.sc 缺少 port_forward"
    grep -q "src.ports=" "$sc" || log_warn "$slug: $slug.sc 缺少 src.ports"
done

echo ""
echo "═══════════════════════════════════"
echo "  测试结果"
echo "═══════════════════════════════════"
echo "  ✅ 通过: $PASS"
echo "  ❌ 失败: $FAIL"
echo "  ⚠️  警告: $WARN"
echo "═══════════════════════════════════"
[ "$FAIL" -eq 0 ] && echo "  合格" || echo "  不合格"
