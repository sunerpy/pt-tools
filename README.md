# pt-tools

[![Go Version](https://img.shields.io/badge/Go-1.25+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/sunerpy/pt-tools.svg)](https://hub.docker.com/r/sunerpy/pt-tools)

`pt-tools` 是一个功能强大的 PT（Private Tracker）站点自动化管理工具，提供 RSS 订阅自动下载、多站点种子搜索、用户信息统计、下载器管理等功能，帮助用户高效管理多个 PT 站点。

![Example](docs/images/user-info.png)

<details>
<summary>点击查看任务列表示例 (Click to view Task List Screenshot)</summary>

![Task List](docs/images/task-list.png)

</details>

## 功能特性

| 功能                 | 描述                                                              |
| -------------------- | ----------------------------------------------------------------- |
| **RSS 自动订阅**     | 自动解析 RSS 订阅，智能识别免费种子并自动下载推送                 |
| **多站点种子搜索**   | 跨站点搜索，支持批量下载和批量推送到下载器,或者下载种子文件到本地 |
| **用户信息统计**     | 聚合展示所有站点的上传量、下载量、分享率、魔力值、等级进度等      |
| **下载器管理**       | 支持多个下载器实例，可配置不同的下载目录和启动策略                |
| **过滤规则**         | 对 RSS 订阅进行精细化筛选，支持关键词/通配符/正则表达式           |
| **免费结束自动暂停** | 监控种子免费状态，免费期结束时自动暂停未完成的下载任务            |
| **版本更新检查**     | 自动检测新版本，支持代理设置，在 Web 界面展示更新日志             |
| **Web 管理界面**     | Web UI 管理后台，方便配置和监控                                   |

### 免费结束自动暂停

针对 PT 站点免费种子的智能管理功能，帮助用户在免费期结束前避免产生不必要的下载量消耗。

**工作原理**：

1. RSS 订阅下载免费种子时，系统自动记录种子的免费结束时间
2. 为每个种子创建独立定时器，在免费结束时刻精确触发检查
3. 免费期结束时，自动检测下载进度，暂停未完成的任务
4. 已完成的种子不受影响，继续正常做种

**功能特点**：

- **精确定时**：独立定时器 + 周期检查双重机制，支持应用重启后自动恢复监控
- **智能判断**：仅暂停未完成任务，已完成任务继续做种
- **手动恢复**：支持在 Web 界面手动恢复暂停的任务（不再受免费限制）
- **批量管理**：支持批量删除暂停任务，可选是否同时删除数据文件
- **历史归档**：查看历史暂停记录和处理结果

**启用方式**：在添加或编辑 RSS 订阅时，开启「免费结束时暂停」开关即可。

## 内置支持站点

| 站点             | 站点类型 | 认证方式 | 支持功能                      |
| ---------------- | -------- | -------- | ----------------------------- |
| **HDSky**        | NexusPHP | Cookie   | RSS、搜索、用户信息、等级要求 |
| **SpringSunday** | NexusPHP | Cookie   | RSS、搜索、用户信息、等级要求 |
| **M-Team**       | mTorrent | API Key  | RSS、搜索、用户信息           |
| **HDDolby**      | NexusPHP | Cookie   | RSS、搜索、用户信息           |

> **扩展站点支持**：如需支持其他站点，欢迎提交 [Issue](https://github.com/sunerpy/pt-tools/issues) 或 [Pull Request](https://github.com/sunerpy/pt-tools/pulls)。

## 快速开始

### Docker 部署（推荐）

镜像地址：[Docker Hub](https://hub.docker.com/r/sunerpy/pt-tools)

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

### Docker Compose（推荐）

```yaml
version: "3.8"
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      TZ: "Asia/Shanghai"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
```

启动后访问 `http://localhost:8080` 进入 Web 管理界面。

**默认登录账号**：`admin` / `adminadmin`

> 更多部署示例请参考：
>
> - [examples/docker-run.md](examples/docker-run.md) - Docker 单容器运行详解
> - [examples/docker-compose.yml](examples/docker-compose.yml) - Docker Compose 编排
> - [examples/binary-run.md](examples/binary-run.md) - 二进制运行和 systemd 配置

### 二进制运行

前往 [Releases 页面](https://github.com/sunerpy/pt-tools/releases) 下载预编译二进制文件。

**Linux**：

```bash
wget https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-linux-amd64.tar.gz
tar -xzf pt-tools-linux-amd64.tar.gz
chmod +x pt-tools
./pt-tools web --host 0.0.0.0 --port 8080
```

**Windows (PowerShell)**：

```powershell
# 下载并解压
Invoke-WebRequest -Uri "https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-windows-amd64.exe.zip" -OutFile "pt-tools.zip"
Expand-Archive -Path "pt-tools.zip" -DestinationPath "."

# 运行
.\pt-tools.exe web --host 0.0.0.0 --port 8080
```

| 系统    | 架构  | 文件名                           |
| ------- | ----- | -------------------------------- |
| Linux   | amd64 | `pt-tools-linux-amd64.tar.gz`    |
| Linux   | arm64 | `pt-tools-linux-arm64.tar.gz`    |
| Windows | amd64 | `pt-tools-windows-amd64.exe.zip` |
| Windows | arm64 | `pt-tools-windows-arm64.exe.zip` |

## 使用指南

### 初次配置流程

1. **启动服务**：使用 Docker 或二进制启动 pt-tools
2. **登录管理界面**：访问 `http://localhost:8080`，使用默认账号登录
3. **修改密码**：首次登录后建议修改默认密码
4. **配置下载器**：添加 qBittorrent 或 Transmission
5. **配置站点**：启用站点并填写 Cookie 或 API Key
6. **配置 RSS 订阅**：添加 RSS 订阅实现自动下载
7. **配置过滤规则**（可选）：创建过滤规则实现精准下载

## 文档

| 文档                                                           | 说明                               |
| -------------------------------------------------------------- | ---------------------------------- |
| **[获取 Cookie / API Key](docs/guide/get-cookie-apikey.md)**   | 详细介绍如何从各站点获取认证信息   |
| **[RSS 订阅配置指南](docs/guide/rss-subscription.md)**         | 如何配置 RSS 订阅实现自动下载      |
| **[过滤规则与追剧指南](docs/guide/filter-rules-tv-series.md)** | 使用过滤规则自动追剧、筛选资源     |
| **[配置说明](docs/configuration.md)**                          | 环境变量、全局设置、下载器配置详解 |
| **[常见问题 (FAQ)](docs/faq.md)**                              | 常见问题和解决方案                 |
| **[开发指南](docs/development.md)**                            | 从源码构建、技术架构、贡献指南     |

## 贡献

欢迎贡献代码或提交问题！

- **提交 Issue**：[GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- **提交 PR**：[GitHub Pull Requests](https://github.com/sunerpy/pt-tools/pulls)

详细的贡献流程请参考 [开发指南](docs/development.md)。

## 更新日志

查看 [Releases](https://github.com/sunerpy/pt-tools/releases) 获取完整更新日志。

## 许可证

[MIT License](LICENSE)

## Star History

如果这个项目对你有帮助，请给一个 Star 支持一下！

[![Star History Chart](https://api.star-history.com/svg?repos=sunerpy/pt-tools&type=Date)](https://star-history.com/#sunerpy/pt-tools&Date)

---

**免责声明**：本工具仅供学习和研究使用，请遵守各 PT 站点的规则，合理使用。
