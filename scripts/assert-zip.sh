#!/usr/bin/env bash
# 校验 dist zip 结构完整（customize 引用的关键文件均存在且带 sha256）
# 行内注释: 管道 grep 一律全量读取(>/dev/null)，禁止 -q —— pipefail 下 grep -q
# 提前退出会让上游 unzip 收 SIGPIPE(141) 产生竞态误报
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
  unzip -l "$ZIP" "$f" | grep "$f" >/dev/null || { echo "MISSING: $f"; exit 1; }
  unzip -l "$ZIP" "$f.sha256sum" | grep "$f.sha256sum" >/dev/null || { echo "MISSING: $f.sha256sum"; exit 1; }
done

# webroot 必须是真前端产物：有 assets 且 index.html 引用相对路径 hash 资源（非占位页）
if ! unzip -l "$ZIP" 'webroot/assets/*' | grep 'assets/' >/dev/null; then
  echo "webroot/assets empty (placeholder build?)"; exit 1
fi
if ! unzip -p "$ZIP" webroot/index.html | grep '\./assets/' >/dev/null; then
  echo "webroot/index.html does not reference ./assets/"; exit 1
fi

# module.prop 占位符必须已渲染
if unzip -p "$ZIP" module.prop | grep '@VERSION' >/dev/null; then
  echo "module.prop not rendered"; exit 1
fi
# LF 检查
for f in module.prop customize.sh service.sh action.sh uninstall.sh verify.sh; do
  if unzip -p "$ZIP" "$f" | grep $'\r' >/dev/null; then
    echo "CRLF found in $f"; exit 1
  fi
done
echo "zip structure OK"
