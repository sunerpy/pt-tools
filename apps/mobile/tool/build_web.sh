#!/usr/bin/env bash
# 与 APK 一起打包的 Vue 编辑器。不要用桌面构建或占位文件替代。
set -euo pipefail
root="$(cd "$(dirname "$0")/../../.." && pwd)"
pnpm --dir "$root/web/frontend" run build:mobile
test -s "$root/apps/mobile/assets/mobile/mobile.html"
