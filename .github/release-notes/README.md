# Release Announcement (Telegram)

Telegram 发版公告由 `telegram-release-announce.yml` 在 GitHub Release `published` 时自动发送，
或通过 `workflow_dispatch` 手动触发。它**不影响** GitHub Release body 与项目 `CHANGELOG.md`。

## 公告正文优先级（高 → 低）

1. **手动输入**：`workflow_dispatch` 触发时填写的 `announcement` 输入。发版时确认好公告文案后直接粘贴即可，**无需在仓库内提交文件**。
2. **覆盖文件（旧机制，仍兼容）**：`.github/release-notes/<tag>.md`（带或不带 `v` 前缀均可）。仅作历史兼容，**不再要求每个版本提交**。
3. **兜底**：release-please 生成的 Release body（含 dependabot 噪音，质量较低，仅在前两者均缺失时使用）。

## 推荐发版流程

1. 合并 release PR，发布 GitHub Release。
2. 在 Actions 中手动运行 **Telegram Release Announcement**（`workflow_dispatch`）：
   - `tag`：填版本 tag（如 `v0.35.0`）
   - `announcement`：粘贴确认好的公告 Markdown（≤ 600 中文字符，见 `pt-tools-workflow` 约定）
3. 工作流用该正文渲染并推送/置顶到 Telegram。

> 不再为每个版本在此目录提交 `<tag>.md`；公告内容在发版时确认后通过手动输入触发即可。
