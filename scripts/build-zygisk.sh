#!/usr/bin/env bash
# 用 NDK + CMake 构建四个 ABI 的 zygisk so
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NDK="${NDK:-/data/.build-tools-cache/Android/ndk/28.2.13676358}"
OUT="$ROOT/build/zygisk"
TOOLCHAIN="$NDK/build/cmake/android.toolchain.cmake"

for abi in arm64-v8a armeabi-v7a x86 x86_64; do
  build="$ROOT/build/native/$abi"
  mkdir -p "$build"
  cmake -G Ninja -S "$ROOT/native" -B "$build" \
    -DCMAKE_TOOLCHAIN_FILE="$TOOLCHAIN" \
    -DANDROID_ABI="$abi" \
    -DANDROID_PLATFORM=android-21 \
    -DANDROID_STL=c++_static \
    -DCMAKE_BUILD_TYPE=Release >/dev/null
  cmake --build "$build" -j >/dev/null
  mkdir -p "$OUT"
  # 行内注释: llvm-strip 去符号缩小体积（保留动态符号）
  "$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-strip" --strip-unneeded \
    "$build/libutsusemi_zygisk.so" -o "$OUT/$abi.so"
  file "$OUT/$abi.so"
done
echo "done: $OUT"
