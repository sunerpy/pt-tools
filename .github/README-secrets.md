# Required secrets for GitHub Actions

Set these in repo Settings → Secrets and variables → Actions:

| Secret | Value |
|---|---|
| `TELEGRAM_BOT_TOKEN` | Bot API token from @BotFather (release announcer) |
| `TELEGRAM_CHAT_ID` | Target group chat_id (negative integer for supergroups) |

`GITHUB_TOKEN` is auto-provided.

The bot must be added to the target group AND granted **"Pin messages"** admin permission. Without that permission the message is still sent, but pinning silently warns and the workflow stays green.

To test without a real release, trigger `Telegram Release Announcement` workflow via Actions → Run workflow → enter tag (e.g. `v0.5.0`).
