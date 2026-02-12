# 配置说明

本文档详细介绍 pt-tools 的所有配置选项，包括环境变量、全局设置、下载器配置和 RSS 订阅配置。

[返回首页](../README.md)

## 目录

- [环境变量](#环境变量)
- [代理配置](#代理配置)
- [全局设置](#全局设置)
- [下载器配置](#下载器配置)
  - [基本配置](#基本配置)
  - [下载目录配置](#下载目录配置)
- [RSS 订阅配置](#rss-订阅配置)
- [数据持久化](#数据持久化)
- [配置示例](#配置示例)

## 环境变量

通过环境变量可以配置 pt-tools 的运行参数，适用于 Docker 部署场景。

| 变量             | 说明           | 默认值          | 示例               |
| ---------------- | -------------- | --------------- | ------------------ |
| `PT_HOST`        | Web 监听地址   | `0.0.0.0`       | `127.0.0.1`        |
| `PT_PORT`        | Web 监听端口   | `8080`          | `8888`             |
| `PT_ADMIN_USER`  | 管理员用户名   | `admin`         | `myadmin`          |
| `PT_ADMIN_PASS`  | 管理员密码     | `adminadmin`    | `MySecurePass123`  |
| `PT_ADMIN_RESET` | 重置管理员密码 | -               | `1` (启用)         |
| `PUID`           | 容器用户 ID    | `1000`          | `1001`             |
| `PGID`           | 容器组 ID      | `1000`          | `1001`             |
| `TZ`             | 时区           | `Asia/Shanghai` | `America/New_York` |

### 环境变量使用示例

**Docker 命令行**：

```bash
docker run -d \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS=your_password \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

**Docker Compose**：

```yaml
environment:
  PT_HOST: "0.0.0.0"
  PT_PORT: "8080"
  PT_ADMIN_USER: "admin"
  PT_ADMIN_PASS: "your_password"
  TZ: "Asia/Shanghai"
```

### 重置管理员密码

如果忘记密码，可以通过环境变量重置：

```bash
docker run -d \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='新密码' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

> 重置完成后，移除 `PT_ADMIN_RESET` 环境变量重新启动容器。

## 代理配置

pt-tools 支持通过标准环境变量配置网络代理，适用于 Docker、systemd 和本地二进制运行。

| 变量                          | 说明                           | 示例                       |
| ----------------------------- | ------------------------------ | -------------------------- |
| `HTTP_PROXY` / `http_proxy`   | HTTP 请求代理                  | `http://127.0.0.1:7890`    |
| `HTTPS_PROXY` / `https_proxy` | HTTPS 请求代理                 | `http://127.0.0.1:7890`    |
| `ALL_PROXY` / `all_proxy`     | 通用代理（作为回退）           | `socks5://127.0.0.1:1080`  |
| `NO_PROXY` / `no_proxy`       | 不走代理的地址列表（逗号分隔） | `localhost,127.0.0.1,.lan` |

说明：

- 建议同时设置 `HTTP_PROXY` 和 `HTTPS_PROXY`。
- 如果未设置 `HTTP_PROXY`/`HTTPS_PROXY`，会尝试使用 `ALL_PROXY`。
- `NO_PROXY` 对内网地址非常有用，可避免本地服务或局域网请求走代理。

### 代理配置示例

**Docker 命令行**：

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e HTTP_PROXY=http://127.0.0.1:7890 \
  -e HTTPS_PROXY=http://127.0.0.1:7890 \
  -e ALL_PROXY=socks5://127.0.0.1:1080 \
  -e NO_PROXY=localhost,127.0.0.1,.lan \
  sunerpy/pt-tools:latest
```

**Docker Compose**：

```yaml
environment:
  HTTP_PROXY: "http://127.0.0.1:7890"
  HTTPS_PROXY: "http://127.0.0.1:7890"
  ALL_PROXY: "socks5://127.0.0.1:1080"
  NO_PROXY: "localhost,127.0.0.1,.lan"
```

## 全局设置

在 Web 管理界面的「全局设置」页面可配置以下选项：

| 配置项               | 说明                             | 默认值   | 建议值           |
| -------------------- | -------------------------------- | -------- | ---------------- |
| **默认间隔(分钟)**   | RSS 任务的默认执行间隔           | 15       | 15-30 分钟       |
| **种子下载目录**     | 保存 `.torrent` 文件的目录       | 默认目录 | 根据需求设置     |
| **启用限速**         | 用于判断种子是否能在免费期内完成 | 关闭     | 按需开启         |
| **下载限速(MB/s)**   | 预估下载速度                     | -        | 根据实际带宽设置 |
| **最大种子大小(GB)** | 超过此大小的种子将被跳过         | 无限制   | 50-100 GB        |
| **自动启动任务**     | 程序启动时是否自动运行 RSS 任务  | 是       | 是               |

### 限速与免费期计算

启用限速后，pt-tools 会根据以下公式判断种子是否能在免费期内完成：

```
预计完成时间 = 种子大小 / 下载限速
如果 预计完成时间 > 免费剩余时间，则跳过该种子
```

## 下载器配置

### 基本配置

pt-tools 支持以下下载器：

| 下载器           | 支持版本 | 推荐程度 |
| ---------------- | -------- | -------- |
| **qBittorrent**  | 4.x+     | 推荐     |
| **Transmission** | 3.x+     | 支持     |

**配置字段说明**：

| 字段         | 说明                   | 示例                           |
| ------------ | ---------------------- | ------------------------------ |
| **名称**     | 下载器标识名称         | `家里qBit`、`NAS主力`          |
| **类型**     | 下载器类型             | `qBittorrent` / `Transmission` |
| **URL**      | WebUI 地址             | `http://192.168.1.10:8080`     |
| **用户名**   | 登录用户名             | `admin`                        |
| **密码**     | 登录密码               | `password`                     |
| **设为默认** | 是否作为默认下载器     | 是/否                          |
| **自动启动** | 推送任务后是否自动开始 | 是/否                          |

### 下载目录配置

每个下载器可配置多个下载目录：

| 字段         | 说明               | 示例                     |
| ------------ | ------------------ | ------------------------ |
| **路径**     | 下载目录的实际路径 | `/data/downloads/movies` |
| **别名**     | 目录的显示名称     | `电影`                   |
| **设为默认** | 是否为默认下载目录 | 是/否                    |

**注意事项**：

- 路径必须是下载器所在机器上的有效路径
- Docker 环境下需要确保路径已正确映射
- 别名方便在推送时快速选择

**目录配置示例**：

```
路径: /data/downloads/movies    别名: 电影     默认: 否
路径: /data/downloads/tv        别名: 剧集     默认: 是
路径: /data/downloads/anime     别名: 动漫     默认: 否
路径: /data/downloads/music     别名: 音乐     默认: 否
```

## RSS 订阅配置

RSS 订阅是 pt-tools 的核心功能，配置字段如下：

| 字段               | 说明                 | 是否必填 | 示例                       |
| ------------------ | -------------------- | -------- | -------------------------- |
| **名称**           | 订阅标识名称         | 是       | `HDSky电影`、`MT电视剧`    |
| **链接**           | RSS 订阅 URL         | 是       | `https://site.com/rss?...` |
| **分类**           | 下载器中的分类标签   | 否       | `PT-Auto`                  |
| **标签**           | 任务标签，便于管理   | 否       | `hdsky,movie`              |
| **间隔(分钟)**     | 执行间隔             | 是       | 5-1440 分钟                |
| **下载器**         | 指定下载器           | 否       | 不指定则用默认             |
| **下载路径**       | 指定下载目录         | 否       | 不指定则用默认             |
| **过滤规则**       | 关联的过滤规则       | 否       | 可多选                     |
| **免费结束时暂停** | 免费期结束时自动暂停 | 否       | 是/否                      |

### RSS 配置建议

| 场景           | 建议间隔   | 是否启用免费暂停 |
| -------------- | ---------- | ---------------- |
| **日常刷流**   | 15-30 分钟 | 是               |
| **追更剧集**   | 10-15 分钟 | 视情况           |
| **新资源抢种** | 5-10 分钟  | 否               |

详细的 RSS 配置指南请参考 [RSS 订阅配置指南](./guide/rss-subscription.md)。

## 数据持久化

pt-tools 的所有数据存储在数据目录中：

| 内容         | Docker 路径      | 本地路径      | 说明                 |
| ------------ | ---------------- | ------------- | -------------------- |
| **数据目录** | `/app/.pt-tools` | `~/.pt-tools` | 所有数据的根目录     |
| **数据库**   | `torrents.db`    | `torrents.db` | SQLite 数据库文件    |
| **种子文件** | `downloads/`     | `downloads/`  | 下载的 .torrent 文件 |
| **日志文件** | `logs/`          | `logs/`       | 运行日志             |

### 数据备份

**备份内容**：

- `torrents.db` - 包含所有配置和历史记录
- `.torrent` 文件（可选）

**备份命令**：

```bash
# Docker 环境
docker cp pt-tools:/app/.pt-tools/torrents.db ./backup/

# 本地环境
cp ~/.pt-tools/torrents.db ./backup/
```

**恢复方法**：

```bash
# Docker 环境
docker cp ./backup/torrents.db pt-tools:/app/.pt-tools/

# 本地环境
cp ./backup/torrents.db ~/.pt-tools/
```

## 配置示例

### 完整 Docker Compose 配置

```yaml
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      PT_ADMIN_USER: "admin"
      PT_ADMIN_PASS: "your_secure_password"
      TZ: "Asia/Shanghai"
      PUID: "1000"
      PGID: "1000"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
      - /path/to/downloads:/downloads # 可选：映射下载目录
    restart: unless-stopped
```

### 典型使用场景配置

**场景 1：纯刷流**

- RSS 间隔：15-30 分钟
- 只下载免费种子
- 启用免费结束暂停
- 设置最大种子大小限制

**场景 2：追剧 + 刷流**

- RSS 间隔：10 分钟
- 创建追剧过滤规则，允许非免费
- 创建通用规则，只下载免费
- 启用免费结束暂停

**场景 3：多站点管理**

- 每个站点单独配置 RSS
- 配置多个下载器（如家里、公司）
- 设置不同的下载目录
- 使用标签区分来源

---

相关文档：

- [RSS 订阅配置指南](./guide/rss-subscription.md)
- [获取认证信息指南](./guide/get-cookie-apikey.md)
- [过滤规则与追剧指南](./guide/filter-rules-tv-series.md)

[返回首页](../README.md)
