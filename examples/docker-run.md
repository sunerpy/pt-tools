# 使用 Docker 运行 pt-tools（Web 默认）

- 默认环境变量：`PT_WEB=1`、`PT_HOST=0.0.0.0`、`PT_PORT=8080`
- 首次运行无需提供任何配置文件，进入 Web 页面完成初始化。

## 直接运行

```bash
docker run -dit \
  -p 8080:8080 \
  --name pt-tools \
  sunerpy/pt-tools:latest
```

访问 `http://localhost:8080/` 进行 Web 初始化。

## 指定数据持久化目录

推荐将数据目录挂载到本机（数据库与下载目录均位于其中）。

```bash
docker run -d \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  --name pt-tools \
  sunerpy/pt-tools:latest
```

> 数据库存放在 `~/.pt-tools/torrents.db`（容器内 `/app/.pt-tools/torrents.db`）；下载目录默认位于 `~/.pt-tools/downloads`（容器内 `/app/.pt-tools/downloads`）。统一挂载 `/app/.pt-tools` 可持久化所有数据。

容器默认启动 Web 管理界面，无需 `config.toml`。