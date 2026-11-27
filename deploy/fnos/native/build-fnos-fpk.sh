#!/usr/bin/env bash
# 打飞牛 fnOS 应用包（.fpk）。本地与发布流程（release.yml 的 fnos-fpk job）共用。
#
# 用法：build-fnos-fpk.sh <版本> <x86|arm> <pt-tools 二进制> <输出目录>
#   版本：例如 1.2.0、1.2.0-rc.1（写进 manifest，飞牛按它判断升级）
#   二进制：对应架构的 Linux pt-tools（x86 是 x86-64，arm 是 aarch64），按 ELF 头核对
# 输出：<输出目录>/pt-tools-fnos-<x86|arm>.fpk，并打印路径和 SHA-256。
#
# 包的结构照官方打包工具 fnpack 1.3.3 的产出：顶层 app.tgz、cmd/、config/、wizard/、manifest、ICON.PNG、ICON_256.PNG；
# app.tgz 里是 bin/pt-tools、ui/ 与 config/；manifest 末尾的 checksum 是 app.tgz 的 MD5。说明见同目录 README.md。
# 用固定的条目顺序、属主、时间（SOURCE_DATE_EPOCH，默认取最近一次提交的时间）和 gzip -n，同样的输入打出同样的字节。
set -euo pipefail

usage() {
	echo "usage: $0 <version> <x86|arm> <pt-tools binary> <output dir>" >&2
	exit 2
}
fail() {
	echo "build-fnos-fpk: $*" >&2
	exit 1
}

[ "$#" -eq 4 ] || usage
version=$1
platform=$2
binary=$3
outdir=$4

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
icons="$here/../apps/pt-tools/fnos"

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
	fail "version must look like 1.2.0 or 1.2.0-rc.1, got '$version'"
case "$platform" in
x86) want_machine=62 ;;   # EM_X86_64
arm) want_machine=183 ;;  # EM_AARCH64
*) fail "platform must be x86 or arm, got '$platform'" ;;
esac
[ -f "$binary" ] && [ -x "$binary" ] || fail "binary is missing or not executable: $binary"
[ "$(head -c 4 "$binary" | od -An -tx1 | tr -d ' \n')" = 7f454c46 ] || fail "not an ELF file: $binary"
[ "$(od -An -tu1 -j4 -N1 "$binary" | tr -d ' ')" = 2 ] || fail "not a 64-bit ELF file: $binary"
machine=$(od -An -tu2 -j18 -N2 --endian=little "$binary" | tr -d ' ')
[ "$machine" = "$want_machine" ] ||
	fail "ELF machine of $binary is $machine, expected $want_machine for $platform"

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
	SOURCE_DATE_EPOCH=$(git -C "$here" log -1 --format=%ct 2>/dev/null) ||
		fail "set SOURCE_DATE_EPOCH (no git history to take it from)"
fi
[[ "$SOURCE_DATE_EPOCH" =~ ^[0-9]+$ ]] || fail "SOURCE_DATE_EPOCH must be a number, got '$SOURCE_DATE_EPOCH'"

for f in "$icons/ICON.PNG" "$icons/ICON_256.PNG" "$icons/ui/images/64.png" "$icons/ui/images/256.png"; do
	[ -f "$f" ] || fail "missing icon $f"
done

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
pkg="$work/pkg"
app="$work/app"
mkdir -p "$pkg/cmd" "$pkg/config" "$pkg/wizard" "$app/bin" "$app/ui/images" "$app/config"

# app.tgz 的内容：程序、桌面入口与图标、权限与资源声明（fnpack 也把 config/ 放进 app.tgz）
install -m 0755 "$binary" "$app/bin/pt-tools"
install -m 0644 "$here/ui/config" "$app/ui/config"
install -m 0644 "$icons/ui/images/64.png" "$app/ui/images/icon_64.png"
install -m 0644 "$icons/ui/images/256.png" "$app/ui/images/icon_256.png"
install -m 0644 "$here/config/privilege" "$here/config/resource" "$app/config/"

for h in main install_init install_callback upgrade_init upgrade_callback \
	uninstall_init uninstall_callback config_init config_callback; do
	install -m 0755 "$here/cmd/$h" "$pkg/cmd/$h"
done
install -m 0644 "$here/config/privilege" "$here/config/resource" "$pkg/config/"
install -m 0644 "$here/wizard/install" "$here/wizard/uninstall" "$pkg/wizard/"
install -m 0644 "$icons/ICON.PNG" "$pkg/ICON.PNG"
install -m 0644 "$icons/ICON_256.PNG" "$pkg/ICON_256.PNG"

deterministic_tar() {
	# deterministic_tar <目录> <输出文件> <条目…>
	local dir=$1 out=$2
	shift 2
	tar --format=ustar --sort=name --owner=0 --group=0 --numeric-owner \
		--mtime="@$SOURCE_DATE_EPOCH" --mode=u+rwX,go+rX,go-w \
		-C "$dir" -cf - "$@" | gzip -n -9 >"$out"
}

deterministic_tar "$app" "$pkg/app.tgz" bin config ui
checksum=$(md5sum "$pkg/app.tgz" | cut -d' ' -f1)

sed -e "s/@VERSION@/$version/" -e "s/@PLATFORM@/$platform/" "$here/manifest.in" >"$pkg/manifest"
if grep -q '@[A-Z_]*@' "$pkg/manifest"; then
	fail "unreplaced placeholder in manifest: $(grep '@[A-Z_]*@' "$pkg/manifest")"
fi
printf 'checksum              = %s\n' "$checksum" >>"$pkg/manifest"

mkdir -p "$outdir"
out="$outdir/pt-tools-fnos-$platform.fpk"
deterministic_tar "$pkg" "$out.tmp" app.tgz cmd config wizard manifest ICON.PNG ICON_256.PNG
mv -f "$out.tmp" "$out"
echo "$out"
sha256sum "$out"
