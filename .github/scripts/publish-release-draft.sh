#!/usr/bin/env bash
# Verify the remote assets of a DRAFT GitHub Release and, on request, publish it.
#
# Usage:
#   publish-release-draft.sh (--verify-only | --publish) [--checksums FILE]
#       [--asset PATH]... [--expect-name NAME]... [--exact]
#
# Environment: GH_TOKEN (contents: write; drafts are invisible otherwise), REPO, TAG
#
# Verification (every listed expectation must hold, otherwise exit 1 and the
# draft is left untouched):
#   --checksums FILE    sha256sum-format lines; every named asset must exist remotely
#                       exactly once with digest sha256:<hex>
#   --asset PATH        the remote asset named basename(PATH) must have the local
#                       byte size and SHA-256 digest (repeatable)
#   --expect-name NAME  the remote asset must exist exactly once (repeatable)
#   --exact             the remote asset count must equal the expected set
#
# Draft-safe lookup: the release is resolved with `gh release view`, which falls
# back to GraphQL for drafts. Never use the REST path releases/tags/TAG for a
# draft; it returns only published releases (404 for the draft).
#
# --publish runs `gh release edit --draft=false`, adds `--latest` only when the
# release is not a prerelease (`--latest=false` otherwise), then re-reads the
# release and asserts draft == false.
#
# Immutable releases: `gh release upload --clobber` re-uploads work only while
# the release is still a draft; after the flip assets and the tag are locked.
#
# Sources:
#   https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/release/shared/fetch.go
#   https://docs.github.com/en/rest/releases/releases?apiVersion=2022-11-28#get-a-release-by-tag-name
#   https://docs.github.com/en/rest/releases/releases#update-a-release
#   https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases
set -euo pipefail

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

fail() {
	echo "::error title=Publication::$*" >&2
	exit 1
}

usage() {
	sed -n '2,34p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

action=
checksums=
exact=0
assets=()
expect_names=()
while (($# > 0)); do
	case "$1" in
	--verify-only | --publish)
		[[ -z "$action" ]] || fail "choose exactly one of --verify-only or --publish"
		action=${1#--}
		shift
		;;
	--checksums)
		[[ $# -ge 2 ]] || fail "--checksums requires a file"
		[[ -z "$checksums" ]] || fail "--checksums may be given once"
		checksums=$2
		shift 2
		;;
	--asset)
		[[ $# -ge 2 ]] || fail "--asset requires a path"
		assets+=("$2")
		shift 2
		;;
	--expect-name)
		[[ $# -ge 2 ]] || fail "--expect-name requires a name"
		expect_names+=("$2")
		shift 2
		;;
	--exact)
		exact=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "unknown argument: $1"
		;;
	esac
done
[[ -n "$action" ]] || fail "one of --verify-only or --publish is required"
for name in GH_TOKEN REPO TAG; do
	[[ -n "${!name:-}" ]] || fail "environment variable ${name} is required"
done
[[ "$TAG" =~ $semver ]] || fail "TAG must be a v-prefixed SemVer, got '${TAG}'"

# --- resolve the draft --------------------------------------------------------
release=$(gh release view "$TAG" --repo "$REPO" --json databaseId,isDraft,isPrerelease,tagName,assets) \
	|| fail "no Release exists for ${TAG} in ${REPO}"
[[ "$(jq -r '.tagName' <<<"$release")" == "$TAG" ]] || fail "release lookup returned a different tag"
[[ "$(jq -r '.isDraft' <<<"$release")" == true ]] || fail "${TAG} is not a draft; refusing to touch a published release"
prerelease=$(jq -r '.isPrerelease' <<<"$release")
[[ "$prerelease" == true || "$prerelease" == false ]] || fail "isPrerelease is not boolean"

remote_count=$(jq -r '.assets | length' <<<"$release")
duplicates=$(jq -r '[.assets[].name] | group_by(.) | map(select(length > 1) | .[0]) | .[]' <<<"$release")
[[ -z "$duplicates" ]] || fail "remote asset names are duplicated: ${duplicates//$'\n'/, }"

# name -> "size digest state" for the remote assets
declare -A remote_size=() remote_digest=() remote_state=()
while IFS=$'\t' read -r name size digest state; do
	remote_size["$name"]=$size
	remote_digest["$name"]=$digest
	remote_state["$name"]=$state
done < <(jq -r '.assets[] | [.name, (.size | tostring), (.digest // "null"), (.state // "null")] | @tsv' <<<"$release")

declare -A expected=()
failures=0
problem() {
	echo "::error title=Publication::$*" >&2
	failures=$((failures + 1))
}

require_remote() {
	local name=$1
	expected["$name"]=1
	if [[ -z "${remote_size[$name]+set}" ]]; then
		problem "asset ${name} is missing from the draft"
		return 1
	fi
	if [[ "${remote_state[$name]}" != uploaded ]]; then
		problem "asset ${name} is in state ${remote_state[$name]}, not uploaded"
		return 1
	fi
	if [[ "${remote_digest[$name]}" == null ]]; then
		problem "asset ${name} has no server-side digest yet; re-run after GitHub finishes processing the upload"
		return 1
	fi
	return 0
}

if [[ -n "$checksums" ]]; then
	[[ -r "$checksums" ]] || fail "checksum file is not readable: ${checksums}"
	line_count=0
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ -n "$line" ]] || continue
		line_count=$((line_count + 1))
		if [[ ! "$line" =~ ^([0-9a-f]{64})[[:space:]][[:space:]\*]([^[:space:]].*)$ ]]; then
			problem "checksum line is not sha256sum format: ${line}"
			continue
		fi
		hex=${BASH_REMATCH[1]}
		name=${BASH_REMATCH[2]}
		require_remote "$name" || continue
		if [[ "${remote_digest[$name]}" != "sha256:${hex}" ]]; then
			problem "remote digest for ${name} is ${remote_digest[$name]}, expected sha256:${hex}"
		fi
	done <"$checksums"
	((line_count > 0)) || fail "checksum file ${checksums} lists no assets"
fi

for path in ${assets[@]+"${assets[@]}"}; do
	[[ -f "$path" ]] || fail "asset file is not readable: ${path}"
	name=$(basename "$path")
	require_remote "$name" || continue
	size=$(stat -c '%s' "$path" 2>/dev/null || stat -f '%z' "$path")
	digest="sha256:$(sha256sum "$path" | cut -d ' ' -f 1)"
	if [[ "${remote_size[$name]}" != "$size" ]]; then
		problem "remote size for ${name} is ${remote_size[$name]}, local file is ${size} bytes"
	fi
	if [[ "${remote_digest[$name]}" != "$digest" ]]; then
		problem "remote bytes differ for ${name}: ${remote_digest[$name]} != ${digest}"
	fi
done

for name in ${expect_names[@]+"${expect_names[@]}"}; do
	require_remote "$name" || true
done

if ((exact)); then
	((${#expected[@]} > 0)) || fail "--exact needs at least one expectation (--checksums, --asset, or --expect-name)"
	if ((remote_count != ${#expected[@]})); then
		extra=$(jq -r --argjson expected "$(printf '%s\n' "${!expected[@]}" | jq -R . | jq -s .)" \
			'[.assets[].name] - $expected | .[]' <<<"$release")
		problem "draft holds ${remote_count} assets but exactly ${#expected[@]} were expected; unexpected: ${extra//$'\n'/, }"
	fi
fi

((failures == 0)) || fail "${failures} remote asset check(s) failed; the draft is unchanged"
echo "verified ${#expected[@]} expected asset(s) on draft ${TAG} (${remote_count} remote, prerelease=${prerelease})"

[[ "$action" == publish ]] || exit 0

# --- publish -------------------------------------------------------------------
args=(release edit "$TAG" --repo "$REPO" --draft=false)
if [[ "$prerelease" == false ]]; then
	args+=(--latest)
else
	args+=(--latest=false)
fi
gh "${args[@]}"
after=$(gh release view "$TAG" --repo "$REPO" --json isDraft,isPrerelease,tagName)
[[ "$(jq -r '.tagName' <<<"$after")" == "$TAG" ]] || fail "post-publish lookup returned a different tag"
[[ "$(jq -r '.isDraft' <<<"$after")" == false ]] || fail "${TAG} is still a draft after the flip"
echo "published ${TAG} (prerelease=${prerelease}, latest=$([[ "$prerelease" == false ]] && echo true || echo false))"
