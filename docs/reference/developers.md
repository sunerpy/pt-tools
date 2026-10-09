# 参与开发

本页介绍如何从源码构建 pt-tools、提交改动和修改本站文档。完整的开发指南见[开发指南](../development.md)。

## 从源码构建

仓库固定使用 Go `1.26.9`、Node.js `25.2.0` 和 pnpm `10.25.0`。克隆后运行完整的本地检查：

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools
make check
```

`make check` 依次检查工具链版本、格式、文档链接、lint、测试和构建。常用目标：

| 命令                   | 作用                         |
| ---------------------- | ---------------------------- |
| `make fmt`             | 格式化 Go 与前端代码         |
| `make lint`            | Go、前端 lint 与类型检查     |
| `make test`            | 前端构建与测试，Go race 测试 |
| `make build`           | 构建前端和当前平台的二进制   |
| `make build-extension` | 校验站点列表并打包浏览器扩展 |

## 提交改动

- 提交信息和 PR 标题遵循 Conventional Commits，例如 `fix(webui): …`、`feat(site): …`。
- 合并之前，CI 会运行与 `make check` 相同的检查。
- 贡献之前请阅读仓库根目录和各子目录的 `AGENTS.md`，其中记录了配置、推送安全检查和通知等模块的约束。

## 新增站点

新增站点只需要在 `site/v2/definitions/` 中添加站点定义，并补齐 fixture 测试；浏览器扩展中的站点列表要同步更新，`make check-sites` 会检查两者是否一致。步骤见[开发指南](../development.md)。

没有编程经验时，可以用浏览器扩展采集脱敏后的页面数据提交给维护者，见[请求新增站点](../guide/request-new-site.md)。

## 修改本站文档

本站的内容就是仓库中的 `docs/` 目录：中文页面在 `docs/` 下，英文页面在 `docs/en/` 下，路径一一对应。修改某个页面时：

1. 同时更新中英文两个版本；只有中文的开发指南和设计文档除外。
2. 页面之间用相对路径链接，例如 `../faq.md`，这样在 GitHub 上和本站中都能打开。
3. 提交 PR 后，`Docs site` 检查会把文档同步进站点并完整构建一次，失效链接、缺少某种语言的页面都会让检查失败。

写作约定、首页数据的格式和截图规范见 `docs/README.md`。合并后，站点会自动更新。

## 设计文档

以下设计文档只有中文，记录的是接口契约和后续方向，不表示相应能力已经发布：

- [ChatOps / MCP / Agent 架构](../design/chatops-mcp-agent.md)
- [MCP Server 设计](../design/phase4-mcp.md)
- [AI Agent 设计](../design/phase5-agent.md)
