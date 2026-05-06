# 媒体刮削子系统指南

pt-tools 内置的媒体刮削子系统对标 tinyMediaManager，提供电影/剧集元数据自动抓取、NFO 写入、Jellyfin/Emby 库刷新等功能。支持：

- 📡 **三源融合**：TMDB + 豆瓣 + LLM（离线）
- 🎬 **全类型**：Movie / TvShow / Season / Episode
- 🔌 **Media Server**：Jellyfin / Emby（自动识别）
- 🤖 **LLM-Native**：10+ provider (OpenAI, Kimi, GLM, Qwen, DeepSeek, Doubao, Ollama 等)
- 🔗 **MCP Server**：暴露 17 个 tools 供 Claude / agent 调用
- 🌐 **代理**：每个数据源独立代理配置

## 部署方式

### 1. 嵌入部署（pt-tools web 命令）

scraper 已作为 `/api/v2/scraper/*` 嵌入到 pt-tools web 服务。直接启动 pt-tools 即可访问：

```bash
pt-tools web --host 0.0.0.0 --port 8080
```

访问 `http://localhost:8080/scraper` 即可打开刮削 UI。

### 2. 独立部署（pt-scraper 二进制）

适合不需要完整 pt-tools 功能、或想独立运行 scraper 的场景。

#### Docker 部署（推荐）

```bash
# 构建
make build-scraper-docker

# 或手动
docker build -f build/Dockerfile.scraper -t sunerpy/pt-scraper:dev .

# 运行
docker run -d \
  --name pt-scraper \
  -p 8090:8090 \
  -p 8091:8091 \
  -v /path/to/media:/media:ro \
  -v pt-scraper-data:/data \
  sunerpy/pt-scraper:dev

# 查看首次启动生成的 API Key（后续请求需要）
docker logs pt-scraper | grep "API Key"
```

#### 二进制部署

```bash
# 编译
CGO_ENABLED=0 go build -o bin/pt-scraper ./cmd/pt-scraper

# 运行
./bin/pt-scraper --mode http --port 8090 --data-dir ~/.pt-scraper
```

## 运行模式（--mode）

| 模式           | 说明                                                     |
| -------------- | -------------------------------------------------------- |
| `http`（默认） | HTTP REST API，供前端 Web UI 或外部系统调用              |
| `mcp-stdio`    | MCP stdio 传输，给 Claude Desktop 等本地 LLM client 使用 |
| `mcp-http`     | MCP Streamable HTTP，供远程 agent 通过网络调用           |
| `both`         | 同时启动 HTTP REST + MCP HTTP（独立端口）                |

### Claude Desktop 接入示例

在 Claude Desktop 配置文件中添加：

```json
{
  "mcpServers": {
    "pt-scraper": {
      "command": "/usr/local/bin/pt-scraper",
      "args": ["--mode", "mcp-stdio", "--data-dir", "/home/user/.pt-scraper"]
    }
  }
}
```

## TMDB API Key 获取

1. 访问 <https://www.themoviedb.org/settings/api>
2. 注册账号 → 申请 API Key（选 v4 Bearer Token）
3. 在 pt-tools Web UI → `/scraper/settings` → Providers tab 填入

**注意**：国内需配置代理访问 api.themoviedb.org，在同一设置页面可填代理 URL。

## LLM Provider 配置

Settings → LLM tab 支持 10+ provider，选择预设后自动填 Base URL：

| Provider        | Base URL                                            | 特点                |
| --------------- | --------------------------------------------------- | ------------------- |
| OpenAI          | `https://api.openai.com/v1`                         | 权威，需代理        |
| Kimi (月之暗面) | `https://api.moonshot.cn/v1`                        | 国产，128k 长上下文 |
| 智谱 GLM        | `https://open.bigmodel.cn/api/paas/v4`              | 国产                |
| 通义 Qwen       | `https://dashscope.aliyuncs.com/compatible-mode/v1` | 国产                |
| DeepSeek        | `https://api.deepseek.com/v1`                       | 高性价比            |
| 字节豆包        | `https://ark.cn-beijing.volces.com/api/v3`          | 国产                |
| Groq            | `https://api.groq.com/openai/v1`                    | 超快推理            |
| **Ollama 本地** | `http://localhost:11434/v1`                         | 完全离线            |

推荐用 **qwen2.5:7b** (Ollama 本地) 作为入门：

```bash
ollama pull qwen2.5:7b
```

## Jellyfin/Emby 接入

1. 在 Jellyfin → Dashboard → API Keys 创建 key
2. Settings → Connectors tab → Add Server：
   - URL：`http://jellyfin:8096`
   - API Key：粘贴
   - Type：auto（自动识别 Jellyfin/Emby）
3. 点击 Test Connection 验证

刮削完成后会自动触发对应库刷新（POST `/Items/{id}/Refresh`）。

## 常见问题

### Q: 豆瓣限流怎么办？

A: 豆瓣 Frodo API 有软限流（~40 req/min）。scraper 内置 1-3s 随机间隔 + 多 User-Agent 轮换。触发限流时：

- 等待 15-30 分钟
- 仅用 TMDB 源（暂时禁用豆瓣）
- 或切到 LLM-Native 模式（离线）

### Q: LLM 会不会幻觉错误的 TMDB ID？

A: scraper 内置反幻觉防护：

- LLM 生成的 `tmdb_id` 会调真实 TMDB API 交叉验证 title/year，不匹配则清空
- URL 字段（poster/fanart）**绝不**从 LLM 填充
- `temperature=0` 强制
- Year 范围 1900-当前年 验证
- IMDB ID 格式 `tt\d{7,9}` 正则验证

### Q: 中文刮削效果差？

A: 默认策略：`Title 中文` Douban 优先，TMDB(zh-CN) 次之，en-US fallback。如果豆瓣和 TMDB 都不可用，试试 LLM 提供中文上下文：在「AI 刮削」面板粘贴该片的剧情简介。

### Q: NFO 兼容性？

A: scraper 默认写 **Universal NFO** 方言：

- Kodi base 格式（字段顺序严格）
- Jellyfin 扩展（`<collectionnumber>`）
- Emby 扩展（`<set tmdbcolid="">`）
- 三端通吃（Kodi/Jellyfin/Emby）

如需严格 Kodi-only，在库设置中选 `NFO 格式: Kodi`。

### Q: 如何启用自动刮削？

A: 编辑库 → 打开「自动刮削」开关。会订阅 pt-tools 的 TorrentCompleted 事件，种子下载完成后自动触发刮削。

## MCP Tools 清单

| Tool                                  | 功能                      |
| ------------------------------------- | ------------------------- |
| `scraper_search_media`                | 跨源搜索电影/剧集         |
| `scraper_get_metadata`                | 按 TMDB ID 获取完整元数据 |
| `scraper_scrape_file`                 | 刮削单个媒体文件          |
| `scraper_scrape_directory`            | 批量刮削整个目录          |
| `scraper_list_libraries`              | 列出媒体库                |
| `scraper_get_task_status`             | 查任务进度                |
| `scraper_list_tasks`                  | 列任务                    |
| `scraper_refresh_jellyfin`            | 触发 Jellyfin/Emby 刷新   |
| `scraper_get_artworks`                | 候选艺术图列表            |
| `scraper_write_nfo`                   | 手动写 NFO                |
| `scraper_parse_filename`              | 解析文件名为结构化数据    |
| `scraper_match_media`                 | 手动匹配 TMDB ID          |
| `scraper_scrape_with_llm`             | LLM 离线刮削              |
| `scraper_generate_metadata_from_text` | 文本 → NFO（LLM）         |
| `scraper_validate_metadata`           | LLM 验证元数据反幻觉      |
| `scraper_enrich_partial`              | 补全部分字段              |
| `scraper_propose_match`               | LLM 提议候选匹配          |

## 贡献与反馈

GitHub: <https://github.com/sunerpy/pt-tools>
Issues: <https://github.com/sunerpy/pt-tools/issues>
