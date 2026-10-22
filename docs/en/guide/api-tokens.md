# API tokens <Badge type="tip" text="1.0 preview" />

API tokens are for clients such as a mobile app or a script: with a token a client can use the pt-tools App API (`/api/app/v1/`), sign in to the [qB-compatible entrance](qbit-compat.md), or let an AI assistant call pt-tools over [MCP](mcp.md), without storing your web login password. A token cannot reach any other part of the web interface; those still require you to sign in.

Open System → API tokens (系统 → API 令牌).

## Creating a token

1. Click New (新建), enter a name (for example "My phone"), and choose the permissions and how long the token stays valid.
2. The dialog that opens shows the token itself, starting with `ptt_`. It is shown only this once: copy it into the client before closing the dialog. If you lose it, revoke it and create another.

| Permission              | What it allows                                                                                                                                                                                                                                          |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Read (读取)             | Overview, sites, torrents in the downloaders, RSS push records, search, brush tasks, organise history, subscriptions and Explore                                                                                                                        |
| Operate (操作)          | Pause, resume and delete torrents, push torrents, sign in to sites, create, pause, resume, search and delete subscriptions                                                                                                                              |
| Config (配置)           | Read and change pt-tools' configuration (the [App API configuration endpoints](../reference/app-api.md#configuration) and the MCP configuration tools); secrets are write-only. Also needs Read or Operate, and MCP read or MCP write for the MCP tools |
| qB compatible (qB 兼容) | Signing in to the [qB-compatible entrance](qbit-compat.md), for tools that only work with qBittorrent such as MoviePilot and IYUU; independent of the other two                                                                                         |
| MCP read (MCP 读取)     | Read-only [MCP](mcp.md) tools for AI assistants: torrents, downloaders, site statistics, subscriptions and search                                                                                                                                       |
| MCP write (MCP 操作)    | MCP tools that pause, resume, delete and push torrents, manage subscriptions and followed shows, and organize and scrape the library; the assistant has to ask you each time. MCP access has to be on under System → MCP access                         |

A token with only Operate cannot read anything, so clients such as the app usually need both; add Config for a script or assistant that should read or change the configuration (existing tokens do not get it automatically); a token for the qB-compatible entrance needs only qB compatible; a token for an AI assistant needs MCP read, plus MCP write if it may change your downloaders. Validity can be 30 days, 90 days, 1 year or no expiry; an expired token stops working on its own. You can create up to 50 tokens.

## Using a token

The client sends `Authorization: Bearer <token>` with every request. Tokens are only accepted in this header, never in the address.

```bash
curl -H "Authorization: Bearer ptt_1_xxxxxxxx" https://pt-tools.example.com/api/app/v1/meta
```

See the [App API](../reference/app-api.md) for the endpoints.

## Revoking and usage

- The list shows each token's permissions, when it was last used and when it expires. Last used is updated at most once every 5 minutes.
- Revoke (撤销) takes effect immediately: clients using that token are disconnected at once.
- Every call a token makes to an Operate endpoint, including calls refused for lack of permission, is recorded under ChatOps → Audit log (操作审计), with the channel API token (API 令牌) and the token number as the triggering user (触发用户). Calls of MCP tools that change your downloaders or subscriptions are recorded there too, with the channel MCP; see [MCP](mcp.md#audit-and-security).

## Security

- pt-tools stores only the SHA-256 digest of each token, not the token itself, so the database alone cannot be used to recover a token.
- Keep tokens as safe as passwords. When reaching pt-tools over the internet, use HTTPS (for example behind a reverse proxy); otherwise the token crosses the network in plain text.
- The settings endpoints the web pages use only accept a web login; tokens cannot reach them. A token with Config can change most settings through the configuration endpoints, but cannot read secrets (cookies, passwords and tokens only show whether they are set), cannot change the login password, API tokens, remote access, the encryption key or the AI assistant's own settings, and cannot create other tokens.
