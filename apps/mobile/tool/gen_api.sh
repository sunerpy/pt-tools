#!/usr/bin/env bash
# 由 docs/reference/app-api-v1.yaml 生成 App API 的 Dart 客户端（packages/ptt_api）。生成的文件不要手改，改契约以后重跑：
#
#   apps/mobile/tool/gen_api.sh
#
# CI（.github/workflows/mobile.yml）跑同一个脚本，再 git diff --exit-code 检查有没有漂移。需要 Java 11 以上。
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
root="$(cd "$here/../.." && pwd)"
version=7.26.0
sha256=1760050094997b9cc790cc1350be095f15da8c08a996b184d3eadc4ccda5e35e
jar="${OPENAPI_GENERATOR_JAR:-${XDG_CACHE_HOME:-$HOME/.cache}/openapi-generator/openapi-generator-cli-$version.jar}"

if [ ! -f "$jar" ]; then
  mkdir -p "$(dirname "$jar")"
  curl -fsSL -o "$jar.tmp" "https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/$version/openapi-generator-cli-$version.jar"
  mv "$jar.tmp" "$jar"
fi
echo "$sha256  $jar" | sha256sum -c --quiet -

out="$here/packages/ptt_api"
# 保留忽略清单，其余整个重新生成（删掉的接口、模型不会留下旧文件）
find "$out" -mindepth 1 -maxdepth 1 ! -name .openapi-generator-ignore -exec rm -rf {} +
java -jar "$jar" generate \
  -i "$root/docs/reference/app-api-v1.yaml" \
  -g dart \
  -o "$out" \
  --additional-properties=pubName=ptt_api,pubVersion=1.0.0,pubAuthor=pt-tools,pubHomepage=https://github.com/sunerpy/pt-tools,pubDescription="pt-tools App API v1 client (generated from docs/reference/app-api-v1.yaml)" \
  --global-property=apiTests=false,modelTests=false,apiDocs=false,modelDocs=false \
  >/dev/null
echo "已生成 $out"
