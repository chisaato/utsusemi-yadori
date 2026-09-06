#!/system/bin/sh
# 管理器 Action 按钮: 切换 frida-server 启停
MODDIR=${0%/*}
CTL="$MODDIR/bin/utsusemi-ctl"
exec 2>"$MODDIR/logs/action.log"
set -x

if "$CTL" --json server status 2>/dev/null | grep -q '"running":true'; then
  "$CTL" server stop
else
  "$CTL" server start
fi
"$CTL" --json server status 2>/dev/null || true
