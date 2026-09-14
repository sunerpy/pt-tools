# pt-tools 文档中心

[返回项目首页](../README.md)

本文档中心按使用场景组织 pt-tools 的部署、配置、自动化、ChatOps 和开发资料。首次使用建议依次阅读「部署与配置」和「RSS 与下载管理」。

## 部署与配置

- [配置说明](configuration.md)：环境变量、代理、全局设置、下载器、持久化、日志和备份。
- [Docker 运行示例](../examples/docker-run.md)：单容器部署、数据目录和管理员密码重置。
- [Docker Compose 示例](../examples/docker-compose.yml)：推荐的持久化与日志轮转配置。
- [二进制运行示例](../examples/binary-run.md)：Linux、Windows、systemd 与常用命令。
- [常见问题](faq.md)：下载器、认证、RSS、免费暂停与数据库排障。

## 站点与认证

- [支持站点](sites.md)：当前 66 个内置站点、认证方式与适配备注。
- [获取 Cookie / API Key](guide/get-cookie-apikey.md)：浏览器扩展同步与手动获取认证信息。
- [站点登录状态与密钥备份](guide/site-login-monitoring.md)：登录状态监控、提醒策略、`secret.key` 备份与合规边界。
- [请求新增站点](guide/request-new-site.md)：采集、脱敏并提交新站点适配资料。
- [PT Tools Helper](../tools/browser-extension/README.md)：Cookie 同步、页面采集和扩展开发。

## RSS 与下载管理

- [RSS 订阅配置](guide/rss-subscription.md)：订阅地址、配置选项与故障排查。
- [过滤规则与追剧](guide/filter-rules-tv-series.md)：关键词、通配符、正则和组合规则。
- [自动删种与磁盘保护](guide/auto-cleanup.md)：清理范围、H&R 保护、磁盘门禁和工作目录清理。

## ChatOps 与通知

- [ChatOps 快速开始](guide/chatops-quickstart.md)：通道选择、命令清单与配置入口。
- [QQ OneBot（NapCat）](guide/chatops-qq-napcat.md)：反向 WebSocket、绑定与排障。
- [Telegram Bot](guide/chatops-telegram.md)：BotFather、代理、绑定与排障。
- [RSS 上新通知](guide/chatops-rss-notify.md)：通知模式、静默时段、digest、重试和配额。

## 开发与设计

- [开发指南](development.md)：固定工具链、构建门禁、代码规范、站点定义与 fixture 测试。
- [ChatOps / MCP / Agent 架构](design/chatops-mcp-agent.md)：能力分层、权限模型与当前实现边界。
- [MCP Server 接口契约](design/phase4-mcp.md)：未来 MCP 工具、传输与安全边界。
- [AI Agent 设计](design/phase5-agent.md)：未来 Agent 模式、接入路径与非目标。

> [!NOTE]
> `docs/design/` 记录设计契约和后续演进方向，不表示相应能力已经对外发布。当前用户可用能力以 README、Web UI 与 Release 说明为准。
