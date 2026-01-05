# pt-tools

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/sunerpy/pt-tools.svg)](https://hub.docker.com/r/sunerpy/pt-tools)

`pt-tools` 是一个功能强大的 PT（Private Tracker）站点自动化管理工具，提供 RSS 订阅自动下载、多站点种子搜索、用户信息统计、下载器管理等功能，帮助用户高效管理多个 PT 站点。

## 功能特性

### 核心功能

| 功能 | 描述 |
|------|------|
| **RSS 自动订阅** | 自动解析 RSS 订阅，智能识别免费种子并自动下载推送 |
| **多站点种子搜索** | 跨站点并发搜索，支持批量下载和批量推送到下载器 |
| **用户信息统计** | 聚合展示所有站点的上传量、下载量、分享率、魔力值、等级进度等 |
| **下载器管理** | 支持多个下载器实例，可配置不同的下载目录和启动策略 |
| **过滤规则** | 对 RSS 订阅进行精细化筛选，支持关键词/通配符/正则表达式 |
| **Web 管理界面** | 现代化的 Vue 3 Web UI，方便配置和监控 |

### 种子搜索功能

- **跨站点并发搜索**：同时搜索多个站点，结果自动合并去重
- **批量操作**：支持批量选择、批量下载、批量推送
- **灵活推送**：可选择目标下载器和下载目录
- **智能去重**：自动识别已存在种子，避免重复推送
- **多维排序**：支持按做种数、大小、发布时间等排序
- **免费标识**：清晰显示种子免费状态和剩余时间

### 下载器管理

- **多客户端支持**：
  - qBittorrent（推荐）
  - Transmission
- **多实例管理**：支持配置多个下载器实例
- **目录管理**：为每个下载器设置多个下载目录（别名管理）
- **默认设置**：支持设置默认下载器和默认下载目录
- **启动控制**：支持任务自动启动/暂停启动配置
- **健康检查**：实时检测下载器连接状态

### 过滤规则

过滤规则可对 RSS 订阅中的种子进行进一步筛选：

| 匹配模式 | 说明 | 示例 |
|----------|------|------|
| **关键词** | 包含指定关键词即匹配 | `4K`, `REMUX` |
| **通配符** | 支持 `*` 和 `?` 通配符 | `*.2024.*`, `S01E??` |
| **正则表达式** | 完整的正则支持 | `S\d{2}E\d{2}`, `(4K\|2160p)` |

**特色功能**：

- 可配置下载符合过滤条件的**非免费种子**（适用于追更特定资源）
- 支持匹配标题、标签或两者
- 支持规则优先级设置

### 用户信息统计

- **数据聚合**：汇总所有站点的上传量、下载量、分享率
- **详细展示**：
  - 魔力值 / 时魔（每小时魔力）
  - 做种积分 / 做种体积
  - 等级进度和升级要求
  - 未读消息数
- **数据刷新**：支持手动刷新单站或全部站点

## 内置支持站点

目前内置支持以下站点：

| 站点 | 站点类型 | 认证方式 | 支持功能 |
|------|----------|----------|----------|
| **HDSky** | NexusPHP | Cookie | RSS、搜索、用户信息、等级要求 |
| **SpringSunday** | NexusPHP | Cookie | RSS、搜索、用户信息、等级要求 |
| **M-Team** | mTorrent | API Key | RSS、搜索、用户信息 |
| **HDDolby** | NexusPHP | Cookie | RSS、搜索、用户信息 |

> **扩展站点支持**：如需支持其他站点，欢迎提交 [Issue](https://github.com/sunerpy/pt-tools/issues) 或 [Pull Request](https://github.com/sunerpy/pt-tools/pulls)。  
> 新站点适配主要涉及 `site/v2/definitions/` 目录下的站点定义文件，可参考现有站点实现。

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

```bash
# Linux
wget https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-linux-amd64.tar.gz
tar -xzf pt-tools-linux-amd64.tar.gz
chmod +x pt-tools
./pt-tools web --host 0.0.0.0 --port 8080

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-windows-amd64.exe.zip" -OutFile "pt-tools.zip"
Expand-Archive -Path "pt-tools.zip" -DestinationPath "."
.\pt-tools.exe web --host 0.0.0.0 --port 8080
```

| 系统 | 架构 | 文件名 |
|------|------|--------|
| Linux | amd64 | `pt-tools-linux-amd64.tar.gz` |
| Linux | arm64 | `pt-tools-linux-arm64.tar.gz` |
| Windows | amd64 | `pt-tools-windows-amd64.exe.zip` |
| Windows | arm64 | `pt-tools-windows-arm64.exe.zip` |

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PT_HOST` | Web 监听地址 | `0.0.0.0` |
| `PT_PORT` | Web 监听端口 | `8080` |
| `PT_ADMIN_USER` | 管理员用户名 | `admin` |
| `PT_ADMIN_PASS` | 管理员密码 | `adminadmin` |
| `PT_ADMIN_RESET` | 重置管理员密码（设为 `1` 启用） | - |
| `PUID` | 容器用户 ID | `1000` |
| `PGID` | 容器组 ID | `1000` |
| `TZ` | 时区 | `Asia/Shanghai` |

## 使用指南

### 初次配置流程

1. **启动服务**：使用 Docker 或二进制启动 pt-tools
2. **登录管理界面**：访问 `http://localhost:8080`，使用默认账号登录
3. **修改密码**：首次登录后建议修改默认密码
4. **配置下载器**：
   - 进入「下载器设置」页面
   - 添加 qBittorrent 或 Transmission
   - 设置下载目录（可选）
   - 设为默认下载器
5. **配置站点**：
   - 进入「站点列表」页面
   - 启用需要的站点
   - 填写 Cookie 或 API Key
   - 添加 RSS 订阅
6. **配置过滤规则**（可选）：
   - 进入「过滤规则」页面
   - 创建需要的过滤规则
   - 关联到 RSS 订阅

### 获取站点认证信息

#### Cookie 认证（HDSky、SpringSunday、HDDolby）

1. 登录站点
2. 打开浏览器开发者工具（F12）
3. 切换到「网络」或「Network」标签
4. 刷新页面，选择任意请求
5. 在请求头中找到 `Cookie` 字段，复制整个值

#### API Key 认证（M-Team）

1. 登录 M-Team
2. 进入「控制面板」→「安全设置」
3. 生成或复制 API Key

### 使用种子搜索

1. 进入「种子搜索」页面
2. 选择要搜索的站点（可多选）
3. 输入关键词，点击搜索
4. 在搜索结果中：
   - 点击下载图标：下载 .torrent 文件
   - 点击推送图标：推送到下载器
   - 勾选多个种子：批量推送
5. 推送时可选择：
   - 目标下载器
   - 下载目录
   - 分类和标签
   - 是否自动开始

### 配置过滤规则示例

**示例 1：只下载 4K 资源**

```
模式: 关键词
内容: 4K,2160p
匹配: 标题
要求免费: 是
```

**示例 2：追更特定剧集（不限免费）**

```
模式: 正则表达式
内容: 某剧名.*S01E\d{2}
匹配: 标题
要求免费: 否  ← 即使非免费也会下载
```

**示例 3：排除特定标签**

```
模式: 通配符
内容: *HDSWEB*
匹配: 标签
要求免费: 是
```

## 配置说明

### 全局设置

| 配置项 | 说明 | 建议值 |
|--------|------|--------|
| 默认间隔(分钟) | RSS 任务的默认执行间隔 | 15-30 分钟 |
| 种子下载目录 | 保存 `.torrent` 文件的目录 | 默认即可 |
| 启用限速 | 用于判断种子是否能在免费期内完成 | 按需开启 |
| 下载限速(MB/s) | 预估下载速度 | 根据实际带宽设置 |
| 最大种子大小(GB) | 超过此大小的种子将被跳过 | 50-100 GB |
| 自动启动任务 | 程序启动时是否自动运行 RSS 任务 | 是 |

### 下载器配置

| 配置项 | 说明 |
|--------|------|
| 名称 | 下载器标识名称，如「家里qBit」「NAS」 |
| 类型 | qBittorrent 或 Transmission |
| URL | WebUI 地址，如 `http://192.168.1.10:8080` |
| 用户名/密码 | 登录凭据 |
| 设为默认 | 勾选后作为默认下载器 |
| 自动启动 | 推送任务后是否自动开始下载 |

**下载目录配置**：

- 每个下载器可配置多个下载目录
- 设置别名方便识别，如「电影」「剧集」「动漫」
- 可设置默认下载目录

### RSS 订阅配置

| 配置项 | 说明 |
|--------|------|
| 名称 | 订阅标识，如「HDSky电影」「MT电视剧」 |
| 链接 | RSS 订阅 URL（从站点获取） |
| 分类 | 下载器中的分类标签 |
| 标签 | 任务标签，便于管理 |
| 间隔(分钟) | 执行间隔，5-1440 分钟 |
| 下载器 | 指定下载器（可选） |
| 下载路径 | 指定下载目录（可选） |
| 过滤规则 | 关联的过滤规则（可选） |

## 数据持久化

所有数据存储在数据目录中：

| 路径 | Docker | 本地 |
|------|--------|------|
| 数据目录 | `/app/.pt-tools` | `~/.pt-tools` |
| 数据库 | `torrents.db` | `torrents.db` |
| 种子文件 | `downloads/` | `downloads/` |

**备份建议**：定期备份 `torrents.db` 文件即可保存所有配置和任务记录。

## 常见问题

### 下载器连接失败

1. 检查下载器 URL 是否正确（注意端口）
2. 检查用户名密码是否正确
3. 确认下载器已启用 WebUI
4. 检查防火墙是否放行端口
5. Docker 环境注意使用宿主机 IP 而非 `localhost`

### Cookie 失效

1. 重新从浏览器获取 Cookie
2. 确保复制完整的 Cookie 字符串
3. 检查站点账号是否正常

### RSS 订阅无法解析

1. 检查 RSS 链接是否正确
2. 确认 Cookie/API Key 有效
3. 查看日志获取详细错误信息

### 种子推送失败

1. 检查下载器连接状态
2. 确认下载目录存在且有写入权限
3. 检查磁盘空间是否充足

## 重置管理员密码

```bash
docker run -d \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='新密码' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

> 重置完成后移除 `PT_ADMIN_RESET` 环境变量重新启动容器。

## 从源码构建

### 依赖要求

- Go 1.22+
- Node.js 18+
- pnpm

### 构建步骤

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools

# 构建前端
cd web/frontend
pnpm install
pnpm build
cd ../..

# 构建后端
go build -o pt-tools .

# 或使用 Makefile（推荐）
make build-local
```

### 开发模式

```bash
# 启动前端开发服务器
cd web/frontend && pnpm dev

# 另一个终端启动后端
go run main.go web --port 8081
```

## 技术架构

| 组件 | 技术栈 |
|------|--------|
| 后端 | Go 1.22+ / Gin / GORM / SQLite |
| 前端 | Vue 3 / Element Plus / TypeScript / Vite |
| CLI | Cobra |
| 日志 | Zap |

### 性能优化

- **两级缓存**：内存缓存
- **熔断器**：自动检测和隔离故障站点
- **连接池**：HTTP 连接复用
- **限流**：防止请求过快触发站点限制
- **并发控制**：合理的并发数避免被站点封禁

## 贡献指南

欢迎贡献代码或提交问题！

- **提交 Issue**：[GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- **提交 PR**：[GitHub Pull Requests](https://github.com/sunerpy/pt-tools/pulls)

### 添加新站点支持

1. 在 `site/v2/definitions/` 目录下创建站点定义文件（参考 `hdsky.go`）
2. 实现站点的认证、搜索、用户信息等接口
3. 在 `init()` 函数中注册站点到 Registry
4. 编写测试用例
5. 更新 README 中的站点列表
6. 提交 PR

### 代码规范

```bash
# 运行代码检查
make lint

# 运行测试
make unit-test

# 格式化代码
make fmt
```

## 更新日志

查看 [Releases](https://github.com/sunerpy/pt-tools/releases) 获取完整更新日志。

## 许可证

[MIT License](LICENSE)

## Star History

如果这个项目对你有帮助，请给一个 Star 支持一下！

[![Star History Chart](https://api.star-history.com/svg?repos=sunerpy/pt-tools&type=Date)](https://star-history.com/#sunerpy/pt-tools&Date)

---

**免责声明**：本工具仅供学习和研究使用，请遵守各 PT 站点的规则，合理使用。
