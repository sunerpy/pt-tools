# 媒体刮削子系统指南

pt-tools 内置的媒体刮削子系统对标 tinyMediaManager，为电影 / 剧集 / 单集提供
自动元数据抓取、NFO 文件生成、Jellyfin / Emby 库刷新等能力。

## 功能速览

- 📡 **多源融合**：TMDB + 豆瓣 + IMDb + LLM（离线）四类数据源，字段级优先级合并
- 🎬 **全类型**：Movie / TvShow / Season / Episode
- 🧾 **NFO 四方言**：`kodi` / `jellyfin` / `emby` / `universal`，字节级兼容
- 🔌 **Media Server**：Jellyfin / Emby，支持 ProductName 自动识别
- 🤖 **LLM-Native**：10 个 provider（OpenAI / Kimi / GLM / Qwen / DeepSeek /
  Doubao / Yi / Baichuan / Groq / Ollama）
- 🔗 **MCP Server**：17 个 tools + 3 个 resource + 3 个 prompt，供 Claude /
  agent 调用
- 🌐 **per-provider 代理**：每个数据源可单独设置 HTTP / SOCKS5 代理
- ♻️ **热加载**：Web UI 保存凭证后立刻生效，无需重启

> 本文档覆盖：**1) 工作原理** · **2) 部署与启用** · **3) 配置参考**（provider /
> library / connector / LLM） · **4) REST API + DB schema** · **5) MCP 集成** ·
> **6) 已知限制** · **7) FAQ**。

---

## 目录

- [架构与工作原理](#架构与工作原理)
- [部署](#部署)
- [数据源配置](#数据源配置)
- [媒体库（Library）配置](#媒体库library配置)
- [Jellyfin / Emby 接入](#jellyfin--emby-接入)
- [代理配置](#代理配置)
- [构建期 API Key 注入（可选）](#构建期-api-key-注入可选)
- [REST API 参考](#rest-api-参考)
- [数据库表结构](#数据库表结构)
- [MCP Server 集成](#mcp-server-集成)
- [已知限制与路线图](#已知限制与路线图)
- [FAQ](#faq)

---

## 架构与工作原理

### 整体架构

刮削子系统位于 `internal/scraper/`，由 pt-tools web 主服务通过
`web/server_scraper.go` 挂载到 `/api/v2/scraper/*`（嵌入模式）。

```
┌──────────────────────────────────────────────────────────────────┐
│  pt-tools web server  (global DB + session auth)                 │
│                                                                  │
│  /api/v2/scraper/*  →  scraper/web/api.go                        │
│                         ├─ HandleScrape        → 入队             │
│                         ├─ HandleSetProviderCredential → 热加载   │
│                         └─ (22 个 endpoint，见 REST API 参考)     │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ bootstrap.ProviderManager  (启动时 + 凭证保存后热加载)    │    │
│  │   → sourceReg: tmdb / douban / imdb                       │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ service.ScrapeService  +  PersistentQueue (workers=3)     │    │
│  │   stage: searching → fetching → fusing →                  │    │
│  │          writing_nfo → downloading_art →                  │    │
│  │          refreshing_server → done                         │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  writerReg: kodi / jellyfin / emby / universal                   │
│  connector: Jellyfin / Emby  (per-config 构造，不走 registry)    │
└──────────────────────────────────────────────────────────────────┘
```

### 刮削流程

用户点 "扫描" 按钮到 NFO 落盘的完整调用链：

1. **入队** — 前端 `POST /api/v2/scraper/scrape` → `PersistentQueue.Enqueue`
   - 插入 `scrape_tasks` 行（`state=pending`）
   - 立即返回 `202 Accepted`
2. **worker 拾取** — 3 个常驻 worker 从内存 channel 取任务
   - `UPDATE scrape_tasks SET state='running'`
3. **解析**（可选） — `stage=parsing`，`filename.go` 从文件名抽取 title/year
4. **搜索** — `stage=searching`，遍历 library 的 `provider_ids`（例如
   `tmdb,douban`）
   - 每个 provider 调用 `SearchMovie` / `SearchTvShow` / `SearchEpisode`
   - 收集成功的候选到 `rawResults`
5. **详情获取** — `stage=fetching`，对每个候选调用 `GetMovieMetadata` 等
6. **融合** — `stage=fusing`，`service.DefaultFuser.Merge` 按字段优先级合并多源
7. **写 NFO** — `stage=writing_nfo`，从 `writerReg.Get(dialect)` 拿到 writer，
   写入 `{basename}.nfo` + `movie.nfo`（电影）/ `tvshow.nfo`（剧集）
8. **下载艺术图** — `stage=downloading_art`，并发 3，FSCache 去重；
   写入 `poster.jpg`、`fanart.jpg` 等
9. **刷新媒体服务器** — `stage=refreshing_server`（仅当库配了 `connector_id`），
   调用 Jellyfin/Emby 的 `/Library/Refresh`
10. **完成** — `UPDATE scrape_tasks SET state='success', progress=100`，
    插入 `scrape_results` 行（包含融合后的 JSON）

> 🎯 **关键：扫描 ≠ 目录遍历**。当前的"扫描"按钮入队的是 **library.path 作为
> 单个 media_path 的刮削任务**，不会递归 walk 目录。批量扫描整个文件夹的
> Scanner 已在代码里（`service/scanner.go`）但**尚未接线到 runtime**，见
> [已知限制](#已知限制与路线图)。

### 任务状态机

`scrape_tasks` 表有两条正交的轴：

- **`state`**：`pending` → `running` →（`success` | `failed` | `retrying` |
  `canceled`）
- **`current_stage`**：`parsing` → `searching` → `fetching` → `fusing` →
  `writing_nfo` → `downloading_art` → `refreshing_server` → `done`

一旦任务失败，`state` 变为 `failed` 或 `retrying`，`current_stage` 记录失败发生
的那一步，`last_error` 记录错误详情，`retry_count` 递增。

### 失败分类

错误通过 `core.IsPermanent(err)` 分类为两类：

- **永久错误**（不重试，直接 `failed`）：
  - `ErrPermanent`（显式标记）
  - `ErrUnauthorized` — 凭证错或过期
  - `ErrNotFound` — 资源不存在
  - `ErrInvalidID` — 请求参数非法
  - `ErrUnsupported` — 功能未实现
- **瞬时错误**（进入 `retrying`，指数退避）：
  - 网络超时 / 5xx / 限流
  - Backoff 时间：第 1 次 `5s`，第 2 次 `30s`，第 3 次及以上 `3min`
  - 超过 `MaxRetries`（默认 3）后降级为 `failed`

这是为什么"TMDB 未配置"的任务会**立刻** failed（永久）而"豆瓣 API 偶发 500"
会重试 3 次。

---

## 部署

### 嵌入模式（推荐，当前仅此模式可用）

无需额外操作。启动 pt-tools 即自动挂载 `/api/v2/scraper/*` + `/scraper` UI：

```bash
pt-tools web --host 0.0.0.0 --port 8080
```

登录后访问 `http://localhost:8080/#/scraper` 打开 UI。

### 独立模式（规划中）

历史版本曾描述 `pt-scraper` 独立二进制（`--mode http|mcp-stdio|mcp-http|both`）。
**截至当前版本，`cmd/pt-scraper/` 源码尚未合入主干**，Makefile 中的
`build-scraper-*` 目标会构建失败。独立模式作为未来特性保留（见
[路线图](#已知限制与路线图)）。

---

## 数据源配置

4 类数据源：**TMDB**（API）、**豆瓣**（零配置）、**IMDb**（零配置）、**LLM**
（10 个 provider）。

所有数据源配置存于 `provider_credentials` 表，可通过 UI「刮削设置 → 数据源」
页编辑。`api_key` / `bearer_token` 字段带 `json:"-"` 标记，不会回显到前端。

### TMDB

**需要用户自备 API Token**（免费申请，详见
<https://www.themoviedb.org/settings/api>）。

运行时凭证优先级（高到低）：

1. DB 行 `provider_credentials.bearer_token` / `.api_key`（UI 配置）
2. 环境变量 `PT_SCRAPER_TMDB_BEARER` / `PT_SCRAPER_TMDB_APIKEY`
3. 构建期 ldflags 注入的 `buildkeys.TmdbBearerToken` / `.TmdbApiKey`（详见
   [构建期 API Key 注入](#构建期-api-key-注入可选)）

UI 字段：

| 字段         | 说明                                   |
| ------------ | -------------------------------------- |
| Bearer Token | v4 Token（推荐），形如 `eyJhbGciOi...` |
| 代理 URL     | 可选，TMDB 被墙时通过代理访问          |

填完点「保存 TMDB 配置」立即生效；「测试凭证」按钮会调用 `/3/configuration`
验证 Token 有效性。

### 豆瓣

**零配置**。pt-tools 内置了豆瓣 Android 客户端的 Frodo App Key + 请求签名算法，
另外实现了 HTML 网页降级爬取。用户不需要提供任何凭证。

可选：通过「代理 URL」绕过数据中心 IP 封禁。

> 豆瓣在直连国内网络时完全可用。从海外数据中心 IP 访问会被 302 到
> `sec.douban.com`，此时填代理即可。

### IMDb

**零配置**。pt-tools 直接抓取 IMDb HTML + JSON-LD 结构化数据，另用 IMDb
autocomplete JSON 接口（`v3.sg.media-imdb.com/suggestion/`）做搜索。

可选：通过「代理 URL」绕过 AWS WAF challenge（数据中心 IP 常见）。

### LLM Provider

10 个内置 preset（见 `internal/scraper/source/llm/presets.go`）：

| Preset     | 描述              | 默认 BaseURL                                        | Strict Schema |
| ---------- | ----------------- | --------------------------------------------------- | :-----------: |
| `openai`   | OpenAI 官方       | `https://api.openai.com/v1`                         |      ✅       |
| `kimi`     | 月之暗面 Moonshot | `https://api.moonshot.cn/v1`                        |      ❌       |
| `glm`      | 智谱 GLM          | `https://open.bigmodel.cn/api/paas/v4`              |      ❌       |
| `qwen`     | 通义 Qwen         | `https://dashscope.aliyuncs.com/compatible-mode/v1` |      ❌       |
| `deepseek` | DeepSeek          | `https://api.deepseek.com`                          |      ❌       |
| `doubao`   | 字节豆包          | `https://ark.cn-beijing.volces.com/api/v3`          |      ❌       |
| `yi`       | 零一万物 Yi       | `https://api.lingyiwanwu.com/v1`                    |      ❌       |
| `baichuan` | 百川              | `https://api.baichuan-ai.com/v1`                    |      ❌       |
| `groq`     | Groq              | `https://api.groq.com/openai/v1`                    |      ✅       |
| `ollama`   | 本地 Ollama       | `http://localhost:11434`                            |      ❌       |

UI 在「刮削设置 → LLM」tab 里选 preset，自动填入 BaseURL，用户再填
`API Key` 和具体模型名（如 `qwen2.5:7b`）。

---

## 媒体库（Library）配置

存于 `media_library_configs` 表。一个 library 对应一个目录 + 一组刮削规则。

UI「刮削 → 媒体库」新增时填：

| 字段           | 含义                                                                     |
| -------------- | ------------------------------------------------------------------------ |
| `name`         | 显示名（必填，唯一）                                                     |
| `type`         | `movie` / `tv` / `mixed`                                                 |
| `path`         | 绝对路径，如 `/media/movies`                                             |
| `provider_ids` | 逗号分隔，按顺序试：`tmdb,douban,imdb,llm` 任选                          |
| `nfo_dialect`  | `kodi` / `jellyfin` / `emby` / `universal`（默认 `universal`）           |
| `connector_id` | 关联的 Jellyfin/Emby（可选）                                             |
| `auto_scrape`  | 订阅 `TorrentCompleted` 事件。**当前版本事件桥未布线，该字段暂无效果**。 |

扫描时机：

- 手动：在「媒体库」页点 "扫描"（每点一次入队一个任务）
- 详情页：「扫描」/「手动匹配」/「AI 刮削」三个入口
- ~~Cron~~：`scan_cron` 字段存在但 Scheduler 未接线（[路线图](#已知限制与路线图)）

---

## Jellyfin / Emby 接入

存于 `connector_configs` 表。支持 Jellyfin / Emby，通过 ProductName 自动识别。

UI「刮削设置 → 媒体服务器」新增：

| 字段    | 说明                                                  |
| ------- | ----------------------------------------------------- |
| 名称    | 显示名                                                |
| URL     | 如 `http://192.168.1.10:8096`                         |
| API Key | Jellyfin / Emby 管理界面 → API Keys 生成              |
| 类型    | `auto`（默认，ProductName 嗅探）/ `jellyfin` / `emby` |

保存后自动 Ping 一次，`last_ping_ok=true` 表示连通。点「测试」按钮可手动复测。

刮削任务完成后，若 library 配了 `connector_id`，会自动调用
`POST /Library/Refresh` 通知媒体服务器重新扫描。

---

## 代理配置

支持四种 scheme：`http` / `https` / `socks5` / `socks5h`。

优先级：**provider 级 `ProxyURL`（DB 字段）** > `HTTPS_PROXY` / `HTTP_PROXY`
环境变量。

详细配置示例（Tailscale / Webshare / 自建 SOCKS5）见
[docs/configuration.md → 媒体刮削 Provider 独立代理](../configuration.md#媒体刮削-provider-独立代理)。

---

## 构建期 API Key 注入（可选）

仅供**私有发行 / 内部团队分发**。Makefile 提供 `BUILDKEYS_LDFLAGS`：

```bash
export TMDB_BEARER_TOKEN="eyJhbGciOi..."
make build-local
```

ldflags 会把这些值写入 `buildkeys/buildkeys.go` 中的变量，作为 DB + env 都
未配置时的最终 fallback。

⚠️ **不要在公开发布的 OSS 二进制里注入**：

- 任何用户可通过 `strings ./pt-tools | grep` 提取 key
- 所有用户共享同一个 key，一旦其中一人滥用，TMDB 会封 key，全部用户全挂
- 正确做法：让用户通过 UI BYOK（每人独立配额）

详见 `internal/scraper/bootstrap/buildkeys/buildkeys.go` 的安全警告。

---

## REST API 参考

所有接口都在 `/api/v2/scraper/*` 前缀下，复用 pt-tools session 认证（需先登录）。

### Library

| 方法     | 路径              | 说明             |
| -------- | ----------------- | ---------------- |
| `GET`    | `/libraries`      | 列出所有库       |
| `POST`   | `/libraries`      | 创建库           |
| `GET`    | `/libraries/{id}` | 详情             |
| `PUT`    | `/libraries/{id}` | 更新             |
| `DELETE` | `/libraries/{id}` | 删除（级联任务） |

### Scrape / Task

| 方法     | 路径          | 说明                          |
| -------- | ------------- | ----------------------------- |
| `POST`   | `/scrape`     | 触发刮削任务（单媒体）        |
| `GET`    | `/tasks`      | 查任务列表（支持 state 过滤） |
| `GET`    | `/tasks/{id}` | 任务详情                      |
| `DELETE` | `/tasks/{id}` | 取消 / 删除任务               |

### Provider credential

| 方法   | 路径                            | 说明                       |
| ------ | ------------------------------- | -------------------------- |
| `GET`  | `/providers`                    | 列出已保存的 provider 凭证 |
| `POST` | `/providers/{name}/credentials` | 保存 / 更新（触发热加载）  |
| `POST` | `/providers/{name}/test`        | 测试凭证有效性             |

### Connector

| 方法   | 路径                    | 说明               |
| ------ | ----------------------- | ------------------ |
| `GET`  | `/connectors`           | 列出所有 connector |
| `POST` | `/connectors/{id}/test` | 连通性测试         |

### LLM / Search / Artwork

| 方法   | 路径             | 说明                      |
| ------ | ---------------- | ------------------------- |
| `GET`  | `/llm/providers` | LLM preset 列表           |
| `POST` | `/llm/generate`  | LLM 从文本生成 NFO        |
| `POST` | `/llm/validate`  | LLM 验证元数据            |
| `POST` | `/search`        | 跨源搜索候选              |
| `GET`  | `/artworks`      | 艺术图候选                |
| `GET`  | `/settings`      | 取全局设置（general tab） |

> `POST /scrape` body 字段：`type` (`movie` / `tv` / `episode`)、`media_path`、
> `title`、`year`、`providers`、`nfo_dialect`、`connector_id`、`overwrite_nfo`、
> `library_id`。`library_id` 可选，但填了会继承库的 provider / dialect 默认值。

---

## 数据库表结构

独立 schema 版本管理：`scraper_schema_versions`（当前版本 1），与 pt-tools 主
schema 版本解耦，便于后续独立迁移。

| 表                      | 主要字段                                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------------------ |
| `media_library_configs` | `name`(唯一) / `type` / `path` / `provider_ids` / `nfo_dialect` / `connector_id` / `auto_scrape` |
| `provider_credentials`  | `provider`(唯一) / `api_key` / `bearer_token` / `base_url` / `model_name` / `proxy_url`          |
| `connector_configs`     | `name`(唯一) / `type` / `base_url` / `api_key` / `last_ping_ok`                                  |
| `scrape_tasks`          | `library_id` / `task_type` / `media_path` / `state` / `current_stage` / `retry_count`            |
| `scrape_results`        | `task_id` / `media_type` / `title` / `year` / `nfo_path` / `poster_path` / `unified_data`        |
| `scraper_overrides`     | 用户手动覆盖字段（按 `result_id` + `field_name` 唯一）                                           |

> 凭证字段（`api_key` / `bearer_token` / connector `api_key`）在 JSON 序列化
> 时被 `json:"-"` 忽略，不会回显到前端。

---

## MCP Server 集成

pt-tools 刮削子系统可作为 MCP Server 暴露给支持 MCP 的 agent（Claude Desktop、
Cursor、Cline 等）。

**17 个 Tools** — 覆盖刮削任务触发、查询、LLM 调用、连通性测试等：

| Tool                      | 作用           |
| ------------------------- | -------------- |
| `scraper_list_libraries`  | 列出所有媒体库 |
| `scraper_get_library`     | 查单库详情     |
| `scraper_create_library`  | 新建库         |
| `scraper_update_library`  | 更新库         |
| `scraper_delete_library`  | 删除库         |
| `scraper_scrape_movie`    | 触发电影刮削   |
| `scraper_scrape_tv_show`  | 触发剧集刮削   |
| `scraper_scrape_episode`  | 触发单集刮削   |
| `scraper_get_task`        | 查任务进度     |
| `scraper_list_tasks`      | 列任务         |
| `scraper_cancel_task`     | 取消任务       |
| `scraper_search_metadata` | 跨源搜索候选   |
| `scraper_get_artworks`    | 拉艺术图候选   |
| `scraper_list_providers`  | 列数据源凭证   |
| `scraper_test_provider`   | Ping provider  |
| `scraper_llm_generate`    | LLM 生成 NFO   |
| `scraper_llm_validate`    | LLM 验证字段   |

**3 个 Resources**（URI）：

- `scraper://libraries`
- `scraper://tasks/recent`
- `scraper://providers`

**3 个 Prompts**：电影刮削工作流、目录批量刮削、LLM 使用说明。

> MCP 独立二进制（`pt-scraper --mode mcp-stdio`）尚未合入；当前可通过嵌入
> 模式直接走 REST API 驱动（MCP 协议封装在 `internal/scraper/mcp/`）。

---

## 已知限制与路线图

> 以下内容如实标注当前 MVP 阶段的缺陷/未实现。**不要**以为在文档里就代表
> 能用。

### ⚠️ 当前限制

1. **扫描按钮 ≠ 目录遍历** — "扫描"入队的是 `media_path = library.path`
   的单个任务，不会 walk 子目录。批量扫描逻辑在 `service/scanner.go` 里
   （含 fsnotify + 定期 walk），但未挂入 runtime。当前需要用户**逐文件触发**
   或通过 API 批量入队。
2. **`auto_scrape` 字段无效** — 依赖 `TorrentCompleted` 事件总线订阅，该
   桥接（`service/event_bridge.go`）代码存在但未在 `server_scraper.go` 中
   布线。下载完成不会自动触发刮削。
3. **`scan_cron` 字段无效** — 没有 scheduler 订阅该字段。
4. **`pt-scraper` 独立二进制缺失** — `cmd/pt-scraper/` 源码不在当前分支；
   `build/Dockerfile.scraper` 和 Makefile 的 `build-scraper-*` 目标会失败。
   独立部署**仅规划中**。
5. **`PUT /api/v2/scraper/settings`** 当前路由到 `HandleGetSettings`（只读），
   通用设置的持久化未完成。
6. **全库刷新** — 刮削完成后调用 `connector.RefreshLibrary(ctx, "")`，空字符串
   = 刷新 Jellyfin/Emby 所有库，非该库的精细刷新。大媒体服务器上会造成额外
   负载。
7. **Movie NFO 双写入** — 电影会写 `{basename}.nfo` + `movie.nfo` 两份；
   若用户手改过 `movie.nfo`，默认 `overwrite_nfo=false` 会让整次刮削在
   writing_nfo 阶段失败。需手动勾选"覆盖 NFO"。
8. **Worker 池无优雅停止** — `queue.Start(context.Background(), ...)` 使用
   detached context，pt-tools 主服务关闭时 worker 不退出（进程退出时会被
   OS 强杀）。

### 🗺️ 路线图（优先级降序）

- **v2-1**：打通 Scanner，实现目录递归扫描 + 新文件自动入队
- **v2-2**：打通 EventBridge，`auto_scrape=true` 时下载完成自动刮削
- **v2-3**：拆分 `cmd/pt-scraper` 独立二进制 + Docker 支持
- **v2-4**：实现 `PUT /settings` 完整读写 + 通用参数生效（scan_interval、
  worker concurrency、cache size）
- **v3-1**：精细库刷新（`RefreshLibrary(ctx, jellyfinLibraryID)`）
- **v3-2**：优雅停机钩子

---

## FAQ

**Q1：豆瓣会被限流 / 封号吗？**

A：已实现三层防御：(a) 请求间随机 1-3 s 间隔；(b) 4 个 Android Frodo UA
轮换；(c) Frodo 失败自动降级到 HTML 网页（另用桌面浏览器 UA 池 + `bid`
cookie + Referer 规避 `sec.douban.com` 反爬）。生产默认 `rateLimit=2s`，
低频使用几乎不会被限。若从海外数据中心访问建议配代理（详见代理章节）。

**Q2：LLM 生成的元数据可靠吗？有没有反幻觉？**

A：4 层反幻觉措施：

- `temperature=0` 降低发散
- `max_tokens` 限制输出长度
- 结构化输出（`response_format=json_schema`，openai / groq 启用 strict）
- 服务端二次校验：年份范围 1900–current_year+1、IMDb ID 正则
  `tt\d{7,10}`、URL 字段剔除、空字符串 reject

即便如此，**仅做"兜底"数据源**，优先从 TMDB / 豆瓣 / IMDb 拉真实元数据。

**Q3：哪种 NFO 方言最好？**

A：

- **Kodi / Kodi 派生播放器**（Plex 除外）：选 `kodi`
- **Jellyfin 专用**：选 `jellyfin`（含 Jellyfin 独有标签）
- **Emby 专用**：选 `emby`
- **跨媒体服务器/通用**：选 `universal`（默认）—— Kodi 基础 +
  Jellyfin `<collectionnumber>` + Emby `<set tmdbcolid>` 合并

**Q4：如何让 pt-tools 自动刮削下载完成的种子？**

A：当前版本 `auto_scrape=true` 字段**无效果**（EventBridge 未布线，见
已知限制 #2）。替代方案：

- 下载完成后在「刮削 → 媒体库」页手动点「扫描」
- 写脚本轮询下载器完成列表，调用 `POST /api/v2/scraper/scrape` 入队

**Q5：中文片怎么刮削更准？**

A：

- `provider_ids` 设为 `douban,tmdb` —— 豆瓣优先
- TMDB 设 `Language=zh-CN`（代码默认值）
- 豆瓣提供中文标题 + 中文剧情 + 中文演员名；Fuser 会优先用豆瓣字段覆盖
  TMDB 英文值
- 英文原名从 TMDB `OriginalTitle` 获取

**Q6：NFO 能跨 Kodi / Jellyfin / Emby 通用吗？**

A：选 `universal` 方言即可。对标 tinyMediaManager 的"所有平台兼容"输出，
字节级匹配 TMM 参考 XML（测试里有 golden fixture）。

**Q7：代理配错了会怎样？**

A：

- **TMDB**：`NewTMDBScraper` 会返回错误，bootstrap 记录 WARN，tmdb 不被
  注册，其他源仍工作
- **豆瓣 / IMDb**：非法代理 URL 静默回退到默认 http.Client（无代理），
  记录 WARN；生产中豆瓣仍然直连可用（国内）

---

## 下一步

- 想添加新的数据源？参考 `internal/scraper/source/tmdb/` 模板，实现
  `core.MediaScraper` + `MovieMetadataScraper` + `TvShowMetadataScraper`，
  在 `bootstrap/providers.go` 的 `Reload` 方法里注册。
- 想添加新的 NFO 方言？参考 `internal/scraper/writer/nfo/universal.go`，
  实现 `core.NfoWriter`，在 `web/server_scraper.go` 的
  `registerScraperWriters` 里加一行。
- 想添加新的 Media Server？参考 `internal/scraper/connector/jellyfin.go`，
  实现 `core.MediaServerConnector`，并在 `connector/factory.go` 的
  `NewConnector` 里加 case。
