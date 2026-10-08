# MCP

MCP (Model Context Protocol) is the protocol AI assistants use to call external tools. pt-tools serves MCP on its web port: once connected, the assistant in an MCP client such as Claude, Cursor or Cherry Studio can look at your torrents, downloaders, site statistics and subscriptions, search torrents, and, after you confirm, pause, delete or push torrents and add subscriptions.

The MCP endpoint shares the web interface's port: `http://<pt-tools host>:<port>/mcp`. The System → MCP access (系统 → MCP 接入) page shows this instance's address, how to set up each kind of client, and the tool list.

## Create a token

Create a token under System → API tokens (系统 → API 令牌), see [API tokens](api-tokens.md):

| Permission           | What it allows                                                                                                                                                     |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| MCP read (MCP 读取)  | Read-only tools: fetched torrents, torrents in the downloaders, downloader status, search, site statistics, update check, finding titles and the subscription list |
| MCP write (MCP 操作) | Tools that change your downloaders or subscriptions: pause, resume, delete and push torrents, add subscriptions                                                    |

Neither permission includes the other: a token with only MCP write cannot call the read-only tools, so you usually pick both. Pick only MCP read when the assistant should only look.

## Set up the client

The examples assume pt-tools runs at `192.168.1.10:8080` and the token is `ptt_1_xxxxxxxx`; use your own values.

### Clients that speak HTTP

Clients such as Cursor, Cherry Studio and VS Code take the address and a request header; choose the Streamable HTTP transport. Where the configuration lives depends on the client; the fields look like this:

```json
{
  "mcpServers": {
    "pt-tools": {
      "url": "http://192.168.1.10:8080/mcp",
      "headers": { "Authorization": "Bearer ptt_1_xxxxxxxx" }
    }
  }
}
```

Claude Code adds it with a command:

```bash
claude mcp add --transport http pt-tools http://192.168.1.10:8080/mcp --header "Authorization: Bearer ptt_1_xxxxxxxx"
```

### Clients that only speak stdio

Clients such as Claude Desktop only start a local program and talk to it over standard input and output. Put a copy of the pt-tools program on the client's machine (it does not have to run the web service) and let `pt-tools mcp` forward to the running pt-tools:

```json
{
  "mcpServers": {
    "pt-tools": {
      "command": "/usr/local/bin/pt-tools",
      "args": ["mcp", "--url", "http://192.168.1.10:8080"],
      "env": { "PT_TOOLS_MCP_TOKEN": "ptt_1_xxxxxxxx" }
    }
  }
}
```

- `pt-tools mcp` opens no database and starts no scheduler; it forwards the running pt-tools' tools as they are. See the [command line](../reference/cli.md#pt-tools-mcp) for its options.
- Put the token in the `PT_TOOLS_MCP_TOKEN` environment variable rather than in `args`: command-line arguments show up in the process list.
- On Windows, set `command` to the full path of `pt-tools.exe`.

## Tools

| Tool                       | Permission | What it does                                                                                                       |
| -------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------ |
| `list_tasks`               | MCP read   | Fetched torrents: what RSS, subscriptions, the app and MCP pushed, filterable by site and keyword                  |
| `list_downloader_torrents` | MCP read   | Torrents in the downloaders, paginated, filterable by downloader, state and keyword, sortable                      |
| `get_downloader_stats`     | MCP read   | Whether each downloader is reachable, its upload and download speed, totals and free space                         |
| `search_torrents`          | MCP read   | Search the enabled sites; results carry the site and torrent ID, never a download link                             |
| `get_site_userinfo`        | MCP read   | Upload, download, ratio, bonus, seeding count, unread messages, login state and check-in per site                  |
| `check_updates`            | MCP read   | Whether a newer release exists; it never upgrades                                                                  |
| `explore_media`            | MCP read   | Find movies and TV shows on TMDB by title, or list this week's trending and popular ones                           |
| `list_subscriptions`       | MCP read   | Subscriptions and their progress: aired, in the library, downloading and missing episodes                          |
| `pause_torrent`            | MCP write  | Pause one torrent                                                                                                  |
| `resume_torrent`           | MCP write  | Resume one paused torrent                                                                                          |
| `delete_torrent`           | MCP write  | Delete one torrent, optionally with its data                                                                       |
| `push_torrent`             | MCP write  | Push a torrent to a downloader, given the site and torrent ID from a search or a download URL of a configured site |
| `add_subscription`         | MCP write  | Subscribe to a movie or a TV season by TMDB ID                                                                     |

- Tools that change your downloaders or subscriptions require `confirm=true`: the assistant has to ask you first.
- `push_torrent` has pt-tools download the torrent file with its own site configuration and goes through the disk-space protection and the site seeding-capacity check as usual. It does not take magnet links and never requests any other address. Torrents pushed through MCP show the source `mcp_push` in the task list.
- The data returned is the same as the [App API](../reference/app-api.md) returns: no cookies, passkeys, passwords, RSS addresses or download links.

## Audit and security

- `/mcp` only accepts API tokens, not a browser session: no token or a wrong one gets 401, a token without an MCP permission gets 403. Revoking a token takes effect immediately.
- Every call of a tool that changes your downloaders or subscriptions is recorded under ChatOps → Audit log (操作审计), including the ones refused for a missing permission or confirmation: the channel is MCP, the command is the tool name and the user is the token ID.
- Over the internet, use HTTPS (for example behind a reverse proxy), or the token crosses the network in plain text. The reverse proxy has to keep the `Authorization` header.
