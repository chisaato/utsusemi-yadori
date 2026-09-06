SKIPUNZIP=1

MODULE_ID=utsusemi
# 默认值与硬编码等价；带前缀 env 仅供 scripts/simulate-install.sh 本地模拟注入
DATA_DIR="${UTSUSEMI_DATA_DIR:-/data/adb/utsusemi}"
STAGE_DIR="${UTSUSEMI_STAGE_DIR:-/data/local/tmp/utsusemi}"

if [ "$ARCH" != "arm" ] && [ "$ARCH" != "arm64" ] && [ "$ARCH" != "x86" ] && [ "$ARCH" != "x64" ]; then
  abort "! Unsupported platform: $ARCH"
fi
ui_print "- Device platform: $ARCH"

ui_print "- Extracting verify.sh"
unzip -o "$ZIPFILE" 'verify.sh' -d "$TMPDIR" >&2
if [ ! -f "$TMPDIR/verify.sh" ]; then
  abort "! Unable to extract verify.sh (zip corrupted?)"
fi
. "$TMPDIR/verify.sh"

ui_print "- Installing module files"
extract "$ZIPFILE" 'module.prop' "$MODPATH"
extract "$ZIPFILE" 'service.sh' "$MODPATH"
extract "$ZIPFILE" 'action.sh' "$MODPATH"
extract "$ZIPFILE" 'uninstall.sh' "$MODPATH"

# ctl 与 zygisk so 按设备 ABI 选装（zip 内 lib/<abi>/utsusemi-ctl、zygisk/<abi>.so）
# Magisk 的 $ARCH ∈ {arm, arm64, x86, x64}，需映射到 zip 内 NDK ABI 目录名
MAGISK_ABI="$ARCH"
[ "$ARCH" = "arm" ] && MAGISK_ABI=armeabi-v7a
[ "$ARCH" = "arm64" ] && MAGISK_ABI=arm64-v8a
[ "$ARCH" = "x64" ] && MAGISK_ABI=x86_64

mkdir -p "$MODPATH/bin" "$MODPATH/zygisk"
extract "$ZIPFILE" "lib/$MAGISK_ABI/utsusemi-ctl" "$MODPATH/bin" true

extract "$ZIPFILE" "zygisk/$MAGISK_ABI.so" "$MODPATH/zygisk" true

# 64 位设备补装 32 位 zygisk so（兼容混合架构应用进程）
if [ "$IS64BIT" = true ]; then
  ABI32=armeabi-v7a
  if [ "$ARCH" = "x64" ]; then
    ABI32=x86
  fi
  extract "$ZIPFILE" "zygisk/$ABI32.so" "$MODPATH/zygisk" true
fi

# webroot: 先清后装，防升级时旧 contenthash 文件残留
# （KSU 系自动处理 webroot 权限/SELinux 上下文，不手动 set_perm）
ui_print "- Installing webroot"
rm -rf "$MODPATH/webroot"
unzip -o "$ZIPFILE" 'webroot/*' -d "$MODPATH" >&2

# 权限: 先整体递归 0644，再对可执行件显式 0755
set_perm_recursive "$MODPATH" 0 0 0755 0644
set_perm "$MODPATH/bin/utsusemi-ctl" 0 0 0755 u:object_r:system_file:s0
set_perm_recursive "$MODPATH/zygisk" 0 0 0755 0755 u:object_r:system_file:s0

# 数据目录: 仅确保存在，绝不覆盖用户 settings/manifest/frida-bin
ui_print "- Preparing data dirs"
mkdir -p "$DATA_DIR" "$STAGE_DIR" "$MODPATH/logs"
chmod 0755 "$DATA_DIR" "$STAGE_DIR"

ui_print "- Installed. Manage via WebUI, Action button or utsusemi-ctl"
