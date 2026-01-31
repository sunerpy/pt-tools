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

添加新 PT 站点只需创建**一个文件**：`site/v2/definitions/<sitename>.go`

系统会自动从 `SiteDefinitionRegistry` 生成运行时元数据，无需手动编辑其他文件。

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
    Schema: "NexusPHP",         // "NexusPHP", "mTorrent", "Gazelle", "Unit3D"
    URLs:   []string{"https://mysite.com/"},

    // 可选字段（有默认值）
    AuthMethod: "cookie",       // "cookie" 或 "api_key"（根据 Schema 自动推断）
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

**Schema 与认证方式映射**：

| Schema     | 站点类型          | 默认认证方式 |
| ---------- | ----------------- | ------------ |
| `NexusPHP` | NexusPHP 架构站点 | Cookie       |
| `mTorrent` | M-Team 等         | API Key      |
| `Gazelle`  | Gazelle 架构站点  | Cookie       |
| `Unit3D`   | Unit3D 架构站点   | API Key      |

**添加步骤**：

1. 在 `site/v2/definitions/` 创建 `<sitename>.go`
2. 参考现有实现（如 `hdsky.go`、`mteam.go`）
3. 运行测试：`go test ./site/v2/...`
4. 更新站点列表文档：`docs/sites.md`
5. 提交 PR

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

如有开发相关问题，欢迎在 [GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions) 讨论。

[返回首页](../README.md)
