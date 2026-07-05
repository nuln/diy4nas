#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html><html><head><meta charset="UTF-8"><title>EasyTier</title></head>
<body style="font-family:sans-serif;background:#0b0f17;color:#e8edf5;padding:20px;margin:0">
<h1>EasyTier</h1>
<p>正在加载服务...如果长时间未响应，请在 fnOS 主界面中打开 EasyTier 管理页面。</p>
<p>Loading service... If this page persists, please open EasyTier from the main fnOS interface.</p>
<script>setTimeout(()=>location.href='/',2000)</script>
</body></html>
HTM