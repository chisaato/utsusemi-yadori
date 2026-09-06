#!/usr/bin/env bash
# 校验 dist zip 结构完整（customize 引用的关键文件均存在且带 sha256）
set -euo pipefail
ZIP="${1:?usage: assert-zip.sh <zip>}"

need=(
  module.prop customize.sh service.sh action.sh uninstall.sh verify.sh
  webroot/index.html
  zygisk/arm64-v8a.so zygisk/armeabi-v7a.so zygisk/x86.so zygisk/x86_64.so
  lib/arm64-v8a/utsusemi-ctl lib/armeabi-v7a/utsusemi-ctl
  lib/x86/utsusemi-ctl lib/x86_64/utsusemi-ctl
)
for f in "${need[@]}"; do
  unzip -l "$ZIP" "$f" | grep -q "$f" || { echo "MISSING: $f"; exit 1; }
  unzip -l "$ZIP" "$f.sha256sum" | grep -q "$f.sha256sum" || { echo "MISSING: $f.sha256sum"; exit 1; }
done

# module.prop 占位符必须已渲染
if unzip -p "$ZIP" module.prop | grep -q '@VERSION'; then
  echo "module.prop not rendered"; exit 1
fi
# LF 检查
for f in module.prop customize.sh service.sh action.sh uninstall.sh verify.sh; do
  if unzip -p "$ZIP" "$f" | grep -q $'\r'; then
    echo "CRLF found in $f"; exit 1
  fi
done
echo "zip structure OK"
