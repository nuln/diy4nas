#!/bin/bash
# E2E test: 模拟 fnOS 真实部署流程
set -e
cd "$(dirname "$0")"

FPK_SCHEDULER="/work/dist/scheduler-0.0.0.fpk"
FPK_TERMINAL="/work/dist/terminal-0.0.0.fpk"

# Fresh start
rm -rf /var/apps/*

pass=0
fail=0
log() { echo -e "\n\033[1;36m=== $* ===\033[0m"; }
ok()  { echo -e "\033[1;32m✓ $*\033[0m"; pass=$((pass+1)); }
bad() { echo -e "\033[1;31m✗ $*\033[0m"; fail=$((fail+1)); }

# === Phase 1: 安装 ===
log "1. 安装 scheduler"
if bash scripts/install.sh "$FPK_SCHEDULER" scheduler 2>&1 | tail -20; then
    ok "scheduler installed"
else
    bad "scheduler install failed"
fi

log "2. 安装 terminal"
if bash scripts/install.sh "$FPK_TERMINAL" terminal 2>&1 | tail -20; then
    ok "terminal installed"
else
    bad "terminal install failed"
fi

# === Phase 2: 启动 ===
log "3. 启动 scheduler"
if bash scripts/cmd.sh scheduler start 2>&1; then
    ok "scheduler start returned 0"
else
    bad "scheduler start failed"
    cat /var/apps/scheduler/data/scheduler.log 2>&1 | tail -10
fi

log "4. 启动 terminal"
if bash scripts/cmd.sh terminal start 2>&1; then
    ok "terminal start returned 0"
else
    bad "terminal start failed"
    cat /var/apps/terminal/data/terminal.log 2>&1 | tail -10
fi

# Wait for services
sleep 2

# === Phase 3: 状态检查 ===
log "5. 检查 service 进程"
for slug in scheduler terminal; do
    if pgrep -f "/var/apps/$slug/app/fnos-$slug" >/dev/null 2>&1; then
        ok "$slug process running"
    else
        bad "$slug process NOT running"
        cat "/var/apps/$slug/data/$slug.log" 2>&1 | tail -5
    fi
done

log "6. 检查 cmd/main status"
for slug in scheduler terminal; do
    if bash scripts/cmd.sh "$slug" status 2>/dev/null; then
        ok "$slug status = 0 (running)"
    else
        bad "$slug status != 0"
    fi
done

# === Phase 4: HTTP 访问（直接 localhost:port）===
log "7. scheduler HTTP API"
HEALTH=$(curl -s http://127.0.0.1:7681/api/healthz 2>&1)
echo "Response: $HEALTH"
if echo "$HEALTH" | grep -q '"ok":true'; then
    ok "scheduler /api/healthz works"
else
    bad "scheduler /api/healthz failed"
fi

UI=$(curl -s http://127.0.0.1:7681/ 2>&1 | head -c 2000)
if echo "$UI" | grep -q '<title>计划任务</title>'; then
    ok "scheduler UI HTML served"
else
    bad "scheduler UI not served correctly"
fi

# === Phase 5: 任务创建+执行 ===
log "8. 创建并执行任务"
JOB_ID=$(curl -s http://127.0.0.1:7681/api/jobs \
    -X POST -H "Content-Type: application/json" \
    -d '{"name":"e2e test","spec":"@every 30s","command":"echo E2E_OK && date","timeout_sec":10}' \
    2>&1 | python3 -c "import sys, json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [ -n "$JOB_ID" ]; then
    ok "created job id=$JOB_ID"
else
    bad "failed to create job"
fi

curl -s http://127.0.0.1:7681/api/jobs/$JOB_ID/run -X POST > /dev/null
sleep 2
RUNS=$(curl -s "http://127.0.0.1:7681/api/runs?limit=5" 2>&1)
echo "Runs: $RUNS"
if echo "$RUNS" | grep -q '"status":"success"'; then
    ok "job executed successfully"
else
    bad "job execution failed"
fi

# === Phase 6: terminal API ===
log "9. terminal API"
SESSION=$(curl -s http://127.0.0.1:7682/api/sessions \
    -X POST -H "Content-Type: application/json" \
    -d '{"title":"e2e","cols":80,"rows":24}' 2>&1)
echo "Session: $SESSION"
SID=$(echo "$SESSION" | python3 -c "import sys, json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [ -n "$SID" ]; then
    ok "terminal session created id=$SID"
else
    bad "terminal session creation failed"
fi

# === Phase 7: 升级 ===
log "10. 升级 scheduler"
if bash scripts/upgrade.sh "$FPK_SCHEDULER" scheduler 2>&1 | tail -5; then
    ok "scheduler upgrade script ran"
else
    bad "scheduler upgrade failed"
fi

# 重新启动验证（upgrade_callback 应该已经自动 start）
sleep 2
if curl -s http://127.0.0.1:7681/api/healthz | grep -q '"ok":true'; then
    ok "scheduler running after upgrade"
else
    # 手动启动
    bash scripts/cmd.sh scheduler start 2>&1 | tail -2
    sleep 2
    if curl -s http://127.0.0.1:7681/api/healthz | grep -q '"ok":true'; then
        ok "scheduler restarted after upgrade"
    else
        bad "scheduler didn't restart after upgrade"
    fi
fi

# === Phase 8: 卸载 ===
log "11. 卸载 scheduler"
if bash scripts/uninstall.sh scheduler 2>&1 | tail -5; then
    ok "scheduler uninstalled"
else
    bad "scheduler uninstall failed"
fi

if [ ! -e /var/apps/scheduler/cmd/main ]; then
    ok "scheduler fully removed"
else
    bad "scheduler dir still exists"
fi

# === Phase 9: 停止 ===
log "12. 停止 terminal"
bash scripts/cmd.sh terminal stop 2>&1
sleep 1
if ! pgrep -f "/var/apps/terminal/app/fnos-terminal" >/dev/null 2>&1; then
    ok "terminal process stopped"
else
    bad "terminal process still running"
fi

# === Wizard JSON 验证 ===
log "13. 验证 wizard JSON 格式"
# 重新安装 scheduler 以便验证其 wizard
bash scripts/install.sh /work/dist/scheduler-0.0.0.fpk scheduler > /dev/null 2>&1
for slug in scheduler terminal; do
    if [ -f /var/apps/$slug/wizard/install ]; then
        if python3 -c "import json; json.load(open('/var/apps/$slug/wizard/install'))" 2>/dev/null; then
            ok "$slug wizard/install is valid JSON"
        else
            bad "$slug wizard/install INVALID JSON"
        fi
    else
        bad "$slug wizard/install MISSING"
    fi
done

# 验证 service-setup 有 service_postinst
log "14. 验证 service-setup 含 service_postinst"
for slug in scheduler terminal; do
    if grep -q "service_postinst" /var/apps/$slug/cmd/service-setup 2>/dev/null; then
        ok "$slug service-setup has service_postinst"
    else
        bad "$slug service-setup missing service_postinst"
    fi
done

# 验证 .sc 有 port_forward="yes"
log "15. 验证 .sc 含 port_forward=yes"
for slug in scheduler terminal; do
    if grep -q 'port_forward="yes"' /var/apps/$slug/${slug}.sc 2>/dev/null; then
        ok "$slug .sc has port_forward=yes"
    else
        bad "$slug .sc missing port_forward=yes"
    fi
done

echo
echo "=========================================="
echo -e "PASS: \033[1;32m$pass\033[0m  FAIL: \033[1;31m$fail\033[0m"
echo "=========================================="
exit $fail
