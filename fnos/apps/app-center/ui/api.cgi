#!/bin/sh
echo "Content-Type: application/json; charset=utf-8"
echo ""

DATA="/var/apps/app-center/data"
SOURCES_FILE="$DATA/sources.json"
mkdir -p "$DATA/fpk"

ACTION="${QUERY_STRING#action=}"
ACTION="${ACTION%%&*}"
SUDO="/usr/bin/sudo"
APPCFG="/usr/sbin/appcfg"

read_post() {
  read -r LINE
  echo "$LINE"
}

case "$ACTION" in
  catalog)
    FIRST=1
    echo '{"apps":['
    
    # 1. 本地 fpk 目录：扫描 /var/apps/app-center/data/fpk/ 中的 .fpk 文件
    for fpk in "$DATA/fpk/"*.fpk; do
      [ -f "$fpk" ] || continue
      SLUG=$(basename "$fpk" .fpk)
      SIZE=$(stat -f%z "$fpk" 2>/dev/null || stat -c%s "$fpk" 2>/dev/null || echo 0)
      [ "$FIRST" -eq 1 ] && FIRST=0 || echo ","
      echo "{\"slug\":\"$SLUG\",\"display_name\":\"$SLUG\",\"version\":\"local\",\"description\":\"本地 FPK 文件\",\"app_type\":\"native\",\"service_port\":0,\"category\":\"local\",\"fpk_file\":\"$fpk\",\"fpk_size\":$SIZE,\"source\":\"local\",\"can_install\":true}"
    done
    
    # 2. 第三方源
    SOURCES=""
    [ -f "$SOURCES_FILE" ] && SOURCES=$(cat "$SOURCES_FILE")
    
    echo "$SOURCES" | grep -o '"url":"[^"]*"' | cut -d'"' -f4 | while read -r BASE_URL; do
      # 尝试读取 fnpack.json
      CAT_URL="${BASE_URL%/}/fnpack.json"
      DATA=$(curl -s --max-time 10 "$CAT_URL" 2>/dev/null)
      
      # 如果 fnpack.json 不存在，尝试 apps.json
      if [ -z "$DATA" ]; then
        CAT_URL="${BASE_URL%/}/apps.json"
        DATA=$(curl -s --max-time 10 "$CAT_URL" 2>/dev/null)
      fi
      
      [ -z "$DATA" ] && continue
      
      # 提取 apps 数组
      APPS=$(echo "$DATA" | grep -o '"apps":\[.*\]' | cut -d: -f2- | sed 's/^\[//;s/\]$//')
      [ -z "$APPS" ] && continue
      
      # 为每个 app 添加 source/base_url/fpk_url
      echo "$APPS" | sed 's/},{/}\n{/g' | while read -r APP; do
        SLUG=$(echo "$APP" | grep -o '"slug":"[^"]*"' | cut -d'"' -f4)
        [ -z "$SLUG" ] && continue
        
        # 添加源信息
        APP="${APP%\}}"",\"source\":\"$BASE_URL\""
        
        # 添加 fpk_url（策略：base_url + dist/slug.fpk）
        APP="$APP,\"fpk_url\":\"${BASE_URL%/}/dist/${SLUG}.fpk\""
        
        # 添加 icon_url
        APP="$APP,\"icon_url\":\"${BASE_URL%/}/apps/${SLUG}/ICON_256.PNG\""
        
        # 添加下载优先级 fpk → 远程安装
        APP="$APP,\"can_install\":true"
        APP="$APP}"
        
        [ "$FIRST" -eq 1 ] && FIRST=0 || echo ","
        echo "$APP"
      done
    done
    
    echo ']}'
    ;;
    
  sources)
    if [ "$REQUEST_METHOD" = "POST" ]; then
      POST=$(read_post)
      echo "$POST" > "$SOURCES_FILE"
      echo '{"status":"saved"}'
    else
      [ -f "$SOURCES_FILE" ] && cat "$SOURCES_FILE" || echo '[]'
    fi
    ;;
    
  install)
    POST=$(read_post)
    SLUG=$(echo "$POST" | grep -o 'slug=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    FPK_URL=$(echo "$POST" | grep -o 'fpk_url=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    
    if [ -n "$FPK_URL" ]; then
      FPK="$DATA/fpk/${SLUG}.fpk"
      echo "  downloading from $FPK_URL..." >&2
      curl -fsSL -o "$FPK" "$FPK_URL" 2>/dev/null || { echo '{"error":"download failed"}'; exit 0; }
    fi
    
    FPK="$DATA/fpk/${SLUG}.fpk"
    if [ ! -f "$FPK" ]; then echo '{"error":"FPK not found"}'; exit 0; fi
    
    $SUDO $APPCFG install "$FPK" >/dev/null 2>&1 &
    echo '{"status":"installing"}'
    ;;
    
  uninstall)
    POST=$(read_post)
    SLUG=$(echo "$POST" | grep -o 'slug=[^&]*' | cut -d= -f2- | sed 's/%20/ /g')
    $SUDO $APPCFG uninstall "$SLUG" >/dev/null 2>&1 &
    echo '{"status":"uninstalling"}'
    ;;
    
  status)
    SLUG="${QUERY_STRING#*slug=}"; SLUG="${SLUG%%&*}"
    if [ -d "/var/apps/$SLUG" ]; then
      /var/apps/$SLUG/cmd/main status >/dev/null 2>&1
      [ $? -eq 0 ] && echo '{"installed":true,"running":true}' || echo '{"installed":true,"running":false}'
    else
      echo '{"installed":false,"running":false}'
    fi
    ;;
    
  *) echo '{"error":"unknown action"}' ;;
esac
