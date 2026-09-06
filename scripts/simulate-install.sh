#!/usr/bin/env bash
# 本地模拟 Magisk 安装流程：解出 zip 内自带的 customize.sh/verify.sh 执行
# （真机执行的就是 zip 内副本），四种 $ARCH 全验，防 ABI 映射回归
# （arm→armeabi-v7a / arm64→arm64-v8a / x64→x86_64）。
# 用法: scripts/simulate-install.sh <zip路径>
# 注意: 本机 sha256sum 为 GNU coreutils，设备为 busybox；template/verify.sh
# 已用无短选项差异的显式比对写法，两侧行为等价。
set -euo pipefail

ZIP_ABS=$(readlink -f "$1")
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/utsusemi-sim.XXXXXX)
trap 'rm -rf "$WORK"' EXIT

unzip -o "$ZIP_ABS" customize.sh verify.sh -d "$WORK/zipscript" >/dev/null

sim() { # $1=ARCH $2=IS64BIT —— 子 shell 隔离，abort 即终止
  local ARCH="$1" IS64BIT="$2"
  local W="$WORK/$ARCH" TMPDIR="$WORK/$ARCH/tmp" MODPATH="$WORK/$ARCH/mod"
  mkdir -p "$TMPDIR" "$MODPATH"
  # Magisk 环境函数 stub（走文件注入，不依赖 bash export -f）
  cat >"$W/stubs.sh" <<EOS
ui_print() { echo "[$ARCH] \$*"; }
abort() { echo "[$ARCH][ABORT] \$*"; exit 1; }
set_perm() { :; }
set_perm_recursive() { :; }
EOS
  (
    export ARCH IS64BIT TMPDIR MODPATH ZIPFILE="$ZIP_ABS"
    export UTSUSEMI_DATA_DIR="$W/data" UTSUSEMI_STAGE_DIR="$W/stage"
    # 真机 Magisk 安装 shell 无 -e/-u/pipefail，还原同等宽松度再执行安装脚本
    set +euo pipefail
    . "$W/stubs.sh"
    . "$WORK/zipscript/customize.sh"
  ) || { echo "[$ARCH] 安装模拟失败"; return 1; }

  # 断言: ctl 必在且可执行；zygisk so 数量 32 位=1、64 位=2（补装 32 位）
  local want_so=1; [ "$IS64BIT" = true ] && want_so=2
  local got_so
  [ -x "$MODPATH/bin/utsusemi-ctl" ] || { echo "[$ARCH] 缺 bin/utsusemi-ctl"; return 1; }
  got_so=$(find "$MODPATH/zygisk" -name '*.so' | wc -l)
  [ "$got_so" -eq "$want_so" ] || { echo "[$ARCH] zygisk so 数量 $got_so != $want_so"; return 1; }
  [ -f "$MODPATH/webroot/index.html" ] || { echo "[$ARCH] 缺 webroot/index.html"; return 1; }
  echo "[$ARCH] OK (ctl + ${got_so} so + webroot)"
}

fail=0
sim arm false   || fail=1
sim arm64 true  || fail=1
sim x86 false   || fail=1
sim x64 true    || fail=1
[ "$fail" -eq 0 ] && echo "PASS: 四 ABI 安装模拟全部通过" || { echo "FAIL"; exit 1; }
