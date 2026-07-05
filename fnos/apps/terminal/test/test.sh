#!/bin/bash
# Terminal 页面响应式 CSS 测试
# 校验 index.html 中的 CSS 和 HTML 结构是否正确
set -e
PASS=0
FAIL=0

log_pass() { PASS=$((PASS+1)); echo "  [PASS] $1"; }
log_fail() { FAIL=$((FAIL+1)); echo "  [FAIL] $1"; }

HTML="../www/ui/index.html"
[ -f "$HTML" ] || { echo "❌ 找不到 $HTML"; exit 1; }

echo "=== Terminal 页面结构测试 ==="
echo ""

# 1. HTML 基本结构
echo "[1] HTML 基本结构..."
if grep -q '<!DOCTYPE html>' "$HTML" && grep -q '</html>' "$HTML"; then
	log_pass "DOCTYPE + html 标签完整"
else
	log_fail "缺少 DOCTYPE 或 /html"
fi

# 2. 无重复 body/html 闭合
echo "[2] 闭合标签..."
BODY_COUNT=$(grep -c '</body>' "$HTML" 2>/dev/null || echo 0)
HTML_COUNT=$(grep -c '</html>' "$HTML" 2>/dev/null || echo 0)
if [ "$BODY_COUNT" -eq 1 ] && [ "$HTML_COUNT" -eq 1 ]; then
	log_pass "body/html 闭合各 1 次（无重复）"
else
	log_fail "body=$BODY_COUNT, html=$HTML_COUNT（期望各 1）"
fi

# 3. viewport meta
echo "[3] viewport meta..."
if grep -q 'name="viewport"' "$HTML"; then
	log_pass "viewport meta 存在"
else
	log_fail "缺少 viewport meta"
fi

# 4. 响应式媒体查询块
echo "[4] 响应式媒体查询..."
if grep -q '@media.*max-width' "$HTML"; then
	log_pass "媒体查询 @media (max-width: ...) 存在"
else
	log_fail "缺少响应式媒体查询"
fi

# 5. 关键响应式规则
echo "[5] 关键响应式规则..."
RULES=(
	".toolbar.*flex-wrap"
	".sidebar.*width: 100%"
	".toast.*bottom: 50px"
	".statusbar.*flex-wrap"
	".tab.*min-width: 80px"
)
MISSING=0
for R in "${RULES[@]}"; do
	if grep -q "$R" "$HTML"; then
		log_pass "规则匹配: $R"
	else
		log_fail "缺少规则: $R"
		MISSING=$((MISSING+1))
	fi
done

# 6. .user-error 样式
echo "[6] .user-error 样式..."
if grep -q '\.user-error' "$HTML" && grep -q 'position: absolute' "$HTML" && grep -q 'inset: 0' "$HTML"; then
	log_pass ".user-error 有 position:absolute + inset:0"
else
	log_fail ".user-error 缺少定位样式"
fi

# 7. layout flex 布局
echo "[7] flex 布局..."
if grep -q '\.layout.*flex-direction: column' "$HTML"; then
	log_pass ".layout 使用 flex column"
else
	log_fail ".layout 缺少 flex column"
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
