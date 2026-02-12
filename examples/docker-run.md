# 使用 Docker 运行 pt-tools

## 功能特性

- RSS 自动订阅下载免费种子
- 多站点种子搜索（参见[支持站点列表](../docs/sites.md)）
- 用户信息统计和等级进度追踪
- 支持 qBittorrent 和 Transmission 下载器
- 过滤规则精细化筛选

## 快速启动

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

访问 `http://localhost:8080` 进入 Web 管理界面。

**默认登录账号**：`admin` / `adminadmin`

## 推荐配置（数据持久化）

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  -e TZ=Asia/Shanghai \
  -e PUID=1000 \
  -e PGID=1000 \
  sunerpy/pt-tools:latest
```

## 数据目录说明

挂载 `/app/.pt-tools` 目录以持久化所有数据：

```
~/pt-data/
├── torrents.db      # SQLite 数据库（配置、任务记录、用户信息缓存）
└── downloads/       # 种子文件下载目录
```

## 环境变量

| 变量             | 说明                 | 默认值          |
| ---------------- | -------------------- | --------------- |
| `PT_HOST`        | 监听地址             | `0.0.0.0`       |
| `PT_PORT`        | 监听端口             | `8080`          |
| `PT_ADMIN_USER`  | 管理员用户名         | `admin`         |
| `PT_ADMIN_PASS`  | 管理员密码           | `adminadmin`    |
| `PT_ADMIN_RESET` | 重置密码（设为 `1`） | -               |
| `PUID`           | 用户 ID              | `1000`          |
| `PGID`           | 组 ID                | `1000`          |
| `TZ`             | 时区                 | `Asia/Shanghai` |
| `HTTP_PROXY`     | HTTP 请求代理        | -               |
| `HTTPS_PROXY`    | HTTPS 请求代理       | -               |
| `ALL_PROXY`      | 通用代理（回退）     | -               |
| `NO_PROXY`       | 不走代理地址列表     | -               |

### 代理配置示例

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

## 重置管理员密码

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='新密码' \
  sunerpy/pt-tools:latest
```

重置完成后，停止容器并移除 `PT_ADMIN_RESET` 环境变量重新启动：

```bash
docker stop pt-tools && docker rm pt-tools

docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

## Web 界面功能

1. **用户信息仪表盘**：查看所有站点的上传量、下载量、魔力值、等级进度等
2. **种子搜索**：跨站点搜索种子，支持批量下载和推送到下载器
3. **下载器管理**：配置 qBittorrent/Transmission，设置下载目录
4. **站点管理**：配置站点认证信息和 RSS 订阅
5. **过滤规则**：对 RSS 进行精细化筛选
6. **任务列表**：查看所有下载任务记录

## 内置支持站点

参见[支持站点列表](../docs/sites.md)。
