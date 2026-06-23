#!/bin/sh
echo "Content-Type: text/html; charset=utf-8"
echo ""
cat << 'HTM'
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>事件钩子</title>
<script>
window.location.href = "api.cgi?path=/";
</script>
</head>
<body>
<p>加载中...</p>
</body>
</html>
HTM
