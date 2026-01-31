# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.0] - 2026-01-31

### CI/CD

- Add dependabot auto-merge workflow with safety checks
- Add dependabot auto-merge workflow with safety checks ([#45](https://github.com/sunerpy/pt-tools/pull/45))
- Bump actions/setup-go from 5 to 6
  Bumps [actions/setup-go](https://github.com/actions/setup-go) from 5 to 6. - [Release notes](https://github.com/actions/setup-go/releases) - [Commits](https://github.com/actions/setup-go/compare/v5...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-go
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-node from 4 to 6
  Bumps [actions/setup-node](https://github.com/actions/setup-node) from 4 to 6. - [Release notes](https://github.com/actions/setup-node/releases) - [Commits](https://github.com/actions/setup-node/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-node
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/download-artifact from 4 to 7
  Bumps [actions/download-artifact](https://github.com/actions/download-artifact) from 4 to 7. - [Release notes](https://github.com/actions/download-artifact/releases) - [Commits](https://github.com/actions/download-artifact/compare/v4...v7)

        ---
        updated-dependencies:
        - dependency-name: actions/download-artifact
         dependency-version: '7'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/cache from 4 to 5
  Bumps [actions/cache](https://github.com/actions/cache) from 4 to 5. - [Release notes](https://github.com/actions/cache/releases) - [Changelog](https://github.com/actions/cache/blob/main/RELEASES.md) - [Commits](https://github.com/actions/cache/compare/v4...v5)

        ---
        updated-dependencies:
        - dependency-name: actions/cache
         dependency-version: '5'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/checkout from 4 to 6
  Bumps [actions/checkout](https://github.com/actions/checkout) from 4 to 6. - [Release notes](https://github.com/actions/checkout/releases) - [Changelog](https://github.com/actions/checkout/blob/main/CHANGELOG.md) - [Commits](https://github.com/actions/checkout/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/checkout
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump vue from 3.5.26 to 3.5.27 in /web/frontend
  Bumps [vue](https://github.com/vuejs/core) from 3.5.26 to 3.5.27. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.26...v3.5.27)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.27
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.39.0 to 1.42.0 in /web/frontend
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.39.0 to 1.42.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.42.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.42.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxfmt from 0.24.0 to 0.27.0 in /web/frontend ([#37](https://github.com/sunerpy/pt-tools/issues/37)) ([#37](https://github.com/sunerpy/pt-tools/pull/37))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.24.0 to 0.27.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.27.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.27.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Features

- **frontend**: 新增用户数据导出分享功能
- 新增 UserDataExport.vue 组件，支持 Canvas 渲染生成分享图片 - 支持导出总上传/下载、分享率、魔力值、做种数等汇总统计 - 支持展示各站点详情：用户名、上传下载量、魔力值、入站时长 - 隐私保护：支持模糊用户名、站点名和站点图标（马赛克效果）- 6 种预设主题配色 + 自定义颜色选择器 - 支持下载 PNG 图片和复制到剪贴板 - 可自由选择要展示的站点
- **frontend**: 新增用户数据导出分享功能 ([#46](https://github.com/sunerpy/pt-tools/pull/46))
- 新增 UserDataExport.vue 组件，支持 Canvas 渲染生成分享图片 - 支持导出总上传/下载、分享率、魔力值、做种数等汇总统计 - 支持展示各站点详情：用户名、上传下载量、魔力值、入站时长 - 隐私保护：支持模糊用户名、站点名和站点图标（马赛克效果）- 支持下载 PNG 图片和复制到剪贴板 - 可自由选择要展示的站点

## [0.6.0] - 2026-01-30

### Bug Fixes

- Downloader sync bugs - RSS subscription sync + auto-enable on set default
- BatchUpdateSiteDownloader now also updates associated RSS subscriptions' downloader_id - setDefaultDownloader automatically enables the downloader when set as default

      Both fixes include corresponding tests.

### Features

- 站点配置统一化与下载器增强
- 简化新增站点配置，修复未配置qbit下载器的错误，增加tg交流群 ([#42](https://github.com/sunerpy/pt-tools/pull/42))
- 修复未配置qbittorrent下载器时跳过站点的问题 - 统一站点配置源，简化新站点添加流程，新增站点只需创建 definitions/.go 文件 - 前端禁用不可用站点并同步数据库状态 - 新增tg交流群

### Miscellaneous

- **build**: 更新 Go 版本至 1.25.6
- 统一构建环境中的 Go 版本 - 确保与 Docker 构建镜像版本一致

## [0.5.0] - 2026-01-24

### Features

- **web**: 增加版本一键自动升级功能
- 新增运行时环境检测与升级状态接口 - 实现 Web 界面触发的二进制自动升级流程 - 支持下载进度跟踪与取消操作 - 前端集成升级控制与状态展示逻辑

## [0.4.2] - 2026-01-24

### Features

- **downloader**: 增强下载器连接检查与错误提示
- 优化 qBittorrent 和 Transmission 的连接错误处理 - 添加详细的中文错误信息和日志记录 - 前端校验下载器表单必填字段并高亮显示状态

### Miscellaneous

- **build**: 切换 changelog 格式化工具至 oxfmt
- 移除 dprint 相关配置与使用 - 使用 oxfmt 替代 dprint 进行 markdown 格式化

## [0.4.1] - 2026-01-24

### Bug Fixes

- **downloader**: 去除 URL 尾部斜杠并优化下载器检查逻辑
- 为 qBittorrent 和 Transmission 的 GetURL 方法添加去除尾斜杠处理 - 改进 downloaderHealthCheck 接口实现，支持真正的连接测试 - 增强错误提示信息，区分不同类型下载器的健康状态

### Miscellaneous

- **frontend**: 使用oxc oxfmt 和 oxlint 并更新 Makefile
- 更改前端格式化工具为 oxfmt，调整 CI 中的格式检查步骤 - 引入 .oxfmtrc.json 配置文件并移除旧的 dprint 配置 - 更新 tsconfig.json、vite.config.ts 及多个 Vue 文件中的语法（主要是添加分号）- 调整 cliff.toml 以支持提交正文内容显示 - 添加 pre-commit 钩子配置支持 fmt 和 lint 命令
- **build**: 优化 release.yml 中的文件重命名逻辑以支持 Windows 可执行文件
- 区分处理 `.exe` 文件和非 `.exe` 文件 - 确保 Windows 平台下保留可执行文件扩展名 - 统一压缩前的临时目录结构操作
- **build**: 调整二进制打包方式以支持 latest 版本下载
- 移除文件名中的版本标签 - 更新 release workflow 中的下载链接为 latest 地址 - 便于用户通过固定链接获取最新构建产物
- **build**: 增强发布工作流中的标签验证与变量引用安全性
- 添加输入标签格式校验，确保符合语义化版本规范 - 优化构建与打包命令中的变量传递方式

## [0.4.0] - 2026-01-19

### Features

- **version**: 增加版本检查功能支持检测 GitHub 新版本并提供更新提醒
- 新增 version/checker 包实现 GitHub Releases 检查逻辑 - 支持语义化版本解析和比较 - 提供 API 接口 /api/version 和 /api/version/check - 前端集成版本检查组件和状态管理 - 支持通过代理获取更新及版本忽略功能

## [0.3.5] - 2026-01-18

### Bug Fixes

- **site**: 修复 HDDolby 种子选择器并优化时间解析逻辑
- 新增对 HDDolby 站点种子列表各项属性的选择器定义 - 改进 NexusPHP 驱动，支持从 onmouseover 属性中提取折扣结束时间 - 添加针对不同站点的折扣时间解析测试用例

## [0.3.4] - 2026-01-18

### Bug Fixes

- **scheduler**: 修复 Manager 事件监听器导致的数据竞态
- 添加 stopped 标志和 eventCancel 用于优雅关闭事件监听 goroutine - StopAll() 现在会设置 stopped=true 并调用 eventCancel() 终止监听 - 事件监听 goroutine 检查 stopped 标志，防止访问已关闭资源 - rss 命令执行后清理 scheduler Manager - 添加 defer m.StopAll() 确保后台 goroutine 正确退出

### Features

- **scheduler**: 优化免费结束监控器的并发处理逻辑
- 防止独立定时器与周期检查协程重复处理相同任务 - 提升系统在某些场景下的稳定性与数据一致性

### Miscellaneous

- **build**: 调整 Makefile 和格式化配置以支持 dprint 工具
- **build**: 优化 CI 流程
- 更新 README 中的 Go 版本标识 - 调整覆盖率上传 artifact 命名规则 - 优化 CI 成功检查逻辑，明确依赖任务结果判断
- **build**: 更新 golangci-lint 安装方式并升级 pnpm 版本
- 使用 go install 替代 GitHub Action 安装 golangci-lint - 将 pnpm 版本从 9 升级至 10
- **ci**: 增加前端构建任务并优化 CI 流程
- 新增独立的 frontend-build job 处理前端构建和检查 - 前端产物通过 artifact 在各 job 间传递 - 移除原有的 frontend-checks job

### Testing

- **site**: 增加测试中的错误处理
- 在多个测试函数中添加缺失的 return 语句以避免继续执行无效逻辑 - 修正部分测试断言和条件判断顺序，确保测试更稳定可靠

## [0.3.2] - 2026-01-17

### Bug Fixes

- **scheduler**: 修复种子被下载器删除后的状态处理逻辑
- 检测到种子不存在时自动标记任务为完成并清空下载器任务ID - 更新前端任务列表显示“已删除”状态 - 优化日志提示信息，区分不同错误原因

## [0.3.0] - 2026-01-17

### CI/CD

- Bump actions/upload-artifact from 4 to 6
  Bumps [actions/upload-artifact](https://github.com/actions/upload-artifact) from 4 to 6. - [Release notes](https://github.com/actions/upload-artifact/releases) - [Commits](https://github.com/actions/upload-artifact/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/upload-artifact
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-node from 4 to 6
  Bumps [actions/setup-node](https://github.com/actions/setup-node) from 4 to 6. - [Release notes](https://github.com/actions/setup-node/releases) - [Commits](https://github.com/actions/setup-node/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-node
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-go from 4 to 6
  Bumps [actions/setup-go](https://github.com/actions/setup-go) from 4 to 6. - [Release notes](https://github.com/actions/setup-go/releases) - [Commits](https://github.com/actions/setup-go/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-go
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/checkout from 4 to 6
  Bumps [actions/checkout](https://github.com/actions/checkout) from 4 to 6. - [Release notes](https://github.com/actions/checkout/releases) - [Changelog](https://github.com/actions/checkout/blob/main/CHANGELOG.md) - [Commits](https://github.com/actions/checkout/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/checkout
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/download-artifact from 4 to 7
  Bumps [actions/download-artifact](https://github.com/actions/download-artifact) from 4 to 7. - [Release notes](https://github.com/actions/download-artifact/releases) - [Commits](https://github.com/actions/download-artifact/compare/v4...v7)

        ---
        updated-dependencies:
        - dependency-name: actions/download-artifact
         dependency-version: '7'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump docker/setup-buildx-action from 2 to 3
  Bumps [docker/setup-buildx-action](https://github.com/docker/setup-buildx-action) from 2 to 3. - [Release notes](https://github.com/docker/setup-buildx-action/releases) - [Commits](https://github.com/docker/setup-buildx-action/compare/v2...v3)

        ---
        updated-dependencies:
        - dependency-name: docker/setup-buildx-action
         dependency-version: '3'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump docker/login-action from 2 to 3
  Bumps [docker/login-action](https://github.com/docker/login-action) from 2 to 3. - [Release notes](https://github.com/docker/login-action/releases) - [Commits](https://github.com/docker/login-action/compare/v2...v3)

        ---
        updated-dependencies:
        - dependency-name: docker/login-action
         dependency-version: '3'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump globals from 16.5.0 to 17.0.0 in /web/frontend
  Bumps [globals](https://github.com/sindresorhus/globals) from 16.5.0 to 17.0.0. - [Release notes](https://github.com/sindresorhus/globals/releases) - [Commits](https://github.com/sindresorhus/globals/compare/v16.5.0...v17.0.0)

        ---
        updated-dependencies:
        - dependency-name: globals
         dependency-version: 17.0.0
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

- **pnpm**: Bump @types/node from 24.10.7 to 25.0.7 in /web/frontend
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 24.10.7 to 25.0.7. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.0.7
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

- **pnpm**: Bump @typescript-eslint/parser in /web/frontend
  Bumps [@typescript-eslint/parser](https://github.com/typescript-eslint/typescript-eslint/tree/HEAD/packages/parser) from 8.52.0 to 8.53.0. - [Release notes](https://github.com/typescript-eslint/typescript-eslint/releases) - [Changelog](https://github.com/typescript-eslint/typescript-eslint/blob/main/packages/parser/CHANGELOG.md) - [Commits](https://github.com/typescript-eslint/typescript-eslint/commits/v8.53.0/packages/parser)

        ---
        updated-dependencies:
        - dependency-name: "@typescript-eslint/parser"
         dependency-version: 8.53.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 3.2.4 to 4.0.17 in /web/frontend
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 3.2.4 to 4.0.17. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.0.17/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.0.17
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/text from 0.32.0 to 0.33.0
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.32.0 to 0.33.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.32.0...v0.33.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.33.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/text from 0.32.0 to 0.33.0
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.32.0 to 0.33.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.32.0...v0.33.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.33.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **task**: 实现免费种子到期自动暂停功能
- 新增 `DownloaderInfo` 结构体及 `GetDownloaderForRSS` 方法，支持获取下载器状态 - 优化任务监控机制，确保免费时间结束时未完成的任务能自动切换至暂停状态

### Miscellaneous

- **frontend**: 更新 pnpm 锁定文件中的依赖版本和 libc 支持
- **build**: 调整 GitHub Actions 触发条件为仅标签推送时更新 CHANGELOG.md
- 移除对 main 分支的监听限制 - 改为只在 v\* 标签推送时触发工作流

## [0.2.0] - 2026-01-11

### Bug Fixes

- 修复构建错误
- 修复前端模板构建错误
- 修复构建错误

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.39.0 to 0.40.0
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.39.0 to 0.40.0. - [Commits](https://github.com/golang/sys/compare/v0.39.0...v0.40.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.40.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- 支持多下载器(qbittorrent,transmission),支持规则过滤，多站点搜索与用户信息聚合，统一接口与文档设计
- 引入 UnifiedPTSite 统一接口 - 新增多站点用户信息聚合（上传/下载/做种）与并发搜索（去重、排序）- 新增 Transmission 下载器支持 - Web 端支持动态站点搜索 - 增强配置管理：ConfigStore.SyncSites、RSS 过滤规则、运行时日志级别 - 废弃旧泛型实现，统一错误处理
- 支持多下载器(qbittorrent,transmission),支持规则过滤，多站点搜索与用户信息聚合，统一接口与文档设计 ([#4](https://github.com/sunerpy/pt-tools/pull/4))

## [0.1.6] - 2025-12-25

### Bug Fixes

- **frontend**: 修复 RSS 相关空值问题
- 修复添加 RSS 时可能出现的空数组访问问题 - 改进重复 RSS URL 检查逻辑，避免空值错误

## [0.1.5] - 2025-12-22

### Bug Fixes

- **internal**: 优化种子过期处理逻辑

## [0.1.4] - 2025-12-22

### Bug Fixes

- **api**: 修复页面添加rss订阅时响应较慢和添加失败的问题
- 前端修正 RSS ID 类型从 string 为 number，保持前后端一致 - 增加前端本地去重判断，提升用户体验 - 所有 RSS 相关操作增加详细日志记录

### Features

- **release**: 优化发布流程,优化release页面内容展示
  使用标准化的 changelog 生成与 Docker 镜像推送逻辑，并修复文件打包路径问题。

## [0.1.3] - 2025-12-16

### Bug Fixes

- **internal**: 站点初始化失败时返回错误并跳过该站点
  修复了 MTEAM、HDSKY 和 CMCT 站点实现中 NewXxxImpl 函数的错误处理逻辑

### Build

- **workflow**: 添加前端构建步骤到release工作流

### Documentation

- **readme**: 更新 README 文档内容与结构
- 补充支持站点列表及认证方式说明 - 完善 Web 配置页面各模块的参数说明

### Features

- **web**: 使用vue3改写前端页面
  在 Dockerfile 中新增前端构建阶段，使用vue3改写页面。

## [0.1.1] - 2025-11-22

### Bug Fixes

- 修复windows下种子保存路径识别错误的问题

## [0.1.0] - 2025-11-17

### Documentation

- **docker**: 移除 Docker 时区设置说明

### Features

- 重构配置系统，新增 Web 管理界面与多项功能优化
- 改用 SQLite 存储配置，移除 viper 依赖 - 新增 Web 管理界面及静态资源，支持密码重置与任务分页 - 引入 TorrentInfo.IsFree、重试计数、错误记录等字段，优化任务列表展示 - 统一工作目录常量，增强站点配置校验，简化 Docker 单目录挂载 - 调整 UI 样式，移除废弃配置与命令，更新 Go 1.25.2 与文档

## [0.0.17] - 2025-07-02

### Bug Fixes

- **docker**: 为添加的用户设置 HOME 环境变量
- 在创建用户时使用 -h 参数指定 HOME 目录 - 解决了pt-tools工作目录错误的问题

## [0.0.16] - 2025-07-02

### Features

- **docker**: 添加环境变量配置并优化容器启动脚本
- 在 README.md 中添加环境变量配置说明，包括 PUID、PGID 和 TZ - 修改 docker-entrypoint.sh，优化 /app 目录权限设置，忽略只读挂载目录的错误

## [0.0.15] - 2025-07-02

### Documentation

- **docker**: 更新 README 中的容器交互命令
- 将 docker exec 命令中的 /bin/bash 改为 /bin/sh
- 更新 README.md 中的项目描述

### Features

- **docker**: 优化 Docker 构建和运行时环境
- 添加 gosu 工具，用于在非 root 用户下运行应用 - 在构建过程中添加 ca-certificates、dpkg 和 gnupg 依赖 - 通过环境变量设置 PUID 和 PGUID，默认为 1000 - 将用户创建和权限设置移至初始化脚本中 - 修改启动命令，使用 gosu 切换到目标用户运行应用 - 优化 Makefile 中的构建命令

## [0.0.14] - 2025-07-02

### Features

- **docker**: 添加 Docker 支持并优化配置流程
- 新增 Dockerfile 和 docker-entrypoint.sh 文件，实现 Docker 化部署 - 更新 Makefile，添加 HTTP_PROXY 等代理变量支持 - 修改 README.md，增加 Docker 部署说明 - 重构 config_init.go，优化配置目录初始化逻辑 - 更新 hooks.go，添加对下载目录的检查和初始化 - 调整 root.go，延迟配置文件加载到程序运行时 - 修改 viper.go，增加对默认配置文件路径的支持

## [0.0.13] - 2025-07-02

### Features

- **run**: 添加程序互斥锁功能
- 实现了 acquireLockOrExit 函数来创建和加锁锁文件 - 在 runCmdFunc 中添加了锁文件的创建和释放逻辑

### Refactor

- **run**: 重构互斥锁实现，支持跨平台
- 移除原有直接使用 unix.Flock 的实现 - 新增 utils 包下的 Locker 接口和具体实现 - 实现了 Unix 和 Windows 平台的锁机制 - 优化了错误处理和资源释放

## [0.0.12] - 2025-07-01

### Features

- **site**: 添加种子信息缓存并优化下载流程
- 新增 bigcache 作为种子信息缓存，提高重复请求的处理效率 - 优化下载工作器中的日志输出，提高错误信息的可读性 - 在下载路径中使用清理后的标题，避免特殊字符导致的文件名错误 - 允许 Collector 重新访问已爬取的 URL

## [0.0.11] - 2025-07-01

### Build

- 更新 Go 依赖至最新版本
- 将 Go 语言版本从 1.23.1 升级到 1.24.3 - 更新多个依赖库至最新版本 - 修复非免费种子误下载的问题
- 更新 Go 依赖至最新版本
- 将 Go 语言版本从 1.23.1 升级到 1.24.3 - 更新多个依赖库至最新版本 - 修复非免费种子误下载的问题

## [0.0.10] - 2025-07-01

### Features

- **internal**: 优化种子处理逻辑并添加过期检查
- 新增 processSingleTorrent 函数，用于独立处理每个种子文件 - 添加种子过期检查逻辑，标记并删除过期种子 - 优化已推送种子的处理流程，避免重复推送
- **internal**: 优化种子处理逻辑并添加过期检查
- 新增 processSingleTorrent 函数，用于独立处理每个种子文件 - 添加种子过期检查逻辑，标记并删除过期种子 - 优化已推送种子的处理流程，避免重复推送

## [0.0.9] - 2025-04-06

### Build

- **ci**: 升级 GitHub Actions 依赖版本
- 将 actions/checkout 从 v3 升级到 v4 - 将 actions/upload-artifact 从 v3 升级到 v4 - 将 actions/download-artifact 从 v3 升级到 v4

## [0.0.8] - 2025-04-06

### Features

- **qbit**: 添加请求自动重试机制并处理禁止访问错误

## [0.0.7] - 2024-12-06

### Refactor

- **rss**: 优化 RSS 任务执行间隔和日志处理
- 新增 getInterval 函数，用于获取 RSS 任务的执行间隔 - 使用全局配置中的默认间隔作为备用 - 优化日志记录，将 Fatal 改为 Error，避免程序意外退出 - 添加信号量控制，确保数据库事务的原子性 - 更新 go.mod 和 go.sum，添加 golang.org/x/sync 依赖

## [0.0.6] - 2024-12-05

### Features

- **log**: 重构日志系统并优化输出格式
- 重构了全局日志初始化和访问方式 - 优化了日志输出格式，增加了更多详细信息 - 调整了日志级别和输出方式 - 修复了一些日志相关的错误处理

## [0.0.5] - 2024-12-05

### Features

- **cmd**: 改进多个子命令描述和逻辑
- 为 `config` 命令更新了描述，简化并增强了帮助信息 - 为 `config init` 添加示例和详细说明 - 增强 `db` 命令，添加 `PersistentPreRun` 以确保配置检查 - 修改 `db init` 和 `db backup` 的描述及运行逻辑，增加用户提示和错误处理 - 改进 `task` 和 `task list` 命令的描述，补充示例，完善输出信息 - 改进配置和日志初始化流程的错误处理

## [0.0.4] - 2024-12-05

### Bug Fixes

- 禁用CGO编译
- 禁用CGO,以支持二进制文件独立运行

## [0.0.3] - 2024-12-04

### Documentation

- **README**: 更新文档快速部署和使用 pt-tools
- 新增一键部署脚本说明 - 添加下载最新 Release 的详细步骤 - 补充快速开始部分，包括初始化配置和运行方法 - 更新 GitHub 仓库链接 - 修正许可证链接
- **README**: 更新 pt-tools 安装命令

### Features

- **site**: 添加对 CMCT 站点的支持
- 新增 CMCT 站点的配置和解析逻辑 - 实现 CMCT 站点的 RSS 订阅和种子下载功能 - 优化站点配置结构，支持更多站点类型 - 重构部分代码以提高可扩展性和可维护性 - 修改release压缩包内的二进制文件名统一为pt-tools - 在全局配置中增加 torrent_size_gb 选项，用于设置默认的下载种子大小限制 - 更新站点配置初始化和处理逻辑

## [0.0.2] - 2024-12-04

### Features

- 添加自动下载安装脚本并优化相关功能
- 新增 download.sh 脚本，实现自动检测平台并下载安装最新版本 pt-tools - 优化 Makefile 中的 upx-binaries 目标，增加对 windows-\*.exe 文件的判断 - 修复 MTTorrentDetail.CanbeFinished 方法，增加对 DiscountEndTime 为空的判断 - 优化 CanbeFinished 方法错误日志，增加 tid 信息

## [0.0.1] - 2024-12-04

### CI/CD

- **release**: 重构 GitHub Actions 工作流
- 更新工作流名称和步骤，增加 Docker 镜像构建和推送 - 移除不必要的环境变量和条件判断 - 简化二进制文件构建和打包流程 - 更新 Dockerfile，增加配置文件路径和调整ENTRYPOINT - 重构 Makefile，支持多平台构建和 UPX 压缩 - 更新 README，优化命令行用法说明 - 在 README.md 中新增配置说明部分，详细介绍配置文件结构和示例 - 新增 config.toml 文件，提供默认配置示例 - 更新 config.go 和 zap.go，调整配置结构和默认日志配置 - 在 Dockerfile 中添加构建环境和基础镜像的参数 - 实现本地和远程构建的逻辑区分 - 优化 Makefile 中的构建命令 - 添加 upx-binaries 目标，使用 UPX 压缩二进制文件 - 增加 package-binaries 目标，将二进制文件打包成 tar.gz 或 zip 格式 - 优化 build-binaries 目标，增加对不同操作系统和架构的支持 - 合并构建、压缩和打包二进制文件的步骤 - 添加 TAG 变量以支持自定义版本标签 - 在 Dockerfile 和 Makefile 中添加构建参数，用于设置版本信息 - 更新 Go 构建命令，将版本信息编译到可执行文件中 - 重构配置文件，增加全局配置和站点配置结构 - 新增 version 命令，用于显示版本信息

---

_Generated by [git-cliff](https://github.com/orhun/git-cliff)_
