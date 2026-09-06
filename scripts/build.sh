#!/usr/bin/env bash
# 一键打包: webui(bun/vite) + ctl(go) + zygisk(ndk/cmake) + template → dist/utsusemi-<version>.zip
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-v0.1.0}"
VERSION_CODE="${VERSION_CODE:-1}"
cd "$ROOT"

# —— webui：构建前端并双写（template/webroot 供 KSU WebUI；ctl/webui/dist 供 go:embed）——
echo "== webui =="
command -v bun >/dev/null || { echo "ERROR: bun not found (frontend is required)"; exit 1; }
( cd web && bun install && bun run build )
rm -rf template/webroot
mkdir -p template/webroot ctl/webui/dist
cp -r web/dist/. template/webroot/
cp -r web/dist/. ctl/webui/dist/

echo "== ctl =="
scripts/build-ctl.sh
echo "== zygisk =="
scripts/build-zygisk.sh

STAGE="$ROOT/build/package"
DIST="$ROOT/dist"
rm -rf "$STAGE"
mkdir -p "$STAGE/lib" "$STAGE/zygisk" "$DIST"

cp -r template/. "$STAGE/"
sed -i "s/@VERSION@/$VERSION/; s/@VERSION_CODE@/$VERSION_CODE/" "$STAGE/module.prop"

for abi in arm64-v8a armeabi-v7a x86 x86_64; do
  mkdir -p "$STAGE/lib/$abi"
  cp "build/ctl/$abi/utsusemi-ctl" "$STAGE/lib/$abi/utsusemi-ctl"
  cp "build/zygisk/$abi.so" "$STAGE/zygisk/$abi.so"
done

# 行内注释: LF 规范化全部文本脚本，杜绝 CRLF 进入模块
find "$STAGE" -name '*.sh' -type f -exec perl -pi -e 's/\r$//' {} +
find "$STAGE" -name 'module.prop' -type f -exec perl -pi -e 's/\r$//' {} +

cd "$STAGE"
# 每个文件生成配套 sha256sum 供 customize.sh 的 verify extract 校验
find . -type f ! -name '*.sha256sum' -print0 | while IFS= read -r -d '' f; do
  sum=$(sha256sum "$f" | cut -d' ' -f1)
  echo "$sum" > "$f.sha256sum"
done

ZIP="$DIST/utsusemi-$VERSION.zip"
rm -f "$ZIP"
zip -r -X "$ZIP" . >/dev/null
echo "built: $ZIP"
unzip -l "$ZIP" | tail -3
