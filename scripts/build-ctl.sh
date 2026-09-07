#!/usr/bin/env bash
# 构建 utsusemi-ctl 的四个 Android ABI（启用 CGO 原生对接 Android Bionic libc 及 DNS）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NDK="${NDK:-/data/.build-tools-cache/Android/ndk/28.2.13676358}"
BIN="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin"
OUT="$ROOT/build/ctl"

# 从 VERSION 文件或环境变量读取版本号（去除前导 v）
RAW_VER="${VERSION:-}"
if [ -z "$RAW_VER" ] && [ -f "$ROOT/VERSION" ]; then
  # shellcheck disable=SC1091
  source "$ROOT/VERSION"
  RAW_VER="${VERSION:-0.3.0}"
fi
CTL_VER="${RAW_VER#v}"

cd "$ROOT/ctl"

# 映射四个 Android ABI 到其对应的 NDK clang 及 Go 编译目标
declare -A ABIS=(
  [arm64-v8a]="aarch64-linux-android21-clang android arm64"
  [armeabi-v7a]="armv7a-linux-androideabi21-clang android arm 7"
  [x86_64]="x86_64-linux-android21-clang android amd64"
  [x86]="i686-linux-android21-clang android 386"
)

LDFLAGS="-s -w -X utsusemi/ctl/cmd.Version=$CTL_VER"

for abi in "${!ABIS[@]}"; do
  set -- ${ABIS[$abi]}
  clang_bin="$1"; goos="$2"; goarch="$3"
  goarm="${4:-}"
  dest="$OUT/$abi"
  mkdir -p "$dest"
  
  CC="$BIN/$clang_bin"
  if [ -n "$goarm" ]; then
    CGO_ENABLED=1 CC="$CC" GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$dest/utsusemi-ctl" .
  else
    CGO_ENABLED=1 CC="$CC" GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$dest/utsusemi-ctl" .
  fi
  file "$dest/utsusemi-ctl"
done
echo "done: $OUT"
