SHELL=/bin/bash
CONFIG_FILE = config.toml
IMAGE_NAME = pt-tools
UPX_VERSION=4.2.4
UPX_DIR=upx-$(UPX_VERSION)-amd64_linux
UPX_BIN=$(UPX_DIR)/upx
PROJECT_ROOT=$(abspath .)
GIT_TAG=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
NEW_TAG=$(shell echo $(GIT_TAG) | awk -F. -v OFS=. '{print $$1, $$2, $$3+1}')

# Docker 镜像仓库
DOCKER_REPO = sunerpy
DOCKER_IMAGE_FULL = $(DOCKER_REPO)/$(IMAGE_NAME)
TAG ?= $(GIT_TAG)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_ID := $(shell git rev-parse HEAD)
ifeq ($(MAKECMDGOALS), prod-new)
    TAG=$(NEW_TAG)
else ifeq ($(MAKECMDGOALS), test-new)
    TAG=$(NEW_TAG)
else
    TAG=$(GIT_TAG)
endif

# 定义多平台
PLATFORMS=linux/amd64 linux/arm64 windows/amd64 windows/arm64
DOCKERPLATFORMS=linux/amd64
DIST_DIR = dist

HTTP_PROXY ?=
HTTPS_PROXY ?=
NO_PROXY ?=

# 默认基础镜像
BUILD_IMAGE ?= golang:1.24.3
BASE_IMAGE ?= alpine:3.20.3
BUILD_ENV ?= remote

.PHONY: build-local build-binaries build-local-docker build-remote-docker push-image clean code-format

# 本地构建二进制
build-local: code-format
	@echo "Building binary for local environment"
	mkdir -p $(DIST_DIR) && \
	GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) CGO_ENABLED=0 \
		go build -ldflags="-s -w \
		-X github.com/sunerpy/pt-tools/version.Version=$(TAG) \
		-X github.com/sunerpy/pt-tools/version.BuildTime=$(BUILD_TIME) \
		-X github.com/sunerpy/pt-tools/version.CommitID=$(COMMIT_ID)" \
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
		-X github.com/sunerpy/pt-tools/version.CommitID=$(COMMIT_ID)" \
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
		if [[ $$file == *windows-*.exe ]]; then \
			echo "Skipping compression for $$file (not supported by UPX)"; \
		elif $(UPX_BIN) -t $$file >/dev/null 2>&1; then \
			echo "Skipping $$file (already packed by UPX)"; \
		else \
			echo "Compressing $$file"; \
			$(UPX_BIN) -9 $$file || echo "Failed to compress $$file"; \
		fi; \
	done

# 压缩二进制文件
package-binaries: upx-binaries
	@echo "Packaging binaries into tar.gz/zip archives"
	for file in $(DIST_DIR)/$(IMAGE_NAME)-*; do \
		if [[ $$file == *.exe ]]; then \
			zip -j $(DIST_DIR)/$$(basename $$file)-$(TAG).zip $$file; \
		else \
			tar -czvf $(DIST_DIR)/$$(basename $$file)-$(TAG).tar.gz -C $(DIST_DIR) $$(basename $$file); \
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
		--build-arg CONFIG_FILE=$(CONFIG_FILE) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
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
		--build-arg CONFIG_FILE=$(CONFIG_FILE) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
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

# 代码格式化
code-format:
	@echo "Formatting code"
	bash style.sh
