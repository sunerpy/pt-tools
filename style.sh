#!/bin/bash
# go build -ldflags="-s -w" -o pt-tools main.go && upx pt-tools
# find ./query ./models -type f -name '*.go' -exec sed -i -r '/^\s*$/d' {} \;
find . -type f -name '*.go' -not -path './vendor/*' -exec sed -i -r '/^\s*$/d' {} \;
find . -type f -name '*.go' -not -path './vendor/*' -exec gofumpt -w {} \;
go mod tidy
go mod vendor