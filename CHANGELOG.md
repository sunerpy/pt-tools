# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.17.0] - 2026-02-21

### Features

- **ui**: 站点管理与首页增加浏览器扩展引导提示
- 站点管理页禁用的「新增站点」按钮增加 Popover 悬浮提示和 Alert 横幅 - 首页 Dashboard 顶部增加可关闭的扩展推荐横幅 - 引导用户通过浏览器扩展快速适配新站点，提供下载和文档链接 - 移除不再使用的 addSite 函数避免构建报错

## [0.16.0] - 2026-02-21

### Documentation

- **guide**: 重构新站点请求指南为扩展优先
- 新增方式一：浏览器扩展自动采集（安装、一键采集、导出提交完整步骤）- 原有手动步骤降级为方式二 - 增加两种方式对比表

### Features

- **cleanup**: 支持免费期结束自动删除未完成种子
  新增全局设置「免费结束自动删除」，开启后免费期结束时未下载完成的种子
  将自动从下载器删除（含数据文件），无需手动操作。默认关闭。

        - SettingsGlobal 新增 AutoDeleteOnFreeEnd 字段
        - FreeEndMonitor 新增自动删除分支，仅作用于免费期结束未完成的种子
        - 系统设置页面新增「免费结束管理」区块含开关和警告提示
        - 暂停任务页面新增自动删除快捷开关（含悬浮提示）
        - README 补充功能说明

        Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-opencode)

### Performance

- **cleanup**: 优化磁盘紧急清理策略
- 紧急清理目标增加缓冲区（阈值 20% 或至少 10GB），避免清理后立即再次触底 - 新增 DiskSpaceLow 事件，推送检测空间不足时通知清理监控立即执行 - CleanupMonitor 订阅事件总线，收到信号后 3 秒去抖再立即清理 - 仅在自动删种启用时才发送磁盘空间不足信号

      Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-opencode)

## [0.15.0] - 2026-02-19

### Bug Fixes

- **build**: 修复站点一致性检查脚本引号匹配
- check-sites.ts 中扩展站点 ID 提取正则兼容双引号和单引号
- **api**: 修复搜索站点校验在测试环境空指针问题
- getEnabledSiteIDs 增加 store 空值检查避免测试中 panic
- **test**: 适配登录接口 JSON 响应变更
- 登录测试预期状态码从 302 改为 200 以匹配 JSON 请求返回 JSON 响应的行为

### CI/CD

- Bump actions/upload-artifact from 4 to 6 ([#53](https://github.com/sunerpy/pt-tools/issues/53)) ([#53](https://github.com/sunerpy/pt-tools/pull/53))
  Bumps [actions/upload-artifact](https://github.com/actions/upload-artifact) from 4 to 6. - [Release notes](https://github.com/actions/upload-artifact/releases) - [Commits](https://github.com/actions/upload-artifact/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/upload-artifact
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump vue-router from 4.6.4 to 5.0.2 in /web/frontend ([#57](https://github.com/sunerpy/pt-tools/issues/57)) ([#57](https://github.com/sunerpy/pt-tools/pull/57))
  Bumps [vue-router](https://github.com/vuejs/router) from 4.6.4 to 5.0.2. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v4.6.4...v5.0.2)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.2
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Features

- **extension**: 增加 PT Tools Helper 浏览器扩展及配套设施
- 新增 Chrome/Edge 浏览器扩展 (tools/browser-extension) - 支持 Cookie 自动同步、批量同步、一键采集站点数据 - 内置 337 个 PT 站点域名识别库，支持中英文界面 - 后端新增 PUT /api/sites/{name} 凭据更新和 /api/ping 健康检查 - 后端增加 CORS 支持、JSON 登录响应、搜索前站点启用校验 - 前端搜索前刷新可用站点列表防止搜索禁用站点 - 新增图标生成脚本和站点一致性检查脚本 - 新增扩展构建发布 CI 流程 (ext-v\* tag 触发 Edge Add-ons 发布) - 更新文档：Cookie 配置优先推荐浏览器扩展同步方式

## [0.14.0] - 2026-02-17

### Dependencies (Frontend)

- **pnpm**: Bump @types/node from 25.2.2 to 25.2.3 in /web/frontend ([#100](https://github.com/sunerpy/pt-tools/issues/100)) ([#100](https://github.com/sunerpy/pt-tools/pull/100))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.2.2 to 25.2.3. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vueuse/core from 14.2.0 to 14.2.1 in /web/frontend ([#101](https://github.com/sunerpy/pt-tools/issues/101)) ([#101](https://github.com/sunerpy/pt-tools/pull/101))
  Bumps [@vueuse/core](https://github.com/vueuse/vueuse/tree/HEAD/packages/core) from 14.2.0 to 14.2.1. - [Release notes](https://github.com/vueuse/vueuse/releases) - [Commits](https://github.com/vueuse/vueuse/commits/v14.2.1/packages/core)

        ---
        updated-dependencies:
        - dependency-name: "@vueuse/core"
         dependency-version: 14.2.1
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.43.0 to 1.48.0 in /web/frontend ([#102](https://github.com/sunerpy/pt-tools/issues/102)) ([#102](https://github.com/sunerpy/pt-tools/pull/102))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.43.0 to 1.48.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.48.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.48.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump marked from 17.0.1 to 17.0.2 in /web/frontend ([#103](https://github.com/sunerpy/pt-tools/issues/103)) ([#103](https://github.com/sunerpy/pt-tools/pull/103))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.1 to 17.0.2. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.1...v17.0.2)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.28.0 to 0.33.0 in /web/frontend ([#104](https://github.com/sunerpy/pt-tools/issues/104)) ([#104](https://github.com/sunerpy/pt-tools/pull/104))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.28.0 to 0.33.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.33.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.33.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Features

- **cleanup**: 磁盘空间保护与自动删种优化 ([#105](https://github.com/sunerpy/pt-tools/issues/105)) ([#105](https://github.com/sunerpy/pt-tools/pull/105))
- RSS 推送前增加磁盘空间预检查，空间不足时拒绝推送并短路剩余种子 - 手动推送入口同步增加空间预检查 - 修复 SaveGlobalSettings 更新分支丢失 Cleanup/MaxRetry 等字段的问题 - 修复 MaxRetry=0 时所有种子被误判为超过重试次数的问题 - 修复 CanbeFinished 单位换算错误导致免费期判断失效的问题 - 新增最短免费时间阈值(MinFreeMinutes)，跳过免费剩余时间不足的种子 - 自动删种预设方案选择后保留选中状态，页面加载时反向匹配预设 - 自动删种检查增加运行状态日志，缩短启动延迟 - NexusPHP 站点(hdsky/novahd)降低默认请求速率，减少频率限制误判 - CleanupDiskProtect 默认值改为 true - 新增自动删种功能文档，更新配置文档和 FAQ

## [0.13.0] - 2026-02-12

### Dependencies (Frontend)

- **pnpm**: Bump vue from 3.5.27 to 3.5.28 in /web/frontend ([#93](https://github.com/sunerpy/pt-tools/issues/93)) ([#93](https://github.com/sunerpy/pt-tools/pull/93))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.27 to 3.5.28. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.27...v3.5.28)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.28
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.2.0 to 25.2.2 in /web/frontend ([#96](https://github.com/sunerpy/pt-tools/issues/96)) ([#96](https://github.com/sunerpy/pt-tools/pull/96))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.2.0 to 25.2.2. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.2
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.40.0 to 0.41.0 ([#95](https://github.com/sunerpy/pt-tools/issues/95)) ([#95](https://github.com/sunerpy/pt-tools/pull/95))
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.40.0 to 0.41.0. - [Commits](https://github.com/golang/sys/compare/v0.40.0...v0.41.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.41.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/text from 0.33.0 to 0.34.0 ([#94](https://github.com/sunerpy/pt-tools/issues/94)) ([#94](https://github.com/sunerpy/pt-tools/pull/94))
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.33.0 to 0.34.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.33.0...v0.34.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.34.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **multi**: 多站点优化与代理支持 ([#97](https://github.com/sunerpy/pt-tools/issues/97)) ([#97](https://github.com/sunerpy/pt-tools/pull/97))
- 增加 ALL_PROXY 支持并统一 HTTP 客户端代理与连接池 - 迁移站点验证器从 net/http 到 requests 库 - 删除无调用者的死代码和冗余依赖 - 新增代理配置文档和使用说明 - 更新站点列表和 docker-compose 配置 - 修复多站点免费解析失败与种子过期误判(HDDolby/RousiPro/SpringSunday) - 修复 RSS 任务统计计数与站点列表 Passkey 缺失 - 美化主题配色系统并重命名高辨识为极光配色

### Miscellaneous

- **config**: 调整 release-please 版本策略
- bump-patch-for-minor-pre-major 改为 false - feat 类型在 v0.x 阶段触发 minor 版本升级

## [0.12.6] - 2026-02-08

### Bug Fixes

- **frontend**: 修复日志页面加载卡顿问题

## [0.12.5] - 2026-02-07

### Bug Fixes

- **release**: 将构建发布流程收敛到 release-please 内部
- 在同一工作流内串联 release-please、build-and-release、update-changelog - 避免跨工作流触发造成重复触发或遗漏 - 确保 release PR 合并后按单链路完成 tag 后构建发布

## [0.12.4] - 2026-02-07

### Bug Fixes

- **release**: 回滚 release-please 标题模式与 component 配置 ([#86](https://github.com/sunerpy/pt-tools/issues/86)) ([#86](https://github.com/sunerpy/pt-tools/pull/86))
- 恢复 pull-request-title-pattern 为 chore: release - 恢复 pull-request-header 为 ## Release - 移除根包 component 配置，回到此前可稳定触发发布的单包模式
- **release**: 拆分 release-please 与 tag 构建流程 ([#88](https://github.com/sunerpy/pt-tools/issues/88)) ([#88](https://github.com/sunerpy/pt-tools/pull/88))
- release-please 工作流仅负责创建 Release PR 与更新 changelog - 新增 release-assets 工作流，仅在 v\* tag 或手动触发时构建并发布资产 - 避免普通 main 提交在未确认 tag 发布前触发 Build and Release
- **release**: 拆分 release-please 与 tag 构建流程 ([#87](https://github.com/sunerpy/pt-tools/pull/87))
- release-please 工作流仅负责创建 Release PR 与更新 changelog - 新增 release-assets 工作流，仅在 v\* tag 或手动触发时构建并发布资产 - 避免普通 main 提交在未确认 tag 发布前触发 Build and Release

## [0.12.3] - 2026-02-07

### Features

- **site**: 增加站点定义 CI 校验体系与 RSS 免费下载说明 ([#84](https://github.com/sunerpy/pt-tools/issues/84)) ([#84](https://github.com/sunerpy/pt-tools/pull/84))
- 新增 SiteDefinition.Validate() 校验方法及完整单元测试 - RegisterSiteDefinition() 增加重复 ID 检测 - 新增 FixtureSuite 框架，全部 6 个内置站点迁移至 fixture 测试 - 清空 legacy 白名单，所有站点通过动态注册表驱动测试 - 更新 docs/development.md 增加测试指南 - README/RSS 指南/过滤规则指南增加警告：默认仅下载免费种子 - 前端 RSS 页面和过滤规则页面增加 warning 级别提醒横幅
- **site**: 增加站点定义 CI 校验体系与 RSS 免费下载说明 ([#85](https://github.com/sunerpy/pt-tools/pull/85))
- 新增 SiteDefinition.Validate() 校验方法及完整单元测试 - RegisterSiteDefinition() 增加重复 ID 检测 - 新增 FixtureSuite 框架，全部 6 个内置站点迁移至 fixture 测试 - 清空 legacy 白名单，所有站点通过动态注册表驱动测试 - 更新 docs/development.md 增加测试指南 - README/RSS 指南/过滤规则指南增加警告：默认仅下载免费种子 - 前端 RSS 页面和过滤规则页面增加 warning 级别提醒横幅

### Miscellaneous

- **ci**: 调整 GitHub Actions 分支触发规则并更新 release-please 配置 ([#83](https://github.com/sunerpy/pt-tools/issues/83)) ([#83](https://github.com/sunerpy/pt-tools/pull/83))

* chore(ci): 调整 GitHub Actions 分支触发规则并更新 release-please 配置

      - 限制 CI 触发分支为 main
      - 更新 release-please 标题模板并指定组件名

      * chore(build): 更新 Go 版本至 1.25.7

      - 同步 Dockerfile 和 Makefile 中的构建镜像版本
      - 更新 go.mod 文件中的 Go 模块版本要求

      * chore(ci): 简化 Go 构建测试工作流并使用 go.mod 指定版本

      - 使用 go.mod 文件指定 Go 版本以确保一致性

      * refactor(site): 抽离时间参数以支持测试断言

      - 新增 parseMTorrentDiscountWithPromotionAt 方法用于注入时间
      - 固定测试时间避免随机性影响断言结果

## [0.12.2] - 2026-02-05

### Bug Fixes

- **rss**: 修复种子大小限制独立于限速开关生效 ([#81](https://github.com/sunerpy/pt-tools/issues/81)) ([#81](https://github.com/sunerpy/pt-tools/pull/81))
- TorrentSizeGB 设置现在即使未启用下载限速也会生效 - 先检查种子大小限制，再检查限速时间
- **rss**: 修复种子大小限制独立于限速开关生效 ([#81](https://github.com/sunerpy/pt-tools/issues/81)) ([#82](https://github.com/sunerpy/pt-tools/pull/82))
- TorrentSizeGB 设置现在即使未启用下载限速也会生效

## [0.12.1] - 2026-02-05

### Bug Fixes

- **rss**: Allow longer intervals and stabilize release-please config ([#78](https://github.com/sunerpy/pt-tools/issues/78)) ([#78](https://github.com/sunerpy/pt-tools/pull/78))
- **release**: Remove manifest schema for release-please ([#79](https://github.com/sunerpy/pt-tools/issues/79)) ([#79](https://github.com/sunerpy/pt-tools/pull/79))
- **release**: Remove manifest schema for release-please ([#80](https://github.com/sunerpy/pt-tools/pull/80))

## [0.12.0] - 2026-02-05

### Features

- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍
- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断 ([#76](https://github.com/sunerpy/pt-tools/pull/76))
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍
- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断 ([#77](https://github.com/sunerpy/pt-tools/pull/77))
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍

## [0.11.0] - 2026-02-05

### CI/CD

- 修复 Release Please 生成文件的格式化问题
- 在 update-changelog job 中格式化所有 Release Please 生成的文件 - 包括 .release-please-manifest.json 和 release-please-config.json - 移除冗余注释
- 修复 Release Please 生成文件的格式化问题 ([#70](https://github.com/sunerpy/pt-tools/pull/70))
- 在 update-changelog job 中格式化所有 Release Please 生成的文件

### Features

- **docker**: 增加 ARM64 架构支持
- **docker**: 增加 ARM64 架构支持 ([#73](https://github.com/sunerpy/pt-tools/pull/73))
- 增加 ARM64 架构支持 (c6d7ad4), closes #72 ([#74](https://github.com/sunerpy/pt-tools/pull/74))

### Miscellaneous

- **main**: Release 0.11.0

## [0.10.2] - 2026-02-04

### Bug Fixes

- 修复下载器地址不通时 web 无法访问的问题 ([#66](https://github.com/sunerpy/pt-tools/issues/66))
- 修复下载器地址不通时 web 无法访问的问题 ([#66](https://github.com/sunerpy/pt-tools/issues/66)) ([#68](https://github.com/sunerpy/pt-tools/pull/68))
- 将下载器健康检查改为 goroutine 异步执行，不阻塞启动 - 健康状态并行加载，互不阻塞

### CI/CD

- 使用 Release Please 自动化版本发布
  替换手动 tag 发布流程为 Release Please 自动化发布:

        - 添加 release-please.yml: 基于 Conventional Commits 自动创建 Release PR
        - 添加 release-please-config.json: 配置版本规则和 changelog 分类
        - 添加 .release-please-manifest.json: 跟踪当前版本 (v0.10.1)
        - 删除 release.yml: 旧的手动 tag 触发发布
        - 删除 changelog.yml: 旧的手动 changelog 更新

### Miscellaneous

- **main**: Release 0.10.2

## [0.10.1] - 2026-02-03

### Bug Fixes

- 修复低版本数据迁移导致的不兼容问题
- 强制同步站点认证方式与默认URL，确保旧数据正确迁移 - 新增 defaultAPIUrlForSite 函数统一设置 MTeam API URL - 支持旧版密码格式兼容（明文/SHA256）自动升级为新格式
- **frontend**: 修复 SiteList 组件 TypeScript 类型错误
  MessageBoxData 类型不能直接解构 value 属性
- 修复低版本数据迁移导致的不兼容问题 ([#64](https://github.com/sunerpy/pt-tools/pull/64))
- 强制同步站点认证方式与默认URL，确保旧数据正确迁移 - 新增 defaultAPIUrlForSite 函数统一设置 MTeam API URL - 支持旧版密码格式自动升级为新格式
- 修复数据库锁、事务超时及 MTeam 促销规则解析问题
- 移除全局信号量，解决前端页面加载慢的问题 - 移除事务中的 HTTP 调用，避免 context deadline exceeded 错误 - RSS 无关联过滤规则时不再匹配全局规则 - MTeam GetTorrentDetail 正确解析 promotionRule 促销折扣

### Dependencies (Frontend)

- **pnpm**: Bump oxfmt from 0.27.0 to 0.28.0 in /web/frontend ([#54](https://github.com/sunerpy/pt-tools/issues/54)) ([#54](https://github.com/sunerpy/pt-tools/pull/54))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.27.0 to 0.28.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.28.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.28.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 4.0.17 to 4.0.18 in /web/frontend ([#55](https://github.com/sunerpy/pt-tools/issues/55)) ([#55](https://github.com/sunerpy/pt-tools/pull/55))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.0.17 to 4.0.18. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.0.18/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.0.18
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump element-plus from 2.13.1 to 2.13.2 in /web/frontend ([#58](https://github.com/sunerpy/pt-tools/issues/58)) ([#58](https://github.com/sunerpy/pt-tools/pull/58))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.1 to 2.13.2. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.1...2.13.2)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vueuse/core from 14.1.0 to 14.2.0 in /web/frontend ([#60](https://github.com/sunerpy/pt-tools/issues/60)) ([#60](https://github.com/sunerpy/pt-tools/pull/60))
  Bumps [@vueuse/core](https://github.com/vueuse/vueuse/tree/HEAD/packages/core) from 14.1.0 to 14.2.0. - [Release notes](https://github.com/vueuse/vueuse/releases) - [Commits](https://github.com/vueuse/vueuse/commits/v14.2.0/packages/core)

        ---
        updated-dependencies:
        - dependency-name: "@vueuse/core"
         dependency-version: 14.2.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.42.0 to 1.43.0 in /web/frontend ([#56](https://github.com/sunerpy/pt-tools/issues/56)) ([#56](https://github.com/sunerpy/pt-tools/pull/56))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.42.0 to 1.43.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.43.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.43.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.0.7 to 25.2.0 in /web/frontend ([#59](https://github.com/sunerpy/pt-tools/issues/59)) ([#59](https://github.com/sunerpy/pt-tools/pull/59))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.0.7 to 25.2.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump sass from 1.97.2 to 1.97.3 in /web/frontend ([#62](https://github.com/sunerpy/pt-tools/issues/62)) ([#62](https://github.com/sunerpy/pt-tools/pull/62))
  Bumps [sass](https://github.com/sass/dart-sass) from 1.97.2 to 1.97.3. - [Release notes](https://github.com/sass/dart-sass/releases) - [Changelog](https://github.com/sass/dart-sass/blob/main/CHANGELOG.md) - [Commits](https://github.com/sass/dart-sass/compare/1.97.2...1.97.3)

        ---
        updated-dependencies:
        - dependency-name: sass
         dependency-version: 1.97.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vitejs/plugin-vue from 6.0.3 to 6.0.4 in /web/frontend ([#61](https://github.com/sunerpy/pt-tools/issues/61)) ([#61](https://github.com/sunerpy/pt-tools/pull/61))
  Bumps [@vitejs/plugin-vue](https://github.com/vitejs/vite-plugin-vue/tree/HEAD/packages/plugin-vue) from 6.0.3 to 6.0.4. - [Release notes](https://github.com/vitejs/vite-plugin-vue/releases) - [Changelog](https://github.com/vitejs/vite-plugin-vue/blob/main/packages/plugin-vue/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite-plugin-vue/commits/plugin-vue@6.0.4/packages/plugin-vue)

        ---
        updated-dependencies:
        - dependency-name: "@vitejs/plugin-vue"
         dependency-version: 6.0.4
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

## [0.10.0] - 2026-02-02

### Features

- **export**: 导出图片显示用户等级信息
- Canvas 导出和 HTML 预览均显示站点等级 - 等级显示在用户名右侧，使用紫色标识
- **site**: 新增 RousiPro 站点支持
- 新增 RousiPro (rousipro) 站点支持 - 修复 NovaHD 免费种子检测问题 (Issue #50)
- **site**: 新增 RousiPro 站点支持 ([#52](https://github.com/sunerpy/pt-tools/pull/52))
- 新增 RousiPro (rousipro) 站点支持 - 修复 NovaHD 免费种子检测问题 (Issue #50)

## [0.9.1] - 2026-01-31

### Bug Fixes

- **site**: 修复站点404错误
- 站点验证改为从 Registry 动态获取，支持所有已注册站点
- **site**: 修复站点404错误 ([#49](https://github.com/sunerpy/pt-tools/pull/49))
- 站点验证改为从 Registry 动态获取，支持所有已注册站点

## [0.9.0] - 2026-01-31

### Features

- **site**: 新增 NovaHD 站点支持 + 修复图片分享功能
  NovaHD 站点支持: - 新增 NovaHD 站点定义，包含 9 个等级要求 - 自定义 DetailParser 配置用于解析优惠和结束时间
- **site**: 新增 NovaHD 站点支持 + 修复图片分享功能 ([#48](https://github.com/sunerpy/pt-tools/pull/48))
- 新增 NovaHD 站点定义，包含 9 个等级要求 - 自定义 DetailParser 配置用于解析优惠和结束时间

      - 修复 HTTP 环境下剪贴板 API 不可用导致的错误
      - 优化分享率颜色对比度，在绿色主题下更易辨识
      - 站点卡片显示入站日期和时长

## [0.8.0] - 2026-01-31

### Features

- **site**: HDDolby 两步验证支持 + 解析逻辑优化
  主要变更：- feat(hddolby): 新增 HDDolbyDriver 支持两步验证（Cookie + 详情页解析）- feat(ratelimit): 实现 SQLite 持久化速率限制器，重启后状态不丢失 - refactor(parser): 统一 NexusPHP 详情页解析配置到 SiteDefinition - feat(discount): 搜索结果页支持可配置的 DiscountMapping - docs: 更新开发指南，添加持久化限流和解析配置说明

        技术细节：
        - 新增 models/rate_limit.go (SiteRateLimit 数据模型)
        - 新增 site/v2/persistent_rate_limiter.go (滑动窗口限速器)
        - 新增 site/v2/hddolby_driver.go (HDDolby 专用驱动)
        - 删除冗余的 site/parser/ 和 site/mocks/ 目录
        - SiteDefinition 新增 DetailParser 和 DiscountMapping 配置

- **site**: HDDolby 两步验证支持 + 支持分享站点数据截图 ([#47](https://github.com/sunerpy/pt-tools/pull/47))
- feat(hddolby): 新增 HDDolby 支持两步验证（Cookie + 详情页解析）- feat: 支持用户统计页面导出分享数据截图，支持模糊站点logo、名称、用户名等自定义项 - refactor(parser): 统一 NexusPHP 详情页解析配置到 SiteDefinition - feat(discount): 搜索结果页支持可配置的 DiscountMapping - docs: 更新开发指南，添加持久化限流和解析配置说明

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
