#!/system/bin/sh
# late_start service 模式执行；ctl 内部判断 autostart 并同步 gadget 发布区
MODDIR=${0%/*}
mkdir -p "$MODDIR/logs"
exec 2>"$MODDIR/logs/service.log"
set -x

"$MODDIR/bin/utsusemi-ctl" boot >>"$MODDIR/logs/service.log" 2>&1 &
