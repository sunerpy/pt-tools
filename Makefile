SHELL=/bin/bash
ENV?=dev
CONFIG_FILE = $(ENV).yaml
NAMESPACE?=default
# ! 注意：如果修改镜像名，需要同时修改 deploy/xxx/values.yaml 文件中的镜像名
IMAGE_NAME = pt-tools
export ENV
export CONFIG_FILE
# 根路径
PROJECT_ROOT=$(abspath .)
# 获取当前的 Git tag 或手动设置的版本
GIT_TAG=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
NEW_TAG=$(shell echo $(GIT_TAG) | awk -F. -v OFS=. '{print $$1, $$2, $$3+1}')
# 判断目标名称，如果是 prod-new 则使用新 tag，否则使用现有 tag
ifeq ($(MAKECMDGOALS), prod-new)
    TAG=$(NEW_TAG)
else ifeq ($(MAKECMDGOALS), test-new)
    TAG=$(NEW_TAG)
else
    TAG=$(GIT_TAG)
endif

# Dockerfile路径
DOCKERFILE_PATH=$(strip $(PROJECT_ROOT))/Dockerfile
REPOSITORY = 107255705363.dkr.ecr.cn-northwest-1.amazonaws.com.cn/sunerpy/${IMAGE_NAME}
IMAGE_FULL_NAME = $(REPOSITORY):$(TAG)-$(ENV)
BUILD_IMAGE_SERVER = golang:1.23.1
.PHONY: test prod build-local code-format

dev: ENV = dev
dev: CONFIG_FILE = $(ENV).yaml
dev: NAMESPACE = dev-opsx
dev: unit-test build-local push-image clean
	@echo "Running build dev with unit tests : $(CONFIG_FILE)"

test: ENV = test
test: CONFIG_FILE = $(ENV).yaml
test: NAMESPACE = test-opsx
test: build-local push-image clean
	@echo "Running build test unit tests : $(CONFIG_FILE)"

test-new: ENV = test
test-new: CONFIG_FILE = $(ENV).yaml
test-new: NAMESPACE = test-opsx
test-new: build-local push-image clean


prod: ENV = prod
prod: CONFIG_FILE = $(ENV).yaml
prod: NAMESPACE = prod-opsx
prod: build-local push-image clean

prod-new: ENV = prod
prod-new: CONFIG_FILE = $(ENV).yaml
prod-new: NAMESPACE = prod-opsx
prod-new: build-local push-image clean

code-format:
	@echo "Formatting code"
	bash style.sh

build-local: code-format
	@echo "Building Docker image with context: $(PROJECT_ROOT)"
	docker build \
		-f $(DOCKERFILE_PATH) \
		--build-arg CONFIG_FILE=$(CONFIG_FILE) \
		--build-arg CONFIG_ENV=$(ENV) \
		-t $(IMAGE_FULL_NAME) $(PROJECT_ROOT)

push-image: build-local
	docker push $(IMAGE_FULL_NAME)

unit-test:
	@echo "Running unit tests"
	go test -p 1 -cover -v -gcflags=all=-l -coverprofile=coverage.out ./... || exit 1

# 清理构建镜像
clean:
	docker rmi $(IMAGE_FULL_NAME)