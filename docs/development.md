# 开发指南

本文档面向希望参与 pt-tools 开发或从源码构建的开发者。

[返回首页](../README.md)

## 目录

- [环境要求](#环境要求)
- [从源码构建](#从源码构建)
  - [克隆仓库](#克隆仓库)
  - [构建前端](#构建前端)
  - [构建后端](#构建后端)
  - [使用 Makefile](#使用-makefile)
- [开发模式](#开发模式)
- [技术架构](#技术架构)
  - [技术栈](#技术栈)
  - [项目结构](#项目结构)
  - [性能优化](#性能优化)
- [贡献指南](#贡献指南)
  - [提交 Issue](#提交-issue)
  - [提交 Pull Request](#提交-pull-request)
  - [添加新站点支持](#添加新站点支持)
- [代码规范](#代码规范)

## 环境要求

| 依赖        | 版本要求 | 说明         |
| ----------- | -------- | ------------ |
| **Go**      | 1.22+    | 后端开发语言 |
| **Node.js** | 18+      | 前端构建环境 |
| **pnpm**    | 8+       | 前端包管理器 |

### 安装依赖

**Go**：

```bash
# Linux (使用官方安装脚本)
wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# macOS (使用 Homebrew)
brew install go
```

**Node.js 和 pnpm**：

```bash
# 使用 nvm 安装 Node.js
nvm install 25
nvm use 25

# 安装 pnpm
npm install -g pnpm
```

## 从源码构建

### 克隆仓库

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools
```

### 构建前端

```bash
cd web/frontend
pnpm install
pnpm build
cd ../..
```

构建产物会输出到 `web/frontend/dist/` 目录，并被嵌入到 Go 二进制文件中。

### 构建后端

```bash
go build -o pt-tools .
```

### 使用 Makefile

推荐使用 Makefile 进行构建，它封装了完整的构建流程：

```bash
# 完整构建（前端 + 后端）
make build-local

# 仅构建前端
make build-frontend

# 仅构建后端
make build-backend

# 查看所有可用命令
make help
```

构建产物位于 `dist/` 目录。

## 开发模式

开发模式下，前端和后端可以分别启动，支持热重载：

**终端 1 - 启动前端开发服务器**：

```bash
cd web/frontend
pnpm dev
```

前端开发服务器默认运行在 `http://localhost:5173`。

**终端 2 - 启动后端服务**：

```bash
go run main.go web --port 8081
```

也可以直接运行`make run-dev`来启动开发环境（默认8080端口）

后端服务运行在 `http://localhost:8081`。

**开发环境配置**：

- 前端会自动代理 API 请求到后端
- 修改前端代码后会自动热重载
- 修改后端代码需要重新运行

## 技术架构

### 项目结构

```
pt-tools/
├── cmd/              # CLI 命令定义 (Cobra)
├── config/           # 日志配置 (Zap)
├── core/             # 运行时生命周期、配置存储、迁移
├── global/           # 全局单例（logger, DB）
├── internal/         # 统一站点接口、RSS 处理器、过滤器
├── models/           # GORM 模型定义
├── scheduler/        # RSS 任务调度器
├── site/v2/          # 站点驱动和定义
│   └── definitions/  # 各站点具体实现
├── thirdpart/        # 第三方集成
│   └── downloader/   # 下载器接口
├── utils/            # 工具函数
├── web/              # HTTP 服务和 API
│   ├── api_*.go      # API 处理器
│   └── frontend/     # Vue 3 前端项目
├── tools/
│   └── browser-extension/  # 浏览器扩展 (TypeScript + Vue 3)
├── main.go           # 程序入口
├── Makefile          # 构建脚本
└── go.mod            # Go 模块定义
```

### 性能优化

pt-tools 采用了多项性能优化措施：

| 优化项         | 说明                                      |
| -------------- | ----------------------------------------- |
| **内存缓存**   | 两级缓存减少数据库访问                    |
| **熔断器**     | 自动检测和隔离故障站点                    |
| **连接池**     | HTTP 连接复用，减少连接开销               |
| **持久化限流** | SQLite 存储滑动窗口，重启后限流状态不丢失 |
| **并发控制**   | 合理的并发数避免被站点封禁                |

## 贡献指南

欢迎贡献代码或提交问题！

### 提交 Issue

在提交 Issue 前，请：

1. 搜索现有 Issue，避免重复
2. 使用清晰的标题描述问题
3. 提供详细的复现步骤
4. 附上相关日志或截图

Issue 地址：[GitHub Issues](https://github.com/sunerpy/pt-tools/issues)

### 提交 Pull Request

1. Fork 仓库到你的账户
2. 创建功能分支：`git checkout -b feature/your-feature`
3. 提交更改：`git commit -m "Add your feature"`
4. 推送分支：`git push origin feature/your-feature`
5. 创建 Pull Request

PR 地址：[GitHub Pull Requests](https://github.com/sunerpy/pt-tools/pulls)

### 添加新站点支持

> **没有编程经验？** 请参考 [请求新增站点支持（无需编程经验）](guide/request-new-site.md)，你只需要提供站点页面数据，维护者会帮你完成适配。

添加新 PT 站点只需创建 **一个定义文件**，系统会自动完成以下工作：

- 从 `SiteDefinitionRegistry` 生成运行时元数据
- 在 `SiteRegistry` 中注册站点
- API 返回 `is_builtin: true` 标识为内置站点
- 前端自动识别为不可删除的预置站点

**无需手动编辑** `models/enter.go`、`web/server.go` 或前端代码。

**站点定义模板**：

```go
package definitions

import (
    v2 "github.com/sunerpy/pt-tools/site/v2"
)

var MySiteDefinition = &v2.SiteDefinition{
    // 必填字段
    ID:     "mysite",           // 唯一标识符（小写）
    Name:   "MySite",           // 显示名称
    Schema: v2.SchemaNexusPHP,    // 见下方「Schema 枚举」
    URLs:   []string{"https://mysite.com/"},

    // 可选字段（有默认值）
    AuthMethod: v2.AuthMethodCookie, // 见下方「AuthMethod 枚举」（根据 Schema 自动推断，通常无需设置）
    RateLimit:  2.0,            // 请求频率限制（默认 2.0 req/s，持久化存储）
    RateBurst:  5,              // 突发请求数（默认 5）

    // 可选元数据
    Aka:            []string{"MS", "MySite别名"},
    Description:    "站点描述",
    FaviconURL:     "https://mysite.com/favicon.ico",
    TimezoneOffset: "+0800",

    // 站点特定选择器（覆盖 Schema 默认值）
    Selectors: &v2.SiteSelectors{
        // 搜索结果页免费图标选择器
        DiscountIcon: "img.pro_free",
        // 自定义免费关键词映射（可选，nil 时使用默认映射）
        DiscountMapping: map[string]v2.DiscountLevel{
            "custom_free": v2.DiscountFree,
        },
    },

    // 详情页解析配置（用于 RSS 详情获取）
    DetailParser: &v2.DetailParserConfig{
        DiscountSelector: "h1 font",                    // 免费标签选择器
        DiscountMapping: map[string]v2.DiscountLevel{   // CSS class → 免费等级
            "free":      v2.DiscountFree,
            "twoupfree": v2.Discount2xFree,
        },
        TimeLayout: "2006-01-02 15:04:05",              // 时间格式
    },

    // 用户信息解析配置
    UserInfo: &v2.UserInfoConfig{...},

    // 等级要求
    LevelRequirements: []v2.SiteLevelRequirement{...},
}

func init() {
    v2.RegisterSiteDefinition(MySiteDefinition)
}
```

**Schema 枚举**（`v2.Schema` 类型，定义在 `site/v2/types.go`）：

| 枚举常量            | 值           | 站点类型            | 默认 AuthMethod      |
| ------------------- | ------------ | ------------------- | -------------------- |
| `v2.SchemaNexusPHP` | `"NexusPHP"` | NexusPHP 架构站点   | `cookie`             |
| `v2.SchemaMTorrent` | `"mTorrent"` | M-Team 等           | `api_key`            |
| `v2.SchemaGazelle`  | `"Gazelle"`  | Gazelle 架构站点    | `cookie`             |
| `v2.SchemaUnit3D`   | `"Unit3D"`   | Unit3D 架构站点     | `api_key`            |
| `v2.SchemaHDDolby`  | `"HDDolby"`  | HDDolby 专用        | `cookie_and_api_key` |
| `v2.SchemaRousi`    | `"Rousi"`    | RousiPro 自定义 API | `passkey`            |

**AuthMethod 枚举**（`v2.AuthMethod` 类型，定义在 `site/v2/types.go`）：

| 枚举常量                       | 值                     | 说明                          |
| ------------------------------ | ---------------------- | ----------------------------- |
| `v2.AuthMethodCookie`          | `"cookie"`             | 浏览器 Cookie 认证            |
| `v2.AuthMethodAPIKey`          | `"api_key"`            | API Key 认证                  |
| `v2.AuthMethodCookieAndAPIKey` | `"cookie_and_api_key"` | Cookie + API Key 双重认证     |
| `v2.AuthMethodPasskey`         | `"passkey"`            | Passkey 认证（用于 RSS/下载） |

**添加步骤**：

1. 在 `site/v2/definitions/` 创建 `<sitename>.go`
2. 参考现有实现（如 `hdsky.go`、`mteam.go`）
3. **必须** 创建 `<sitename>_fixture_test.go`，提供 fixture 数据证明解析逻辑正确（见下方「Fixture 测试要求」）
4. 运行测试：`go test ./site/v2/...`（CI 会自动验证定义的完整性和正确性）
5. 更新站点列表文档：`docs/sites.md`
6. 提交 PR

> **注意**：旧版本需要手动更新 `models/enter.go` 中的 `AllowedSiteGroups` 和前端的预置站点列表，现已不再需要。系统会从 Registry 自动识别内置站点。

**自定义驱动（单文件模式）**：

如果站点架构与现有 Schema 不兼容，可以在 **同一个文件** 中实现完整的驱动逻辑。参考 `definitions/rousipro.go` 的实现：

```go
package definitions

import (
    "context"
    "encoding/json"
    "fmt"

    v2 "github.com/sunerpy/pt-tools/site/v2"
    "go.uber.org/zap"
)

var CustomSiteDefinition = &v2.SiteDefinition{
    ID:          "customsite",
    Name:        "CustomSite",
    Schema:      v2.Schema("CustomSite"), // 在 site/v2/types.go 中添加新 Schema 常量
    URLs:        []string{"https://customsite.com/"},
    CreateDriver: createCustomDriver,
}

func init() {
    v2.RegisterSiteDefinition(CustomSiteDefinition)
}

func createCustomDriver(config v2.SiteConfig, logger *zap.Logger) (v2.Site, error) {
    var opts v2.CustomOptions
    if err := json.Unmarshal(config.Options, &opts); err != nil {
        return nil, err
    }

    driver := &customDriver{baseURL: config.BaseURL, apiKey: opts.APIKey}

    return v2.NewBaseSite(driver, v2.BaseSiteConfig{
        ID:     config.ID,
        Name:   config.Name,
        Kind:   v2.SiteKind("custom"),
        Logger: logger,
    }), nil
}

// customDriver 实现 v2.Driver 接口
type customDriver struct {
    baseURL string
    apiKey  string
}

// 实现 Driver 接口的方法...
func (d *customDriver) PrepareSearch(q v2.SearchQuery) (customReq, error) { ... }
func (d *customDriver) Execute(ctx context.Context, req customReq) (customRes, error) { ... }
func (d *customDriver) ParseSearch(res customRes) ([]v2.TorrentItem, error) { ... }
func (d *customDriver) GetUserInfo(ctx context.Context) (v2.UserInfo, error) { ... }
func (d *customDriver) PrepareDownload(id string) (customReq, error) { ... }
func (d *customDriver) ParseDownload(res customRes) ([]byte, error) { ... }
```

当设置了 `CreateDriver` 时，系统会优先使用它而非基于 Schema 的驱动查找。这种模式的优势是 **新站点只需要一个文件**，所有驱动逻辑都包含在 `definitions/<site>.go` 中。

### Fixture 测试要求

**所有站点**（无论是复用 NexusPHP 还是自定义驱动）都 **必须** 提供 fixture 测试，以便仓库 owner 通过审查实际数据来判断 PR 的正确性。

测试文件命名：`definitions/<sitename>_fixture_test.go`

#### FixtureSuite 注册（必须）

每个站点 **必须** 在 `init()` 中注册 `FixtureSuite`，包含三个必填测试函数：

| 字段       | 验证内容          | 说明                               |
| ---------- | ----------------- | ---------------------------------- |
| `Search`   | 搜索/RSS 列表解析 | 标题、大小、免费状态、免费结束时间 |
| `Detail`   | 种子详情页解析    | 免费等级、HR 状态、大小            |
| `UserInfo` | 用户信息解析      | 上传量、下载量、分享率、等级       |

`TestAllSites_FixtureCoverage`（在 `definitions_validation_test.go` 中）会自动遍历所有注册站点，检查是否有 `FixtureSuite` 且三个字段非 nil。**缺少任一字段的新站点 CI 会直接失败。**

```go
func init() {
    RegisterFixtureSuite(FixtureSuite{
        SiteID:   "mysite",
        Search:   testMySiteSearch,
        Detail:   testMySiteDetail,
        UserInfo: testMySiteUserInfo,
    })
}
```

#### 辅助函数

测试框架（`definitions/fixture_helper_test.go`）提供以下辅助函数：

```go
RequireNoSecrets(t, "fixture_name", data)
resp := DecodeFixtureJSON[ResponseType](t, "fixture_name", jsonString)
doc := FixtureDoc(t, "fixture_name", htmlString)
```

**隐私保护**：`RequireNoSecrets` 自动扫描以下模式，匹配则测试失败：

| 检测内容                              | 说明                                       |
| ------------------------------------- | ------------------------------------------ |
| `c_secure_uid=...` 等 NexusPHP cookie | 真实登录凭证                               |
| `PHPSESSID=...`                       | PHP 会话 ID                                |
| `passkey=<32+位hex>`                  | 真实 passkey                               |
| `Bearer <32+位token>`                 | 真实 API Token（`FAKE_`/`TEST_` 前缀除外） |

**规则**：

- 所有 fixture 数据 **必须** 是 Go 字符串常量，定义在 `_test.go` 文件中（不会编入生产二进制）
- 用户名、ID 等应脱敏（使用虚构值）
- 凭证字段使用 `FAKE_TEST_` 前缀

---

#### NexusPHP 站点（HTML fixture）

NexusPHP 站点通过 CSS 选择器解析 HTML 页面。参考 `hdsky_fixture_test.go`：

```go
package definitions

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
    RegisterFixtureSuite(FixtureSuite{
        SiteID:   "mysite",
        Search:   testMySiteSearch,
        Detail:   testMySiteDetail,
        UserInfo: testMySiteUserInfo,
    })
}

// 脱敏的搜索页 HTML — 保留真实 DOM 结构，替换敏感数据
const mysiteSearchFixture = `<html><body>
<table class="torrents"><tbody>
<tr>
  <td class="rowfollow"><img alt="Movie" /></td>
  <td class="rowfollow">
    <table class="torrentname"><tr><td class="embedded">
      <a href="details.php?id=12345">Test.Movie.2025</a>
      <img class="pro_free" src="pic/trans.gif" alt="Free"
        onmouseover="domTT_activate(this, event, 'content', '&lt;span title=&quot;2026-03-01 12:00:00&quot;&gt;29天&lt;/span&gt;')" />
    </td></tr></table>
  </td>
  <td class="rowfollow"></td>
  <td class="rowfollow"><span title="2025-01-15 08:30:00">1天前</span></td>
  <td class="rowfollow">42.5 GB</td>
  <td class="rowfollow">150</td>
  <td class="rowfollow">10</td>
  <td class="rowfollow">500</td>
</tr>
</tbody></table>
</body></html>`

func testMySiteSearch(t *testing.T) {
    def, ok := v2.GetDefinitionRegistry().Get("mysite")
    require.True(t, ok)

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        _, _ = w.Write([]byte(mysiteSearchFixture))
    }))
    defer server.Close()

    driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
        BaseURL: server.URL, Cookie: "test=1", Selectors: def.Selectors,
    })
    driver.SetSiteDefinition(def)

    res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
    require.NoError(t, err)

    items, err := driver.ParseSearch(res)
    require.NoError(t, err)
    require.Len(t, items, 1)
    assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
    assert.False(t, items[0].DiscountEndTime.IsZero())
}

func testMySiteDetail(t *testing.T) {
    def, ok := v2.GetDefinitionRegistry().Get("mysite")
    require.True(t, ok)
    doc := FixtureDoc(t, "detail", mysiteDetailFixture)
    parser := v2.NewNexusPHPParserFromDefinition(def)
    info := parser.ParseAll(doc.Selection)
    assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
    assert.NotEmpty(t, info.TorrentID)
}

func testMySiteUserInfo(t *testing.T) {
    def, ok := v2.GetDefinitionRegistry().Get("mysite")
    require.True(t, ok)
    driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
        BaseURL: def.URLs[0], Cookie: "test=1",
    })
    driver.SetSiteDefinition(def)

    doc := FixtureDoc(t, "index", mysiteIndexFixture)
    for field, expected := range map[string]string{"id": "12345", "name": "TestUser"} {
        sel := def.UserInfo.Selectors[field]
        assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
    }
}
```

#### 自定义驱动站点（JSON fixture）

自定义驱动站点使用 JSON API。参考 `rousipro_fixture_test.go`：

```go
func init() {
    RegisterFixtureSuite(FixtureSuite{
        SiteID:   "mysite",
        Search:   testMySiteSearch,
        Detail:   testMySiteDetail,
        UserInfo: testMySiteUserInfo,
    })
}

func testMySiteSearch(t *testing.T) {
    resp := DecodeFixtureJSON[myResponse](t, "search", mySearchFixtureJSON)
    driver := newMyDriver(myDriverConfig{BaseURL: "https://mysite.com", Passkey: "FAKE_TEST_KEY"})
    items, err := driver.ParseSearch(resp)
    require.NoError(t, err)
    require.Len(t, items, 2)
    assert.Equal(t, v2.DiscountFree, items[0].DiscountLevel)
}

func testMySiteDetail(t *testing.T) {
    // 验证 JSON detail 解析 + 免费状态判断
}

func testMySiteUserInfo(t *testing.T) {
    // 验证用户信息 JSON 解析
}
```

> **提示**：`TestCreateDriver_Smoke`（在 `definitions_validation_test.go` 中）会自动遍历所有 `CreateDriver` 站点。新增自定义驱动时，在 `fakeOptionsForSchema()` 中添加对应 Schema 的 fake options 即可。

## 代码规范

### 运行代码检查

```bash
# 运行 lint 检查
make lint

# 运行单元测试
make unit-test

# 格式化代码
make fmt
```

### Go 代码规范

- 遵循 [Effective Go](https://golang.org/doc/effective_go) 指南
- 使用 `gofmt` 格式化代码
- 导入分组：标准库、第三方库、项目内部包
- 错误处理：不要忽略错误，适当包装错误信息
- 注释：导出的函数和类型必须有文档注释

### 前端代码规范

- 遵循 Vue 3 Composition API 风格
- 使用 TypeScript 类型注解
- 组件命名使用 PascalCase
- 使用 ESLint 和 Prettier 格式化

### 提交信息规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Type 类型**：

- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `style`: 代码格式（不影响功能）
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具相关

**示例**：

```
feat(site): add support for NewSite

- Implement cookie authentication
- Add search and user info functions
- Update site registry

Closes #123
```

---

## 浏览器扩展开发与发布

### 本地开发

```bash
cd tools/browser-extension
pnpm install
pnpm dev          # watch 模式
pnpm build        # 单次构建
make build-extension  # 构建 + 站点一致性检查 + 打包 zip
```

详见 [扩展 README](../tools/browser-extension/README.md)。

### 发布到 Edge Add-ons

扩展使用独立的版本号和 tag（`ext-v*`），与 pt-tools 主版本互不影响。

#### 首次发布（手动）

1. **注册 Edge 开发者账号**（免费）
   - 访问 [Partner Center](https://partner.microsoft.com/dashboard/microsoftedge/public/login?ref=dd)
   - 可以使用 GitHub 账号直接登录
   - 完成注册表单（个人开发者即可）

2. **手动上传第一版扩展**

   首次发布必须通过 Partner Center 手动操作，API 仅支持更新已有扩展。
   - 运行 `make build-extension` 生成 `tools/browser-extension/pt-tools-helper.zip`
   - Partner Center → Microsoft Edge → 扩展 → 创建新扩展
   - 上传 zip 包，填写扩展名称、描述、截图等
   - 提交审核（通常 1-3 个工作日）

3. **获取 Product ID**

   审核通过后，在 Partner Center 的扩展详情页可以看到 **Product ID**（一个 GUID）。

4. **启用 Publish API 并获取凭证**
   - Partner Center → Microsoft Edge → Publish API
   - 点击 **"enable the new experience"** 旁的 **Enable** 按钮（启用 v1.1 API）
   - 点击 **Create API credentials**
   - 记录 **Client ID** 和 **API Key**

5. **配置 GitHub Secrets**

   在仓库 Settings → Secrets and variables → Actions 中添加：

   | Secret            | 值                                     |
   | ----------------- | -------------------------------------- |
   | `EDGE_PRODUCT_ID` | Partner Center 扩展详情页的 Product ID |
   | `EDGE_CLIENT_ID`  | Publish API 页面的 Client ID           |
   | `EDGE_API_KEY`    | Publish API 页面的 API Key             |

#### 后续发布（自动）

配置完成后，后续版本发布只需：

```bash
# 1. 更新版本号（package.json + manifest.ts 保持一致）
# 2. 提交代码
git add -A && git commit -m "chore(extension): bump to 0.2.0"
git push

# 3. 打 tag 触发自动发布
git tag ext-v0.2.0
git push origin ext-v0.2.0
```

CI 流程：`ext-v*` tag → 校验版本号一致性 → 站点一致性检查 → 构建 → 上传到 Edge Add-ons → 创建 GitHub Release。

也可以在 GitHub Actions 页面手动触发 `Extension Publish` workflow（支持 dry run 模式仅构建不上传）。

---

如有开发相关问题，欢迎在 [GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions) 讨论。

[返回首页](../README.md)
