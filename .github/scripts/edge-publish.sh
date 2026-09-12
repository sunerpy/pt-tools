#!/usr/bin/env bash
# Edge Add-ons 发布 —— 由 .github/workflows/release.yml 的 edge-publish job 调用。
#
# 依据 CI/Release 迁移设计 v7 §4.8：
#   * 不得对整个 job 施加 blanket continue-on-error；
#   * 仅"前一次 submission 仍在 InReview"这一已识别的不透明失败可软失败
#     （warning + exit 0），其余包错误与 API 错误一律保持 red；
#   * 同一 tag 重跑必须幂等，且不得产生第二个 submission。
#
# 幂等机制的设计偏离（须知）：v7 原文要求"上传前查询商店当前版本"。Edge
# Add-ons Public API 的「已发布版本」查询端点无法从既有可用证据中确认，
# 因此本脚本不凭猜测调用未验证端点，改用与 Telegram 公告一致的
# **tag 键控 marker**（Release body 内的 HTML 注释）达成同一性质：
#   第一层 —— marker 命中即跳过，同 tag 重跑不会产生第二个 submission；
#   第二层 —— Edge 自身的串行提交约束（InReview 不透明失败）兜底。
# v7 阶段 6 的 stable 验收仍须实测「同一 stable tag 重跑未产生第二个
# submission」，并记录两次 run/job ID 与最终商店状态。
#
# 环境变量（全部由 workflow 的 env: 传入，run: 块内不出现 ${{ }} 插值）：
#   EDGE_API_KEY EDGE_CLIENT_ID EDGE_PRODUCT_ID ZIP_FILE
#   GH_TOKEN GITHUB_REPOSITORY TAG_NAME   （marker 读写用）
#
# 参考：https://learn.microsoft.com/en-us/microsoft-edge/extensions-chromium/publish/api/using-addons-api
set -euo pipefail

EDGE_API="${EDGE_API:-https://api.addons.microsoftedge.microsoft.com}"
MARKER='<!-- pt-tools-edge-submitted -->'

for name in EDGE_API_KEY EDGE_CLIENT_ID EDGE_PRODUCT_ID ZIP_FILE TAG_NAME; do
	if [ -z "${!name:-}" ]; then
		echo "::error title=Edge::environment variable ${name} is required" >&2
		exit 1
	fi
done

if [ ! -s "$ZIP_FILE" ]; then
	echo "::error title=Edge::package not found or empty: ${ZIP_FILE}" >&2
	exit 1
fi

auth=(-H "Authorization: ApiKey ${EDGE_API_KEY}" -H "X-ClientID: ${EDGE_CLIENT_ID}")

# ---- 第一层幂等：tag 键控 marker ----
body=$(gh release view "$TAG_NAME" --repo "$GITHUB_REPOSITORY" --json body --jq '.body // ""')
if printf '%s' "$body" | grep -qF "$MARKER"; then
	echo "Edge submission for ${TAG_NAME} already recorded; skipping upload to avoid a second submission."
	exit 0
fi

# ---- 上传包 ----
headers_file=$(mktemp)
body_file=$(mktemp)
trap 'rm -f "$headers_file" "$body_file"' EXIT

http_code=$(curl -sS -o "$body_file" -D "$headers_file" -w "%{http_code}" \
	"${auth[@]}" \
	-H "Content-Type: application/zip" \
	-X POST \
	-T "$ZIP_FILE" \
	"${EDGE_API}/v1/products/${EDGE_PRODUCT_ID}/submissions/draft/package")

echo "Upload response: ${http_code}"
echo "::group::Upload response headers"
cat "$headers_file"
echo "::endgroup::"
echo "::group::Upload response body"
cat "$body_file"
echo "::endgroup::"

if [ "$http_code" != "202" ]; then
	echo "::error title=Edge::upload failed with HTTP ${http_code}" >&2
	exit 1
fi

# OperationID 由响应的 Location 头返回（不是 body）。
upload_op=$(awk 'BEGIN{IGNORECASE=1} /^Location:/ {print $2}' "$headers_file" | tr -d '\r\n' | xargs)
if [ -z "$upload_op" ]; then
	echo "::error title=Edge::upload succeeded but the Location header carried no OperationID" >&2
	exit 1
fi
echo "Upload OperationID: ${upload_op}"

# ---- 等待上传处理 ----
# 区分两种 Failed：
#   1. 通用失败 message 且 errorCode/errors 皆空 —— 通常是 Microsoft 因
#      「该 product 已有 InReview 的 submission」而拒绝，无法通过 Public API
#      自愈，需等前一次审核完成。实测可能 1s 内返回，也可能约 30s 返回。
#      此时输出 warning 并以 exit 0 退出（软失败），不阻塞已完成的 release。
#   2. errorCode 或 errors 非空 —— 真实包错误，硬失败。
soft_fail=false
status=""
status_response=""
started_at=$(date +%s)
for i in $(seq 1 30); do
	sleep 10
	status_response=$(curl -sS "${auth[@]}" \
		"${EDGE_API}/v1/products/${EDGE_PRODUCT_ID}/submissions/draft/package/operations/${upload_op}")
	status=$(printf '%s' "$status_response" | grep -oP '"status"\s*:\s*"\K[^"]+' || echo "")
	echo "Attempt ${i}: status=${status:-<empty>}"

	if [ "$status" = "Succeeded" ]; then
		echo "Upload processing complete"
		break
	fi

	if [ "$status" = "Failed" ]; then
		elapsed=$(($(date +%s) - started_at))
		message=$(printf '%s' "$status_response" | grep -oP '"message"\s*:\s*"\K[^"]+' || echo "")
		error_code=$(printf '%s' "$status_response" | grep -oP '"errorCode"\s*:\s*"\K[^"]+' || echo "")
		has_errors=$(printf '%s' "$status_response" | grep -oP '"errors"\s*:\s*\[\K[^]]*' || echo "")

		echo "::group::Failure response"
		echo "$status_response"
		echo "::endgroup::"
		echo "Elapsed: ${elapsed}s, message=${message:-<empty>}, errorCode=${error_code:-<null>}, errors=[${has_errors:-<empty>}]"

		if [ -z "$error_code" ] && [ -z "$has_errors" ] \
			&& [ "$message" = "An error occurred while performing the operation" ]; then
			echo "::warning::Edge rejected the upload with an opaque failure (elapsed ${elapsed}s, no errorCode/errors)."
			echo "::warning::Most likely cause: a previous submission for this product is still InReview."
			echo "::warning::Edge serializes submissions: only one in-flight submission per product is allowed."
			echo "::warning::Action: wait for the previous submission to reach InExtensionStore (Live), then re-run:"
			echo "::warning::  gh workflow run release.yml --ref main"
			echo "::warning::Skipping the store publish. The GitHub Release and its assets are unaffected."
			soft_fail=true
			break
		fi

		echo "::error title=Edge::upload processing failed (real package error: errorCode=${error_code:-<null>}, elapsed=${elapsed}s)" >&2
		exit 1
	fi
done

if [ "$status" != "Succeeded" ] && [ "$soft_fail" != "true" ]; then
	echo "::error title=Edge::upload processing did not reach Succeeded after 30 polls (last status: ${status:-<empty>})" >&2
	echo "Last response: ${status_response}" >&2
	exit 1
fi

if [ "$soft_fail" = "true" ]; then
	exit 0
fi

# ---- 提交发布 ----
: >"$headers_file"
: >"$body_file"
http_code=$(curl -sS -o "$body_file" -D "$headers_file" -w "%{http_code}" \
	"${auth[@]}" \
	-H "Content-Type: application/json" \
	-X POST \
	-d "{\"notes\":\"Automated publish via CI for ${TAG_NAME}\"}" \
	"${EDGE_API}/v1/products/${EDGE_PRODUCT_ID}/submissions")

echo "Publish response: ${http_code}"
echo "::group::Publish response body"
cat "$body_file"
echo "::endgroup::"

if [ "$http_code" != "202" ]; then
	echo "::error title=Edge::publish failed with HTTP ${http_code}" >&2
	exit 1
fi

publish_op=$(awk 'BEGIN{IGNORECASE=1} /^Location:/ {print $2}' "$headers_file" | tr -d '\r\n' | xargs)
if [ -z "$publish_op" ]; then
	echo "::error title=Edge::publish succeeded but the Location header carried no OperationID" >&2
	exit 1
fi
echo "Publish OperationID: ${publish_op}"

# ---- 记录 marker（在等待审核之前）----
# 提交已被 Edge 接受，此后同一 tag 的重跑绝不能再提交一次，
# 因此在轮询审核状态之前先落 marker。
printf '%s' "$body" >body.md
printf '\n%s\n' "$MARKER" >>body.md
gh release edit "$TAG_NAME" --repo "$GITHUB_REPOSITORY" --notes-file body.md
echo "Recorded ${MARKER} on ${TAG_NAME}"

# ---- 等待发布处理 ----
status=""
for i in $(seq 1 30); do
	sleep 10
	status_response=$(curl -sS "${auth[@]}" \
		"${EDGE_API}/v1/products/${EDGE_PRODUCT_ID}/submissions/operations/${publish_op}")
	status=$(printf '%s' "$status_response" | grep -oP '"status"\s*:\s*"\K[^"]+' || echo "")
	echo "Attempt ${i}: status=${status:-<empty>}"

	if [ "$status" = "Succeeded" ]; then
		echo "Submission accepted by Edge Add-ons; pending Microsoft review"
		break
	fi

	if [ "$status" = "Failed" ]; then
		echo "::error title=Edge::publish processing failed" >&2
		echo "$status_response" >&2
		exit 1
	fi
done

if [ "$status" != "Succeeded" ]; then
	echo "::error title=Edge::publish processing did not reach Succeeded after 30 polls (last status: ${status:-<empty>})" >&2
	echo "Last response: ${status_response}" >&2
	exit 1
fi
