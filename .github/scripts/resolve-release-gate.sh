#!/usr/bin/env bash
# Resolve the release gate for the same-run release templates and expose the
# tag-peel helper the candidate controller uses.
#
# Usage (inside the release-please job, after actions/checkout):
#   bash .github/scripts/resolve-release-gate.sh
#   bash .github/scripts/resolve-release-gate.sh --peel-tag vX.Y.Z   # prints the commit SHA
#
# Environment (all values arrive through `env:`, never inline in `run:`):
#   EVENT_NAME        github.event_name (push or workflow_dispatch)
#   INPUT_TAG         inputs.tag_name on workflow_dispatch: an existing DRAFT tag to rebuild
#   RELEASE_CREATED   release-please output release_created ("true" when a Release was created)
#   RELEASE_TAG       release-please output tag_name
#   RELEASE_VERSION   release-please output version (cross-checked against the tag)
#   RELEASE_SHA       release-please output sha (optional; cross-checked against the peeled tag)
#   GH_TOKEN          github.token; the job holds contents: write so drafts are visible
#   REPO              github.repository
#   REF_PROTECTED     github.ref_protected (required; must be "true")
#   RUN_REF           github.ref (defaults to GITHUB_REF)
#   DEFAULT_BRANCH    expected default branch (defaults to the repository's default branch via the API)
#   GATE_PASSTHROUGH  optional space-separated list of env names copied to GITHUB_OUTPUT with
#                     lowercase keys (for example "BUILD_BINARIES NPM_PUBLISH"), because job-level
#                     `if:` cannot read workflow env but can read needs.<job>.outputs.*
#
# Outputs (GITHUB_OUTPUT): build_artifacts, tag_name, version, prerelease, release_sha
#
# Trust rule: the job that runs this script holds a write token, so the script
# refuses to run unless the run is on the protected default branch. This mirrors
# the candidate controller's trust job.
#
# Sources:
#   https://raw.githubusercontent.com/googleapis/release-please-action/main/README.md (outputs)
#   https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/release/shared/fetch.go
#     (gh release view resolves drafts through GraphQL; REST releases/tags/TAG does not)
#   https://docs.github.com/en/actions/reference/security/secure-use#principle-of-least-privilege
set -euo pipefail

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

fail() {
	echo "::error title=Release gate::$*" >&2
	exit 1
}

require_env() {
	local name
	for name in "$@"; do
		[[ -n "${!name:-}" ]] || fail "environment variable ${name} is required"
	done
}

require_semver_tag() {
	[[ "$1" =~ $semver ]] || fail "tag must be a v-prefixed SemVer such as v1.2.3, got '${1}'"
}

# Print the commit a tag points at, dereferencing annotated tag objects.
peel_tag() {
	local tag=$1 ref object_type sha annotated hops=0
	ref=$(gh api "repos/${REPO}/git/ref/tags/${tag}") || fail "tag ${tag} does not exist in ${REPO}"
	object_type=$(jq -r '.object.type' <<<"$ref")
	sha=$(jq -r '.object.sha' <<<"$ref")
	while [[ "$object_type" == tag ]]; do
		hops=$((hops + 1))
		((hops <= 5)) || fail "tag ${tag} nests more than five tag objects"
		annotated=$(gh api "repos/${REPO}/git/tags/${sha}")
		object_type=$(jq -r '.object.type' <<<"$annotated")
		sha=$(jq -r '.object.sha' <<<"$annotated")
	done
	[[ "$object_type" == commit ]] || fail "tag ${tag} does not peel to a commit (${object_type})"
	[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || fail "tag ${tag} peeled to an invalid SHA"
	printf '%s\n' "$sha"
}

emit() {
	local out=${GITHUB_OUTPUT:-/dev/stdout}
	printf '%s\n' "$@" >>"$out"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	sed -n '2,36p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
	exit 0
fi

require_env GH_TOKEN REPO

if [[ "${1:-}" == "--peel-tag" ]]; then
	[[ $# -eq 2 ]] || fail "--peel-tag takes exactly one tag"
	require_semver_tag "$2"
	peel_tag "$2"
	exit 0
fi
[[ $# -eq 0 ]] || fail "unknown arguments: $*"

# --- trust: protected default branch only -----------------------------------
[[ -n "${REF_PROTECTED+set}" ]] || fail "set REF_PROTECTED: \${{ github.ref_protected }} in the step env"
run_ref=${RUN_REF:-${GITHUB_REF:-}}
[[ -n "$run_ref" ]] || fail "RUN_REF (github.ref) is required"
default_branch=${DEFAULT_BRANCH:-}
if [[ -z "$default_branch" ]]; then
	default_branch=$(gh api "repos/${REPO}" --jq .default_branch)
fi
[[ "$REF_PROTECTED" == true ]] \
	|| fail "the release gate runs only on a protected branch (github.ref_protected is '${REF_PROTECTED}'); protect ${default_branch} with a ruleset or branch protection"
[[ "$run_ref" == "refs/heads/${default_branch}" ]] \
	|| fail "the release gate runs only from refs/heads/${default_branch}, not ${run_ref}"

# --- gate --------------------------------------------------------------------
require_env EVENT_NAME
case "$EVENT_NAME" in
workflow_dispatch)
	[[ -n "${INPUT_TAG:-}" ]] || fail "workflow_dispatch requires the tag_name input"
	require_semver_tag "$INPUT_TAG"
	tag=$INPUT_TAG
	draft_message="manual rebuilds require an existing draft Release for ${tag}"
	;;
push)
	if [[ "${RELEASE_CREATED:-}" != true ]]; then
		echo "no release was created by this push; downstream release jobs stay skipped"
		emit "build_artifacts=false" "tag_name=" "version=" "prerelease=" "release_sha="
		exit 0
	fi
	[[ -n "${RELEASE_TAG:-}" ]] || fail "release-please reported release_created without a tag_name"
	require_semver_tag "$RELEASE_TAG"
	tag=$RELEASE_TAG
	draft_message="release-please must create the Release as a draft (set \"draft\": true in release-please-config.json)"
	;;
*)
	fail "unsupported event ${EVENT_NAME}; the release gate runs on push and workflow_dispatch only"
	;;
esac

release_sha=$(peel_tag "$tag")
if [[ -n "${RELEASE_SHA:-}" && "$RELEASE_SHA" != "$release_sha" ]]; then
	fail "release-please reported sha ${RELEASE_SHA} but ${tag} peels to ${release_sha}"
fi
version=${tag#v}
if [[ -n "${RELEASE_VERSION:-}" && "$RELEASE_VERSION" != "$version" ]]; then
	fail "release-please reported version ${RELEASE_VERSION} but the tag is ${tag}"
fi

release=$(gh release view "$tag" --repo "$REPO" --json isDraft,isPrerelease,tagName) \
	|| fail "no Release exists for ${tag}"
[[ "$(jq -r '.tagName' <<<"$release")" == "$tag" ]] || fail "Release lookup returned a different tag"
[[ "$(jq -r '.isDraft' <<<"$release")" == true ]] || fail "$draft_message"
prerelease=$(jq -r '.isPrerelease' <<<"$release")
[[ "$prerelease" == true || "$prerelease" == false ]] || fail "isPrerelease is not boolean"

echo "release gate open: ${tag} (${version}) at ${release_sha}, prerelease=${prerelease}"
emit "build_artifacts=true" "tag_name=${tag}" "version=${version}" \
	"prerelease=${prerelease}" "release_sha=${release_sha}"

for name in ${GATE_PASSTHROUGH:-}; do
	[[ "$name" =~ ^[A-Z][A-Z0-9_]*$ ]] || fail "GATE_PASSTHROUGH names must match ^[A-Z][A-Z0-9_]*$: ${name}"
	value=${!name-}
	[[ "$value" != *$'\n'* ]] || fail "GATE_PASSTHROUGH value for ${name} must be single-line"
	emit "${name,,}=${value}"
done
