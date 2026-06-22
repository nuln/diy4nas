#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html><html><head><meta charset="UTF-8"><meta http-equiv="refresh" content="0;url=/"><title>计划任务</title></head>
<body style="font-family:sans-serif;background:#f5f6fa;color:#1f2329;padding:20px;margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh">
<div style="text-align:center">
<h1>计划任务</h1>
<p>正在加载管理界面...</p>
<p style="color:#86909c;font-size:13px">如长时间未响应，请检查 Go 服务是否运行（<code>cmd/main status</code>）</p>
</div>
</body></html>
HTM
