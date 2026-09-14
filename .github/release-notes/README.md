# Release notes 模板

本目录只维护发布工作流附加到 GitHub Release 正文末尾的安装说明：

- `stable-footer.md`：稳定版本；
- `rc-footer.md`：预发布版本。

`.github/workflows/release.yml` 在 Release 仍为 draft 时选择模板、替换 `TAG_NAME`，并用成对 HTML marker 覆盖旧 footer。release-please 独占 `CHANGELOG.md` 和正文中的版本变更；这里不再保存逐版本公告文件，也不运行第二套 changelog 生成器。

Telegram 公告是同一 Release run 在公开后的尽力而为副作用：

1. 合并 release PR 前，按 `pt-tools-workflow` 预览面向用户的公告并取得用户明确批准；
2. 批准后确保仓库变量 `ANNOUNCE_TELEGRAM` 不是 `false`；
3. `telegram-announce` job 从已公开 Release 正文渲染 MarkdownV2，并用 `<!-- pt-tools-tg-announced -->` marker 保证同 tag 不重复发送。

恢复或验收发布链时可把 `ANNOUNCE_TELEGRAM=false`，此时不会发送公告。不要重新引入独立的 `release: published` workflow：由 `GITHUB_TOKEN` 公开 Release 不会触发该事件的新 workflow run。
