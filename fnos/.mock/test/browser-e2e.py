#!/usr/bin/env python3
"""Playwright 浏览器 E2E：通过 nginx 反代模拟 fnOS 桌面访问"""
import asyncio
import os
import subprocess
import sys
from playwright.async_api import async_playwright

NGINX_CONF = """worker_processes 1;
pid /tmp/nginx.pid;
error_log /tmp/nginx-logs/error.log;
events { worker_connections 64; }
http {
    map $http_upgrade $connection_upgrade {
        default upgrade;
        "" close;
    }
    server {
        listen 8080;
        location /app/scheduler/ {
            proxy_pass http://127.0.0.1:7681/;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_buffering off;
        }
        location /app/terminal/ {
            proxy_pass http://127.0.0.1:7682/;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 86400;
            proxy_buffering off;
        }
    }
}
"""

def start_nginx():
    os.makedirs("/tmp/nginx-logs", exist_ok=True)
    with open("/tmp/nginx-test.conf", "w") as f:
        f.write(NGINX_CONF)
    subprocess.run(["nginx", "-t", "-c", "/tmp/nginx-test.conf"], check=True, capture_output=True)
    subprocess.run(["nginx", "-c", "/tmp/nginx-test.conf"], check=True)
    import time; time.sleep(1)

def stop_nginx():
    subprocess.run(["nginx", "-c", "/tmp/nginx-test.conf", "-s", "stop"], capture_output=True)

def ensure_services():
    """Install + start both apps if not running."""
    import time
    for slug, fpk in [("scheduler", "/work/dist/scheduler-0.0.0.fpk"),
                      ("terminal", "/work/dist/terminal-0.0.0.fpk")]:
        # Stop if running
        try:
            subprocess.run(["bash", f"/work/.mock/scripts/cmd.sh", slug, "stop"],
                          capture_output=True, timeout=10)
        except: pass
        # Reinstall
        subprocess.run(["bash", "/work/.mock/scripts/install.sh", fpk, slug],
                      check=True, capture_output=True)
        # Start
        subprocess.run(["bash", "/work/.mock/scripts/cmd.sh", slug, "start"],
                      check=True, capture_output=True)
    time.sleep(2)

async def main():
    ensure_services()
    start_nginx()
    results = []
    errors = []
    try:
        async with async_playwright() as p:
            browser = await p.chromium.launch()
            ctx = await browser.new_context()
            page = await ctx.new_page()
            page.on("pageerror", lambda exc: errors.append(f"pageerror: {exc}"))
            page.on("console", lambda msg: errors.append(f"console.{msg.type}: {msg.text}") if msg.type in ("error",) else None)

            # === scheduler ===
            print("=== scheduler ===")
            try:
                await page.goto("http://127.0.0.1:8080/app/scheduler/", wait_until="networkidle", timeout=15000)
                title = await page.title()
                print(f"  Title: {title}")
                assert "计划任务" in title, f"Unexpected title: {title}"
                results.append(("scheduler UI loads", True))

                nav_count = await page.locator("nav button").count()
                print(f"  Nav buttons: {nav_count}")
                assert nav_count == 4
                results.append(("scheduler has 4 nav tabs", True))

                await page.locator('nav button:has-text("任务列表")').click()
                await page.wait_for_timeout(500)
                await page.locator('button:has-text("+ 新建任务")').click()
                await page.wait_for_timeout(500)
                modal_visible = await page.locator('#job-modal').is_visible()
                print(f"  Job modal visible: {modal_visible}")
                assert modal_visible
                results.append(("scheduler create job modal", True))

                await page.locator('#job-name').fill("playwright test")
                await page.locator('#job-spec').fill("@every 1m")
                await page.locator('#job-cmd').fill("echo PLAYWRIGHT_OK")
                async with page.expect_response(lambda r: "/api/jobs" in r.url, timeout=10000) as resp_info:
                    await page.locator('button:has-text("保存"):not(:has-text("设置"))').click()
                resp = await resp_info.value
                print(f"  POST /api/jobs status={resp.status}")
                assert resp.status == 201
                results.append(("scheduler create job submit", True))

                row_text = await page.locator('table tbody tr').first.text_content()
                print(f"  First row: {row_text[:80]}")
                assert "playwright test" in row_text
                results.append(("scheduler job appears in list", True))

                await page.locator('button:has-text("运行")').first.click()
                await page.wait_for_timeout(2500)
                await page.locator('nav button:has-text("执行历史")').click()
                await page.wait_for_timeout(1000)

                log_btn = page.locator('button:has-text("查看日志")').first
                if await log_btn.count() > 0:
                    await log_btn.click()
                    await page.wait_for_timeout(1500)
                    log_content = await page.locator('#log-content').text_content()
                    print(f"  Log (first 100): {repr(log_content[:100])}")
                    if "PLAYWRIGHT_OK" in (log_content or ""):
                        results.append(("scheduler run log shows output", True))
                    else:
                        results.append(("scheduler run log shows output", False))
                        print(f"    Expected 'PLAYWRIGHT_OK', got: {(log_content or '')[:200]}")
                else:
                    results.append(("scheduler run log shows output", False))
                    print("    No '查看日志' button found")
            except Exception as e:
                results.append(("scheduler flow", False))
                print(f"  ERROR: {e}")

            # === terminal ===
            print("\n=== terminal ===")
            try:
                await page.goto("http://127.0.0.1:8080/app/terminal/", wait_until="networkidle", timeout=15000)
                title = await page.title()
                print(f"  Title: {title}")
                assert "网页终端" in title
                results.append(("terminal UI loads", True))

                await page.wait_for_timeout(3000)
                has_xterm = await page.evaluate("typeof Terminal !== 'undefined'")
                print(f"  xterm.js loaded: {has_xterm}")
                if not has_xterm:
                    print("  (CDN may be unreachable; checking via API instead)")
                    results.append(("terminal xterm.js CDN", False))
                else:
                    results.append(("terminal xterm.js CDN", True))

                await page.wait_for_timeout(2500)
                tab_count = await page.locator('.tab').count()
                print(f"  Tabs: {tab_count}")
                assert tab_count >= 1
                results.append(("terminal session tab created", True))

                if has_xterm:
                    active_pane = page.locator('.term-pane.active .xterm').first
                    pane_count = await page.locator('.term-pane.active').count()
                    print(f"  Active panes: {pane_count}")
                    if pane_count == 0:
                        await page.locator('.tab').first.click()
                        await page.wait_for_timeout(500)
                    term = page.locator('.term-pane.active .xterm').first
                    await term.click(timeout=5000)
                    await page.wait_for_timeout(500)
                    await page.keyboard.type("echo BROWSER_TEST")
                    await page.keyboard.press("Enter")
                    await page.wait_for_timeout(2000)
                    term_text = await page.locator('.xterm-rows').first.text_content()
                    print(f"  Terminal content (first 250): {repr((term_text or '')[:250])}")
                    if "BROWSER_TEST" in (term_text or ""):
                        results.append(("terminal xterm.js receives input/output", True))
                    else:
                        results.append(("terminal xterm.js receives input/output", False))
            except Exception as e:
                results.append(("terminal flow", False))
                print(f"  ERROR: {e}")

            await browser.close()
    finally:
        stop_nginx()

    print("\n=== Browser E2E Results ===")
    passed = sum(1 for _, ok in results if ok)
    failed = sum(1 for _, ok in results if not ok)
    for name, ok in results:
        mark = "✓" if ok else "✗"
        print(f"  {mark} {name}")
    print(f"\nPASS: {passed}  FAIL: {failed}")

    if errors:
        print("\n=== Page errors (first 10) ===")
        for e in errors[:10]:
            print(f"  {e}")

    return 0 if failed == 0 else 1

sys.exit(asyncio.run(main()))
