#!/system/bin/sh
# 卸载: 清理发布区；控制区数据（已下载二进制/配置）默认保留供重装恢复
STAGE_DIR=/data/local/tmp/utsusemi
rm -rf "$STAGE_DIR"
echo "utsusemi: stage cleaned; data kept at /data/adb/utsusemi"
