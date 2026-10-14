#!/usr/bin/env bash
# 由 pt-tools 的版本推导手机 App 的构建号（Android 的 versionCode 由它而来）。
#
# App 跟 pt-tools 用同一个版本、同一个 Release 发布（release.yml 的 mobile job）。Android 只看 versionCode
# 决定能不能覆盖安装，所以它必须随版本单调变大，而且同一个版本重建时不变。映射：
#
#   X.Y.Z-rc.N  ->  X*1000000 + Y*10000 + Z*100 + N    （N ∈ 1..98）
#   X.Y.Z       ->  X*1000000 + Y*10000 + Z*100 + 99
#
# 例：1.0.0-rc.28 -> 1000028，1.0.0 -> 1000099，1.0.1-rc.1 -> 1000101。Y、Z 不能超过 99，X 不能超过 999。
# 按 ABI 拆包时 Flutter 再加上 ABI×1000（armeabi-v7a 1、arm64-v8a 2、x86_64 4），同一个 ABI 的包仍按版本递增；
# 单独发布过的 mobile-v1.0.0-rc.1 构建号是 1（arm64 的 versionCode 2001），比这里的任何值都小，可以直接覆盖安装。
#
# 用法：
#   mobile-build-number.sh 1.0.0-rc.28    # -> 1000028
#   mobile-build-number.sh v1.0.0         # 带 v 的 tag 也认 -> 1000099
#
# 失败即非零退出并打印诊断（GitHub Actions 的 ::error:: 注解）。
set -euo pipefail

die() {
	echo "::error title=App build number::$*" >&2
	exit 1
}

version="${1:-}"
[ -n "$version" ] || die "缺少版本号"
version="${version#v}"

# 三段都不许有前导零（否则 1.01.0 和 1.1.0 会得到同一个构建号）
num='(0|[1-9][0-9]*)'
if [[ ! "$version" =~ ^${num}\.${num}\.${num}(-rc\.([1-9][0-9]*))?$ ]]; then
	die "版本必须是 X.Y.Z 或 X.Y.Z-rc.N，实际 '${1:-}'"
fi
major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}
patch=${BASH_REMATCH[3]}
rc=${BASH_REMATCH[5]}

[ "$major" -le 999 ] || die "主版本 ${major} 超出 0..999"
[ "$minor" -le 99 ] || die "次版本 ${minor} 超出 0..99"
[ "$patch" -le 99 ] || die "修订号 ${patch} 超出 0..99"
if [ -n "$rc" ]; then
	[ "$rc" -le 98 ] || die "RC 序号 ${rc} 超出 1..98"
	last=$rc
else
	last=99
fi

echo $((major * 1000000 + minor * 10000 + patch * 100 + last))
