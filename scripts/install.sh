#!/bin/sh
# pt-tools 安装脚本（POSIX sh）
#
# 依据 CI/Release 迁移设计 v7 §3.1 与 github-project-scaffold
# references/install-script.md 的 checksum 契约：
#   1. 解析 TOOL_VERSION，或回退到最新已发布 Release；
#   2. 选择本平台归档；
#   3. 从**同一 tag** 下载归档与 checksums.txt；
#   4. 在 checksum 文件中定位该归档的精确条目；
#   5. 本地计算 SHA-256；
#   6. 摘要不符则拒绝解压（绝不降级为警告）。
#
# 本脚本为项目自有实现（已在 .github/scaffold.json 的 drift_allow 登记）：
# pt-tools 采用**无版本号**资产命名以保住 releases/latest/download/... 稳定
# 链接，与 profile 自带安装脚本假定的 BINARY_VERSION_OS_ARCH 命名不兼容。
#
# 环境变量：
#   TOOL_VERSION      要安装的版本（例如 v0.48.0 或 0.48.0）；缺省取最新 Release
#   TOOL_INSTALL_DIR  安装目录（缺省 $HOME/.local/bin）
#
# 支持矩阵：linux/amd64、linux/arm64。**不提供 macOS 产物**；
# Windows 请使用 scripts/install.ps1。
set -eu

OWNER=sunerpy
REPO=pt-tools
BINARY=pt-tools

INSTALL_DIR="${TOOL_INSTALL_DIR:-${HOME}/.local/bin}"

die() {
	echo "error: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

need curl
need tar
need mkdir
need install

# ---- SHA-256 计算器 ----
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		die "neither sha256sum nor shasum is available; cannot verify integrity"
	fi
}

# ---- 平台探测 ----
os=$(uname -s)
arch=$(uname -m)

case "$os" in
Linux) os_name=linux ;;
Darwin) die "pt-tools 不提供 macOS 产物；请使用 Docker 镜像 sunerpy/pt-tools" ;;
MINGW* | MSYS* | CYGWIN*) die "Windows 请使用 scripts/install.ps1" ;;
*) die "unsupported operating system: ${os}" ;;
esac

case "$arch" in
x86_64 | amd64) arch_name=amd64 ;;
aarch64 | arm64) arch_name=arm64 ;;
*) die "unsupported architecture: ${arch}" ;;
esac

asset="${BINARY}-${os_name}-${arch_name}.tar.gz"

# ---- 版本解析 ----
if [ -n "${TOOL_VERSION:-}" ]; then
	case "$TOOL_VERSION" in
	v*) tag="$TOOL_VERSION" ;;
	*) tag="v${TOOL_VERSION}" ;;
	esac
else
	echo "Resolving the latest release..."
	tag=$(curl -fsSL "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
		| sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| head -n 1)
	[ -n "$tag" ] || die "could not resolve the latest release tag"
fi

echo "Installing ${BINARY} ${tag} (${os_name}/${arch_name}) into ${INSTALL_DIR}"

base="https://github.com/${OWNER}/${REPO}/releases/download/${tag}"

tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT HUP TERM

# ---- 从同一 tag 下载归档与校验和 ----
echo "Downloading ${asset}..."
curl -fsSL -o "${tmp}/${asset}" "${base}/${asset}" \
	|| die "download failed: ${base}/${asset}"

echo "Downloading checksums.txt..."
curl -fsSL -o "${tmp}/checksums.txt" "${base}/checksums.txt" \
	|| die "download failed: ${base}/checksums.txt (该版本可能早于 checksums 契约)"

# ---- 定位精确条目并校验 ----
# checksums.txt 的每行形如 "<64 hex>  <basename>"，名称为裸 basename。
expected=$(awk -v want="$asset" '
	{
		name = $2
		sub(/^\*/, "", name)
		if (name == want) { print $1; found = 1; exit }
	}
	END { if (!found) exit 1 }
' "${tmp}/checksums.txt") || die "checksums.txt 中没有 ${asset} 的条目；拒绝安装"

case "$expected" in
[0-9a-f]*) : ;;
*) die "checksums.txt 中 ${asset} 的摘要格式非法: ${expected}" ;;
esac

actual=$(sha256_of "${tmp}/${asset}")

if [ "$expected" != "$actual" ]; then
	echo "error: 校验和不匹配，拒绝解压" >&2
	echo "  asset:    ${asset}" >&2
	echo "  expected: ${expected}" >&2
	echo "  actual:   ${actual}" >&2
	exit 1
fi
echo "Checksum OK (${actual})"

# ---- 解压并安装 ----
tar -xzf "${tmp}/${asset}" -C "$tmp"
[ -f "${tmp}/${BINARY}" ] || die "归档内未找到 ${BINARY}"

mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "Installed: ${INSTALL_DIR}/${BINARY}"
case ":${PATH}:" in
*":${INSTALL_DIR}:"*) ;;
*) echo "提示：${INSTALL_DIR} 不在 PATH 中，请将其加入 shell 配置。" ;;
esac

"${INSTALL_DIR}/${BINARY}" version || true
