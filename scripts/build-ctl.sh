#!/usr/bin/env bash
# 构建 utsusemi-ctl 的四个 Android ABI
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/build/ctl"
cd "$ROOT/ctl"

# 仅 android/arm64 支持 CGO_ENABLED=0 免 cgo 构建（主目标，自带 Android DNS 行为）
# 其余 ABI 用 GOOS=linux 纯静态（可在 Android 运行；纯 Go 解析器在无 resolv.conf 的
# 设备上可能无法解析域名——已知限制，可用镜像源 IP 或 USB 导入绕过）
declare -A ABIS=( [arm64-v8a]="android arm64" [armeabi-v7a]="linux arm" [x86_64]="linux amd64" [x86]="linux 386" )
for abi in "${!ABIS[@]}"; do
  set -- ${ABIS[$abi]}
  goos="$1"; goarch="$2"
  dest="$OUT/$abi"
  mkdir -p "$dest"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "-s -w" -o "$dest/utsusemi-ctl" .
  file "$dest/utsusemi-ctl"
done
echo "done: $OUT"
