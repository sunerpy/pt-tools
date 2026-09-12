#!/usr/bin/env bash
# GoReleaser build post hook：对产物做 UPX 压缩。
#
# 为什么需要包装脚本而不是直接 `upx -9 {{ .Path }}`：
#   UPX 4.2.4 不支持 win64/arm64，直接调用会以
#     CantPackException: win64/arm64 is not yet supported
#   失败，从而让整个 release 在该 target 上中断（实测证实）。
#
# 迁移前的 Makefile 用 `upx -9 $file || echo "Failed to compress"` 容忍所有
# 失败，因此 windows/arm64 一直未被压缩且无人察觉。本脚本不恢复那种
# fail-open 行为：只对**已知且已记录**的不支持平台显式跳过，
# 其余任何 UPX 失败都保持硬失败。
#
# 用法（由 .goreleaser.yaml 传入模板变量）：
#   compress-binary.sh <binary-path> <goos> <goarch>
set -euo pipefail

if [ "$#" -ne 3 ]; then
	echo "usage: $0 <binary-path> <goos> <goarch>" >&2
	exit 1
fi

path=$1
os=$2
arch=$3

[ -f "$path" ] || {
	echo "::error title=UPX::binary not found: ${path}" >&2
	exit 1
}

# 已知不支持组合：UPX 至 4.2.4 仍未实现 win64/arm64。
if [ "$os" = "windows" ] && [ "$arch" = "arm64" ]; then
	echo "skip upx: ${os}/${arch} is not supported by UPX (CantPackException: win64/arm64)"
	exit 0
fi

command -v upx >/dev/null 2>&1 || {
	echo "::error title=UPX::upx is required but not installed" >&2
	exit 1
}

upx -9 "$path"
