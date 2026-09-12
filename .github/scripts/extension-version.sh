#!/usr/bin/env bash
# 由 release tag 推导浏览器扩展 manifest 的版本号。
#
# 依据 CI/Release 迁移设计 v7 §3.3：Chrome/Edge manifest 的 version 必须是
# 1..4 段点分整数、每段 0..65535，逐段自左数值比较，因此 SemVer 的预发布
# 后缀不能直接使用。四段单调映射：
#
#   v0.48.0-rc.N  ->  0.48.0.N      （N ∈ 1..65534）
#   v0.48.0       ->  0.48.0.65535
#
# 单调性：0.48.0.1 < … < 0.48.0.65534 < 0.48.0.65535 < 0.49.0.1，
# 因此安装过任意 RC 的用户都能正常升级到稳定版。
#
# 用法：
#   extension-version.sh v0.48.0-rc.3   # -> 0.48.0.3
#   TAG_NAME=v0.48.0 extension-version.sh
#
# 失败即非零退出并打印诊断（GitHub Actions 的 ::error:: 注解）。
set -euo pipefail

die() {
	echo "::error title=Extension version::$*" >&2
	exit 1
}

tag="${1:-${TAG_NAME:-}}"
[ -n "$tag" ] || die "缺少 tag：请传参或设置 TAG_NAME"

case "$tag" in
v*) ;;
*) die "tag 必须以 v 开头，实际 '${tag}'" ;;
esac

core="${tag#v}"
base="${core%%-*}"

if [ "$core" = "$base" ]; then
	# 稳定版哨兵。注意：这会耗尽同一 SemVer 下的 post-stable 商店 revision
	# 空间——商店专用 respin 必须提升产品 patch/minor，不能靠第四段。
	fourth=65535
else
	suffix="${core#*-}"
	case "$suffix" in
	rc.*) fourth="${suffix#rc.}" ;;
	*) die "仅支持 rc.N 预发布后缀，实际 '${suffix}'" ;;
	esac
	case "$fourth" in
	'' | *[!0-9]*) die "RC 序号不是整数：'${fourth}'" ;;
	esac
	# 去掉前导零后再比较，避免 08 之类被当作八进制。
	fourth=$((10#$fourth))
	if [ "$fourth" -lt 1 ] || [ "$fourth" -gt 65534 ]; then
		die "RC 序号 ${fourth} 超出 1..65534"
	fi
fi

# base 必须恰为三段整数。
saved_ifs="$IFS"
IFS='.'
# shellcheck disable=SC2086
set -- $base
IFS="$saved_ifs"
[ "$#" -eq 3 ] || die "版本主体必须是三段，实际 '${base}'"

for seg in "$1" "$2" "$3"; do
	case "$seg" in
	'' | *[!0-9]*) die "版本段不是整数：'${seg}'" ;;
	esac
	if [ "$((10#$seg))" -gt 65535 ]; then
		die "版本段 ${seg} 超出 0..65535"
	fi
done

printf '%s.%s.%s.%s\n' "$((10#$1))" "$((10#$2))" "$((10#$3))" "$fourth"
