SHELL = /bin/bash
IMAGE_NAME = pt-tools
UPX_VERSION = 4.2.4
UPX_DIR = upx-$(UPX_VERSION)-amd64_linux
UPX_BIN = $(UPX_DIR)/upx
PROJECT_ROOT = $(abspath .)
GIT_TAG = $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
NEW_TAG = $(shell echo $(GIT_TAG) | awk -F. -v OFS=. '{print $$1, $$2, $$3+1}')

# Docker 镜像仓库
TAG ?= $(GIT_TAG)
DOCKER_REPO = sunerpy
DOCKER_IMAGE_FULL = $(DOCKER_REPO)/$(IMAGE_NAME)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_ID := $(shell git rev-parse HEAD)
ifeq ($(MAKECMDGOALS), prod-new)
  TAG = $(NEW_TAG)
else ifeq ($(MAKECMDGOALS), test-new)
  TAG = $(NEW_TAG)
else
  TAG ?= $(GIT_TAG)
endif

# 定义多平台
PLATFORMS = linux/amd64 linux/arm64 windows/amd64 windows/arm64
DOCKERPLATFORMS = linux/amd64,linux/arm64
DIST_DIR = dist

# Proxy 设置 (支持大小写环境变量)
HTTP_PROXY ?= $(http_proxy)
HTTPS_PROXY ?= $(https_proxy)
NO_PROXY ?= $(no_proxy)

# 默认基础镜像
BUILD_IMAGE ?= golang:1.25.7
BASE_IMAGE ?= alpine:3.20.3
NODE_IMAGE ?= node:25.2.0-alpine
BUILD_ENV ?= remote

.PHONY: build-local build-binaries build-local-docker build-remote-docker push-image clean fmt fmt-oxfmt fmt-go fmt-check lint unit-test coverage-summary build-extension generate-icons check-sites

# 本地构建二进制
build-local: fmt build-frontend
	@echo "Building binary for local environment"
	mkdir -p $(DIST_DIR) && \
	GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) CGO_ENABLED=0 \
	go build -ldflags="-s -w \
	-X github.com/sunerpy/pt-tools/version.Version=$(TAG) \
	-X github.com/sunerpy/pt-tools/version.BuildTime=$(BUILD_TIME) \
	-X github.com/sunerpy/pt-tools/version.CommitID=$(COMMIT_ID) \
	-X github.com/sunerpy/pt-tools/version.BuildOS=$(shell go env GOOS) \
	-X github.com/sunerpy/pt-tools/version.BuildArch=$(shell go env GOARCH)" \
	-o $(DIST_DIR)/$(IMAGE_NAME) .

# 多平台二进制构建
build-binaries:
	@echo "Building binaries for platforms: $(PLATFORMS)"
	for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		OUTPUT=$(DIST_DIR)/$(IMAGE_NAME)-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then OUTPUT=$$OUTPUT.exe; fi; \
			echo "Building for $$platform -> $$OUTPUT"; \
			GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w \
			-X github.com/sunerpy/pt-tools/version.Version=$(TAG) \
			-X github.com/sunerpy/pt-tools/version.BuildTime=$(BUILD_TIME) \
			-X github.com/sunerpy/pt-tools/version.CommitID=$(COMMIT_ID) \
			-X github.com/sunerpy/pt-tools/version.BuildOS=$$GOOS \
			-X github.com/sunerpy/pt-tools/version.BuildArch=$$GOARCH" \
			-o $$OUTPUT . || exit 1; \
		done

# 检测并安装 UPX
install-upx: build-binaries
	@echo "Checking for UPX..."
	if [ ! -f "$(UPX_BIN)" ]; then \
		echo "UPX not found. Downloading UPX $(UPX_VERSION)..."; \
		curl -sL -o upx.tar.xz https://github.com/upx/upx/releases/download/v$(UPX_VERSION)/upx-$(UPX_VERSION)-amd64_linux.tar.xz; \
		mkdir -p $(UPX_DIR); \
		tar -Jxf upx.tar.xz --strip-components=1 -C $(UPX_DIR); \
		chmod +x $(UPX_BIN); \
		rm upx.tdu ar.xz; \
		echo "UPX installed successfully."; \
	else \
		echo "UPX already installed at $(UPX_BIN)."; \
	fi

upx-binaries: install-upx
	@echo "Compressing binaries with UPX"
	for file in $(DIST_DIR)/$(IMAGE_NAME)-*; do \
		if $(UPX_BIN) -t $$file >/dev/null 2>&1; then \
			echo "Skipping $$file (already packed by UPX)"; \
		else \
			echo "Compressing $$file"; \
			$(UPX_BIN) -9 $$file || echo "Failed to compress $$file"; \
		fi; \
	done

# 压缩二进制文件 (不带版本号，支持 GitHub latest 重定向)
package-binaries: upx-binaries
	@echo "Packaging binaries into tar.gz/zip archives"
	for file in $(DIST_DIR)/$(IMAGE_NAME)-*; do \
		if [[ $$file == *.exe ]]; then \
			zip -j $(DIST_DIR)/$$(basename $$file).zip $$file; \
		else \
			tar -czvf $(DIST_DIR)/$$(basename $$file).tar.gz -C $(DIST_DIR) $$(basename $$file); \
		fi; \
	done

# Docker 镜像本地构建
build-local-docker: BUILD_ENV = local
build-local-docker:
	@echo "Preparing dependencies"
	@mkdir -p dist
	@echo "Building local Docker image"
	docker buildx build \
	--progress=plain \
	--network host \
	--platform $(DOCKERPLATFORMS) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
	--build-arg NODE_IMAGE=$(NODE_IMAGE) \
	--build-arg BUILD_ENV=$(BUILD_ENV) \
	--build-arg TAG=$(TAG) \
	--build-arg BUILD_TIME=$(BUILD_TIME) \
	--build-arg COMMIT_ID=$(COMMIT_ID) \
	--build-arg HTTP_PROXY=$(HTTP_PROXY) \
	--build-arg HTTPS_PROXY=$(HTTPS_PROXY) \
	--build-arg NO_PROXY=$(NO_PROXY) \
	--no-cache \
	-t $(DOCKER_IMAGE_FULL):$(TAG) \
	-t $(DOCKER_IMAGE_FULL):latest .

# Docker 镜像远程构建
build-remote-docker: BUILD_ENV = remote
build-remote-docker:
	@echo "Preparing dependencies"
	@mkdir -p dist
	@echo "Preparing dependencies"
	go mod vendor
	@echo "Building remote Docker image"
	docker buildx build \
	--progress=plain \
	--platform $(DOCKERPLATFORMS) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
	--build-arg NODE_IMAGE=$(NODE_IMAGE) \
	--build-arg BUILD_ENV=$(BUILD_ENV) \
	--build-arg TAG=$(TAG) \
	--build-arg BUILD_TIME=$(BUILD_TIME) \
	--build-arg COMMIT_ID=$(COMMIT_ID) \
	-t $(DOCKER_IMAGE_FULL):$(TAG) \
	-t $(DOCKER_IMAGE_FULL):latest \
	--push .

# 清理构建文件
clean:
	@echo "Cleaning up"
	rm -rf $(DIST_DIR) || true
	rm -rf $(UPX_DIR) || true

clean-docker:
	@echo "Cleaning Docker cache"
	docker builder prune -f

lint: lint-go lint-frontend

lint-go:
	@echo "Running Go linters..."
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo ""; \
		echo "Running go vet instead..."; \
		go vet ./...; \
	fi

lint-frontend:
	@echo "Running frontend linters with oxlint..."
	@cd web/frontend && if [ ! -d "node_modules" ]; then \
		echo "Installing dependencies..."; \
		pnpm install; \
	fi && \
	pnpm lint:check && \
	echo "" && \
	echo "Running Vue type check..." && \
	pnpm vue-tsc --noEmit

# Go 文件查找（排除 vendor 和 frontend）
GO_FILES = $(shell find . -name "*.go" -not -path "./vendor/*" -not -path "./web/frontend/*")

# 代码格式化
fmt: fmt-oxfmt fmt-go
	@echo "Formatting complete."

fmt-oxfmt:
	@echo "Formatting with oxfmt..."
	@cd web/frontend && if [ ! -d "node_modules" ]; then \
		echo "Installing dependencies..."; \
		pnpm install; \
	fi && \
	pnpm oxfmt --no-error-on-unmatched-pattern "$(PROJECT_ROOT)"

fmt-go:
	@echo "Formatting Go code..."
	@if command -v goimports > /dev/null 2>&1; then \
		echo "$(GO_FILES)" | tr ' ' '\n' | xargs -P 4 goimports -w -local github.com/sunerpy/pt-tools; \
	else \
		echo "goimports not found. Install with:"; \
		echo "  go install golang.org/x/tools/cmd/goimports@latest"; \
	fi
	@if command -v gofumpt > /dev/null 2>&1; then \
		echo "$(GO_FILES)" | tr ' ' '\n' | xargs -P 4 gofumpt -extra -w; \
	else \
		echo "gofumpt not found. Install with:"; \
		echo "  go install mvdan.cc/gofumpt@latest"; \
	fi

fmt-check:
	@echo "Checking code format..."
	@cd web/frontend && if [ ! -d "node_modules" ]; then \
		echo "Installing dependencies..."; \
		pnpm install; \
	fi && \
	pnpm oxfmt --no-error-on-unmatched-pattern --check "$(PROJECT_ROOT)"

unit-test:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 go test ./... -count=1 -race -cover -covermode=atomic -coverprofile=$(DIST_DIR)/coverage.out
	go tool cover -html=$(DIST_DIR)/coverage.out -o $(DIST_DIR)/coverage.html
	@echo "Coverage report: $(DIST_DIR)/coverage.html"

coverage-summary: unit-test
	@mkdir -p $(DIST_DIR)
	@test -f $(DIST_DIR)/coverage.out || (echo "Run make unit-test first"; exit 1)
	@echo "Filtering coverage data (excluding test files and mocks)..."
	@grep -v "_test.go" $(DIST_DIR)/coverage.out | grep -v "/mocks/" > $(DIST_DIR)/filtered_coverage.out
	@echo ""
	@echo "=== Coverage Summary (excluding test and mock files) ==="
	@go tool cover -func=$(DIST_DIR)/filtered_coverage.out | tee $(DIST_DIR)/coverage.txt
	@echo ""
	@echo "Filtered coverage saved to: $(DIST_DIR)/coverage.txt"

# 前端构建
build-frontend:
	@echo "Building frontend..."
	pnpm --dir web/frontend install
	pnpm --dir web/frontend build
	@echo "Frontend built to web/static/dist"

# 浏览器扩展构建（自动检查站点一致性）
build-extension: check-sites
	@echo "Building browser extension..."
	pnpm --dir tools/browser-extension install
	pnpm --dir tools/browser-extension run pack
	@echo "Extension packaged: tools/browser-extension/pt-tools-helper.zip"

# 从 public/pt-tools.png 一键生成所有尺寸图标（前端 + 扩展）
generate-icons:
	@echo "Generating icons from public/pt-tools.png..."
	node --experimental-strip-types scripts/generate-icons.ts

# 检查扩展内置站点与 Go 项目定义一致
check-sites:
	@echo "Checking built-in site consistency..."
	node --experimental-strip-types scripts/check-sites.ts

# 开发运行（先构建前端，再运行后端）
run-dev: build-frontend
	@echo "Starting development server..."
	go run main.go web

# 本地 CI 测试 (使用 act)
# ACT_IMAGE: 自定义 runner 镜像，解决 Docker Hub 网络问题
# ACT_CPU: 容器 CPU 限制 (默认 2 核)
# ACT_MEMORY: 容器内存限制 (默认 4g)
# GOPROXY: Go 模块代理
# 示例: make ci-local ACT_IMAGE=ghcr.io/catthehacker/ubuntu:act-latest ACT_CPU=2 ACT_MEMORY=4g GOPROXY=https://goproxy.cn,direct
ACT_IMAGE ?=
ACT_CPU ?= 2
ACT_MEMORY ?= 4g
GOPROXY ?= https://goproxy.cn,direct

# 构建 act 的环境变量参数
ACT_ENV_ARGS :=
ifneq ($(HTTP_PROXY),)
	ACT_ENV_ARGS += --env HTTP_PROXY=$(HTTP_PROXY) --env http_proxy=$(HTTP_PROXY)
endif
ifneq ($(HTTPS_PROXY),)
	ACT_ENV_ARGS += --env HTTPS_PROXY=$(HTTPS_PROXY) --env https_proxy=$(HTTPS_PROXY)
endif
ifneq ($(NO_PROXY),)
	ACT_ENV_ARGS += --env NO_PROXY=$(NO_PROXY) --env no_proxy=$(NO_PROXY)
endif
ifneq ($(GOPROXY),)
	ACT_ENV_ARGS += --env GOPROXY=$(GOPROXY)
endif

# 容器资源限制参数
ACT_CONTAINER_OPTS := --container-options "--cpus=$(ACT_CPU) --memory=$(ACT_MEMORY)"

ci-local:
	@if command -v act > /dev/null 2>&1; then \
		if [ -n "$(ACT_IMAGE)" ]; then \
			act push --container-architecture linux/amd64 -W .github/workflows/ci.yml -P ubuntu-latest=$(ACT_IMAGE) $(ACT_ENV_ARGS) $(ACT_CONTAINER_OPTS); \
		else \
			act push --container-architecture linux/amd64 -W .github/workflows/ci.yml $(ACT_ENV_ARGS) $(ACT_CONTAINER_OPTS); \
		fi; \
	else \
		echo "act not found. Install with:"; \
		echo "  brew install act  # macOS"; \
		echo "  curl -s https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash  # Linux"; \
		exit 1; \
	fi

ci-local-dry:
	@if command -v act > /dev/null 2>&1; then \
		if [ -n "$(ACT_IMAGE)" ]; then \
			act push --container-architecture linux/amd64 -W .github/workflows/ci.yml -P ubuntu-latest=$(ACT_IMAGE) $(ACT_ENV_ARGS) $(ACT_CONTAINER_OPTS) --dryrun; \
		else \
			act push --container-architecture linux/amd64 -W .github/workflows/ci.yml $(ACT_ENV_ARGS) $(ACT_CONTAINER_OPTS) --dryrun; \
		fi; \
	else \
		echo "act not found."; \
		exit 1; \
	fi
