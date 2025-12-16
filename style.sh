#!/bin/bash
set -e

echo "=== 格式化 Go 代码 ==="
# go build -ldflags="-s -w" -o pt-tools main.go && upx pt-tools
# find ./query ./models -type f -name '*.go' -exec sed -i -r '/^\s*$/d' {} \;
find . -type f -name '*.go' -not -path './vendor/*' -exec sed -i -r '/^\s*$/d' {} \;
find . -type f -name '*.go' -not -path './vendor/*' -exec gofumpt -w {} \;
go mod tidy
go mod vendor

echo "=== 格式化 Vue/TypeScript 代码 ==="
cd web/frontend
# 安装依赖（如果需要）
if [ ! -d "node_modules" ]; then
    echo "安装前端依赖..."
    pnpm install
fi
# 运行 Prettier 格式化
pnpm format
cd ../..

echo "=== 格式化完成 ==="
