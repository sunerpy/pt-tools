#!/bin/bash

# 配置变量
REPO="sunerpy/pt-tools"
PLATFORM="linux-amd64" # 替换为你的默认平台
INSTALL_DIR="/usr/local/bin"

# 检测系统架构
detect_platform() {
    case "$(uname -s)-$(uname -m)" in
    Linux-x86_64) PLATFORM="linux-amd64" ;;
    Linux-aarch64) PLATFORM="linux-arm64" ;;
    Darwin-x86_64) PLATFORM="darwin-amd64" ;;
    Darwin-arm64) PLATFORM="darwin-arm64" ;;
    *)
        echo "Unsupported platform: $(uname -s)-$(uname -m)"
        exit 1
        ;;
    esac
}

# 获取最新版本的下载 URL
get_latest_release_url() {
    echo "Fetching the latest release for platform: $PLATFORM..."
    DOWNLOAD_URL=$(curl -s https://api.github.com/repos/$REPO/releases/latest |
        grep "browser_download_url.*$PLATFORM" |
        cut -d '"' -f 4)

    if [ -z "$DOWNLOAD_URL" ]; then
        echo "Failed to find a valid download URL for platform: $PLATFORM"
        exit 1
    fi
}

# 下载和安装工具
install_pt_tools() {
    echo "Downloading pt-tools from: $DOWNLOAD_URL"
    curl -L -o pt-tools-$PLATFORM.tar.gz "$DOWNLOAD_URL"

    echo "Extracting the binary..."
    tar -xvzf pt-tools-$PLATFORM.tar.gz

    echo "Installing pt-tools to $INSTALL_DIR..."
    sudo mv pt-tools "$INSTALL_DIR/pt-tools"
    sudo chmod +x "$INSTALL_DIR/pt-tools"

    echo "Cleaning up..."
    rm -f pt-tools-$PLATFORM.tar.gz

    echo "pt-tools installed successfully!"
}

# 主流程
detect_platform
get_latest_release_url
install_pt_tools

# 验证安装
echo "Verifying installation..."
pt-tools version
